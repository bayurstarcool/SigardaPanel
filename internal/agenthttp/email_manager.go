package agenthttp

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/labstack/echo/v5"
)

// EmailManager handles email account management (Postfix + Dovecot).
type EmailManager struct {
	Domain    string `json:"domain"`
	Email     string `json:"email"`
	Password  string `json:"password"`
	Quota     int    `json:"quota"` // MB, 0 = unlimited
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

// manageEmail handles email operations (create, delete, list, update-password).
func (r *Router) manageEmail(c *echo.Context) error {
	var payload struct {
		Action    string `json:"action"`
		Domain    string `json:"domain"`
		Email     string `json:"email"`
		Password  string `json:"password"`
		Quota     int    `json:"quota"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}

	// Sanitize all inputs
	payload.Domain = sanitizeInput(payload.Domain)
	payload.Email = sanitizeInput(payload.Email)
	payload.FirstName = sanitizeInput(payload.FirstName)
	payload.LastName = sanitizeInput(payload.LastName)

	switch payload.Action {
	case "install":
		return r.installEmailServer(c)
	case "create":
		return r.createEmailAccount(c, payload.Domain, payload.Email, payload.Password, payload.Quota, payload.FirstName, payload.LastName)
	case "delete":
		return r.deleteEmailAccount(c, payload.Email)
	case "list":
		return r.listEmailAccounts(c, payload.Domain)
	case "update-password":
		return r.updateEmailPassword(c, payload.Email, payload.Password)
	case "status":
		return r.emailStatus(c)
	default:
		return echo.NewHTTPError(http.StatusBadRequest, "unknown action: "+payload.Action)
	}
}

// installEmailServer installs Postfix + Dovecot + Roundcube.
func (r *Router) installEmailServer(c *echo.Context) error {
	ctx := c.Request().Context()

	slog.Info("email: installing email server")

	// Install Postfix
	if _, err := r.executor.RunShell(ctx, "apt-get", "install", "-y", "postfix"); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to install Postfix: "+err.Error())
	}

	// Install Dovecot
	if _, err := r.executor.RunShell(ctx, "apt-get", "install", "-y", "dovecot-core", "dovecot-imapd", "dovecot-pop3d", "dovecot-lmtpd"); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to install Dovecot: "+err.Error())
	}

	// Install Roundcube
	if _, err := r.executor.RunShell(ctx, "apt-get", "install", "-y", "roundcube", "roundcube-core", "roundcube-mysql", "roundcube-plugins"); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to install Roundcube: "+err.Error())
	}

	// Configure Postfix main.cf
	postfixConfig := `# SigardaPanel Postfix Configuration
smtpd_banner = $myhostname ESMTP
biff = no
append_dot_mydomain = no
readme_directory = no
compatibility_level = 3.6

# TLS parameters
smtpd_tls_cert_file = /etc/ssl/certs/ssl-cert-snakeoil.pem
smtpd_tls_key_file = /etc/ssl/private/ssl-cert-snakeoil.key
smtpd_use_tls = yes
smtpd_tls_session_cache_database = btree:${data_directory}/smtpd_scache
smtp_tls_session_cache_database = btree:${data_directory}/smtp_scache

smtpd_relay_restrictions = permit_mynetworks permit_sasl_authenticated defer_unauth_destination
myhostname = mail.sigarda.dev
alias_maps = hash:/etc/aliases
alias_database = hash:/etc/aliases
mydestination = $myhostname, localhost.localdomain, localhost
relayhost =
mynetworks = 127.0.0.0/8 [::ffff:127.0.0.0]/104 [::1]/128
mailbox_size_limit = 0
recipient_delimiter = +
inet_interfaces = all
inet_protocols = all

# Virtual mailbox settings
virtual_mailbox_domains = hash:/etc/postfix/vdomains
virtual_mailbox_base = /var/mail/vhosts
virtual_mailbox_maps = hash:/etc/postfix/vmailbox
virtual_minimum_uid = 100
virtual_uid_maps = static:5000
virtual_gid_maps = static:5000
virtual_alias_maps = hash:/etc/postfix/valias

# SASL authentication
smtpd_sasl_type = dovecot
smtpd_sasl_path = private/auth
smtpd_sasl_auth_enable = yes
smtpd_tls_security_level = may
`

	if err := writeFileSecure("/etc/postfix/main.cf", postfixConfig, 0644); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to write Postfix config: "+err.Error())
	}

	// Configure Dovecot
	dovecotConfig := `# SigardaPanel Dovecot Configuration
protocols = imap pop3 lmtp
listen = *, ::

mail_location = maildir:/var/mail/vhosts/%d/%n
mail_privileged_group = mail

namespace inbox {
  inbox = yes
}

service auth {
  unix_listener /var/spool/postfix/private/auth {
    mode = 0660
    user = postfix
    group = postfix
  }
}

auth_mechanisms = plain login

passdb {
  driver = passwd-file
  args = /etc/dovecot/passwd
}

userdb {
  driver = static
  args = uid=5000 gid=5000 home=/var/mail/vhosts/%d/%n
}

ssl = required
ssl_cert = </etc/ssl/certs/ssl-cert-snakeoil.pem
ssl_key = </etc/ssl/private/ssl-cert-snakeoil.key
`

	if err := writeFileSecure("/etc/dovecot/dovecot.conf", dovecotConfig, 0644); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to write Dovecot config: "+err.Error())
	}

	// Create virtual mail user
	r.executor.RunShell(ctx, "useradd", "-r", "-u", "5000", "-g", "mail", "-d", "/var/mail/vhosts", "-s", "/sbin/nologin", "vmail")
	r.executor.RunShell(ctx, "mkdir", "-p", "/var/mail/vhosts")

	// Create empty domain/mailbox files
	writeFileSecure("/etc/postfix/vdomains", "", 0644)
	writeFileSecure("/etc/postfix/vmailbox", "", 0644)
	writeFileSecure("/etc/postfix/valias", "", 0644)
	writeFileSecure("/etc/dovecot/passwd", "", 0600)

	// Set permissions
	r.executor.RunShell(ctx, "chown", "-R", "vmail:mail", "/var/mail/vhosts")
	r.executor.RunShell(ctx, "chmod", "750", "/var/mail/vhosts")

	// Start services
	r.executor.RunShell(ctx, "systemctl", "enable", "postfix")
	r.executor.RunShell(ctx, "systemctl", "restart", "postfix")
	r.executor.RunShell(ctx, "systemctl", "enable", "dovecot")
	r.executor.RunShell(ctx, "systemctl", "restart", "dovecot")

	slog.Info("email: email server installed")

	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": "Email server installed (Postfix + Dovecot + Roundcube)",
	})
}

// Mail account files (variables so tests can point them elsewhere).
var (
	emailPasswdFile   = "/etc/dovecot/passwd"
	emailMailboxFile  = "/etc/postfix/vmailbox"
	emailDomainsFile  = "/etc/postfix/vdomains"
	emailMailboxRoot  = "/var/mail/vhosts"
	emailPostfixCheck = "/etc/postfix/main.cf"
)

var emailLocalRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._+-]{0,63}$`)

// parseEmail splits and validates a mailbox address (lower-cased). When
// domain is given, the address must belong to it.
func parseEmail(email, domain string) (local, dom string, err error) {
	email = strings.ToLower(strings.TrimSpace(email))
	at := strings.LastIndex(email, "@")
	if at <= 0 || at == len(email)-1 {
		return "", "", fmt.Errorf("invalid email address")
	}
	local, dom = email[:at], email[at+1:]
	if !emailLocalRe.MatchString(local) || strings.Contains(local, "..") || !isValidDomain(dom) {
		return "", "", fmt.Errorf("invalid email address")
	}
	if domain != "" && !strings.EqualFold(dom, domain) {
		return "", "", fmt.Errorf("the address must be in the domain %s", domain)
	}
	return local, dom, nil
}

// validMailPassword: any printable text of 8-128 characters without ':'
// (the passwd-file separator). It is passed to doveadm as one argv entry,
// never through a shell.
func validMailPassword(p string) error {
	if len(p) < 8 || len(p) > 128 {
		return fmt.Errorf("password must be 8-128 characters")
	}
	for _, r := range p {
		if r < 0x20 || r == 0x7f || r == ':' {
			return fmt.Errorf("password contains invalid characters")
		}
	}
	return nil
}

func (r *Router) mailHash(ctx context.Context, password string) (string, error) {
	out, err := r.executor.RunShell(ctx, "doveadm", "pw", "-s", "SHA512-CRYPT", "-p", password)
	hash := strings.TrimSpace(out)
	if err != nil || !strings.HasPrefix(hash, "{SHA512-CRYPT}") || strings.ContainsAny(hash, ":\n") {
		return "", fmt.Errorf("failed to generate password hash")
	}
	return hash, nil
}

// createEmailAccount creates a new email account.
func (r *Router) createEmailAccount(c *echo.Context, domain, email, password string, quota int, firstName, lastName string) error {
	ctx := c.Request().Context()
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		if _, d, err := parseEmail(email, ""); err == nil {
			domain = d
		}
	}
	if !isValidDomain(domain) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid domain")
	}
	local, _, err := parseEmail(email, domain)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	email = local + "@" + domain
	if err := validMailPassword(password); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if lines, err := readFileLines(emailPasswdFile); err == nil {
		for _, l := range lines {
			if strings.HasPrefix(l, email+":") {
				return echo.NewHTTPError(http.StatusConflict, "the account "+email+" already exists")
			}
		}
	}
	hash, err := r.mailHash(ctx, password)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if err := appendFileSecure(emailPasswdFile, email+":"+hash+"\n", 0600); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to add email account")
	}
	if err := appendFileSecure(emailMailboxFile, fmt.Sprintf("%s %s/%s/\n", email, domain, local), 0644); err != nil {
		_ = rewriteLines(emailPasswdFile, func(l string) (string, bool) { return l, !strings.HasPrefix(l, email+":") })
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to add mailbox")
	}
	if err := addDomainToPostfix(domain); err != nil {
		slog.Warn("email: failed to add domain", "error", err)
	}

	mailboxDir := filepath.Join(emailMailboxRoot, domain, local)
	r.executor.RunShell(ctx, "mkdir", "-p", mailboxDir)
	r.executor.RunShell(ctx, "chown", "-R", "vmail:mail", filepath.Join(emailMailboxRoot, domain))
	r.executor.RunShell(ctx, "postmap", emailMailboxFile)
	r.executor.RunShell(ctx, "postmap", emailDomainsFile)
	r.executor.RunShell(ctx, "systemctl", "reload", "postfix")
	r.executor.RunShell(ctx, "systemctl", "reload", "dovecot")

	slog.Info("email: account created", "email", email)
	return c.JSON(http.StatusOK, map[string]any{"ok": true, "message": fmt.Sprintf("Email account %s created", email)})
}

// deleteEmailAccount deletes an email account (exact address only; the
// mailbox directory is kept).
func (r *Router) deleteEmailAccount(c *echo.Context, email string) error {
	ctx := c.Request().Context()
	local, domain, err := parseEmail(email, "")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	email = local + "@" + domain
	found := false
	if err := rewriteLines(emailPasswdFile, func(l string) (string, bool) {
		if strings.HasPrefix(l, email+":") {
			found = true
			return "", false
		}
		return l, true
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to remove email account")
	}
	if !found {
		return echo.NewHTTPError(http.StatusNotFound, "no account "+email)
	}
	if err := rewriteLines(emailMailboxFile, func(l string) (string, bool) {
		return l, !strings.HasPrefix(l, email+" ")
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to remove mailbox")
	}
	r.executor.RunShell(ctx, "postmap", emailMailboxFile)
	r.executor.RunShell(ctx, "systemctl", "reload", "postfix")
	r.executor.RunShell(ctx, "systemctl", "reload", "dovecot")

	slog.Info("email: account deleted", "email", email)
	return c.JSON(http.StatusOK, map[string]any{"ok": true, "message": fmt.Sprintf("Email account %s deleted", email)})
}

// listEmailAccounts lists email accounts, optionally for one domain.
func (r *Router) listEmailAccounts(c *echo.Context, domain string) error {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain != "" && !isValidDomain(domain) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid domain")
	}
	lines, err := readFileLines(emailPasswdFile)
	if err != nil {
		if os.IsNotExist(err) {
			return c.JSON(http.StatusOK, map[string]any{"accounts": []map[string]string{}})
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to read accounts")
	}
	accounts := []map[string]string{}
	for _, line := range lines {
		if line == "" || line[0] == '#' {
			continue
		}
		email, _, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		_, d, err := parseEmail(email, "")
		if err != nil || (domain != "" && d != domain) {
			continue
		}
		accounts = append(accounts, map[string]string{"email": email, "domain": d})
	}
	return c.JSON(http.StatusOK, map[string]any{"accounts": accounts})
}

// updateEmailPassword updates an email account password.
func (r *Router) updateEmailPassword(c *echo.Context, email, password string) error {
	ctx := c.Request().Context()
	local, domain, err := parseEmail(email, "")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	email = local + "@" + domain
	if err := validMailPassword(password); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	hash, err := r.mailHash(ctx, password)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	found := false
	if err := rewriteLines(emailPasswdFile, func(l string) (string, bool) {
		if strings.HasPrefix(l, email+":") {
			found = true
			return email + ":" + hash, true
		}
		return l, true
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update password")
	}
	if !found {
		return echo.NewHTTPError(http.StatusNotFound, "no account "+email)
	}
	r.executor.RunShell(ctx, "systemctl", "reload", "dovecot")
	slog.Info("email: password updated", "email", email)
	return c.JSON(http.StatusOK, map[string]any{"ok": true, "message": fmt.Sprintf("Password updated for %s", email)})
}

// emailStatus returns email server status in the shape the web page reads.
func (r *Router) emailStatus(c *echo.Context) error {
	ctx := c.Request().Context()
	postfixStatus, _ := r.executor.RunShell(ctx, "systemctl", "is-active", "postfix")
	dovecotStatus, _ := r.executor.RunShell(ctx, "systemctl", "is-active", "dovecot")
	postfixStatus, dovecotStatus = strings.TrimSpace(postfixStatus), strings.TrimSpace(dovecotStatus)
	_, errMain := os.Stat(emailPostfixCheck)
	_, errPasswd := os.Stat(emailPasswdFile)
	return c.JSON(http.StatusOK, map[string]any{
		"installed":       errMain == nil && errPasswd == nil,
		"postfix_running": postfixStatus == "active",
		"dovecot_running": dovecotStatus == "active",
		"postfix":         postfixStatus,
		"dovecot":         dovecotStatus,
		"roundcube":       "available",
	})
}

// Helper functions

func writeFileSecure(path, content string, perm int) error {
	return os.WriteFile(path, []byte(content), os.FileMode(perm))
}

func appendFileSecure(path, content string, perm int) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, os.FileMode(perm))
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return err
}

// addDomainToPostfix lists domain in the virtual mailbox domains map.
func addDomainToPostfix(domain string) error {
	lines, err := readFileLines(emailDomainsFile)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, l := range lines {
		if f := strings.Fields(l); len(f) > 0 && f[0] == domain {
			return nil
		}
	}
	return appendFileSecure(emailDomainsFile, domain+" OK\n", 0644)
}

// rewriteLines rewrites a file line by line (keep=false drops the line),
// atomically and keeping the file's mode (the passwd file holds hashes).
func rewriteLines(path string, fn func(line string) (string, bool)) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	lines, err := readFileLines(path)
	if err != nil {
		return err
	}
	var out []string
	for _, l := range lines {
		if l == "" {
			continue
		}
		if nl, keep := fn(l); keep {
			out = append(out, nl)
		}
	}
	content := strings.Join(out, "\n")
	if content != "" {
		content += "\n"
	}
	tmp := path + ".sigarda-tmp"
	if err := os.WriteFile(tmp, []byte(content), info.Mode().Perm()); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readFileLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return strings.Split(string(data), "\n"), nil
}
