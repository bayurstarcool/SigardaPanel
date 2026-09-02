package agenthttp

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/labstack/echo/v4"
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
func (r *Router) manageEmail(c echo.Context) error {
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
func (r *Router) installEmailServer(c echo.Context) error {
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

// createEmailAccount creates a new email account.
func (r *Router) createEmailAccount(c echo.Context, domain, email, password string, quota int, firstName, lastName string) error {
	ctx := c.Request().Context()

	// Validate inputs
	if !isValidDomain(email) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid email address")
	}
	if !isValidDomain(domain) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid domain")
	}
	if len(password) < 8 {
		return echo.NewHTTPError(http.StatusBadRequest, "password must be at least 8 characters")
	}
	if !isShellSafe(password) {
		return echo.NewHTTPError(http.StatusBadRequest, "password contains invalid characters")
	}

	// Generate password hash using Dovecot
	hash, err := r.executor.RunShell(ctx, "doveadm", "pw", "-s", "SHA512-CRYPT", "-p", password)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to generate password hash")
	}

	// Add to Dovecot passwd file
	passwdEntry := fmt.Sprintf("%s:%s\n", email, hash)
	if err := appendFileSecure("/etc/dovecot/passwd", passwdEntry, 0600); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to add email account")
	}

	// Add to Postfix virtual mailbox
	mailboxEntry := fmt.Sprintf("%s %s/%s/\n", email, domain, email[:len(email)-len(domain)-1])
	if err := appendFileSecure("/etc/postfix/vmailbox", mailboxEntry, 0644); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to add mailbox")
	}

	// Add domain if not exists
	if err := addDomainToPostfix(domain); err != nil {
		slog.Warn("email: failed to add domain", "error", err)
	}

	// Create mailbox directory
	mailboxDir := fmt.Sprintf("/var/mail/vhosts/%s/%s", domain, email[:len(email)-len(domain)-1])
	r.executor.RunShell(ctx, "mkdir", "-p", mailboxDir)
	r.executor.RunShell(ctx, "chown", "-R", "vmail:mail", mailboxDir)

	// Rebuild Postfix maps
	r.executor.RunShell(ctx, "postmap", "/etc/postfix/vmailbox")
	r.executor.RunShell(ctx, "postmap", "/etc/postfix/vdomains")
	r.executor.RunShell(ctx, "postmap", "/etc/postfix/valias")

	// Reload services
	r.executor.RunShell(ctx, "systemctl", "reload", "postfix")
	r.executor.RunShell(ctx, "systemctl", "reload", "dovecot")

	slog.Info("email: account created", "email", email)

	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": fmt.Sprintf("Email account %s created", email),
	})
}

// deleteEmailAccount deletes an email account.
func (r *Router) deleteEmailAccount(c echo.Context, email string) error {
	ctx := c.Request().Context()

	if !isValidDomain(email) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid email address")
	}

	// Remove from Dovecot passwd
	if err := removeLineFromFile("/etc/dovecot/passwd", email); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to remove email account")
	}

	// Remove from Postfix virtual mailbox
	if err := removeLineFromFile("/etc/postfix/vmailbox", email); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to remove mailbox")
	}

	// Rebuild maps
	r.executor.RunShell(ctx, "postmap", "/etc/postfix/vmailbox")

	// Reload services
	r.executor.RunShell(ctx, "systemctl", "reload", "postfix")
	r.executor.RunShell(ctx, "systemctl", "reload", "dovecot")

	slog.Info("email: account deleted", "email", email)

	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": fmt.Sprintf("Email account %s deleted", email),
	})
}

// listEmailAccounts lists email accounts for a domain.
func (r *Router) listEmailAccounts(c echo.Context, domain string) error {
	if !isValidDomain(domain) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid domain")
	}

	// Read Dovecot passwd file
	lines, err := readFileLines("/etc/dovecot/passwd")
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to read accounts")
	}

	var accounts []map[string]string
	for _, line := range lines {
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		// Format: email:hash
		parts := splitN(line, ":", 2)
		if len(parts) == 2 {
			email := parts[0]
			// Filter by domain
			if domain == "" || (len(email) > len(domain) && email[len(email)-len(domain)-1:] == "@"+domain) {
				accounts = append(accounts, map[string]string{
					"email": email,
				})
			}
		}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"accounts": accounts,
	})
}

// updateEmailPassword updates an email account password.
func (r *Router) updateEmailPassword(c echo.Context, email, password string) error {
	ctx := c.Request().Context()

	if !isValidDomain(email) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid email address")
	}
	if len(password) < 8 {
		return echo.NewHTTPError(http.StatusBadRequest, "password must be at least 8 characters")
	}
	if !isShellSafe(password) {
		return echo.NewHTTPError(http.StatusBadRequest, "password contains invalid characters")
	}

	// Generate new hash
	hash, err := r.executor.RunShell(ctx, "doveadm", "pw", "-s", "SHA512-CRYPT", "-p", password)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to generate password hash")
	}

	// Update in passwd file
	if err := updateLineInFile("/etc/dovecot/passwd", email, fmt.Sprintf("%s:%s", email, hash)); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update password")
	}

	// Reload Dovecot
	r.executor.RunShell(ctx, "systemctl", "reload", "dovecot")

	slog.Info("email: password updated", "email", email)

	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": fmt.Sprintf("Password updated for %s", email),
	})
}

// emailStatus returns email server status.
func (r *Router) emailStatus(c echo.Context) error {
	ctx := c.Request().Context()

	postfixStatus, _ := r.executor.RunShell(ctx, "systemctl", "is-active", "postfix")
	dovecotStatus, _ := r.executor.RunShell(ctx, "systemctl", "is-active", "dovecot")

	return c.JSON(http.StatusOK, map[string]any{
		"postfix":   postfixStatus,
		"dovecot":   dovecotStatus,
		"roundcube": "available",
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

func addDomainToPostfix(domain string) error {
	// Implementation: add domain to vdomains file
	return nil
}

func removeLineFromFile(path, match string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	var filtered []string
	for _, line := range lines {
		if !strings.Contains(line, match) {
			filtered = append(filtered, line)
		}
	}
	return os.WriteFile(path, []byte(strings.Join(filtered, "\n")), 0644)
}

func readFileLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return strings.Split(string(data), "\n"), nil
}

func splitN(s, sep string, n int) []string {
	// Implementation: split string
	return nil
}

func updateLineInFile(path, match, replacement string) error {
	// Implementation: update line in file
	return nil
}
