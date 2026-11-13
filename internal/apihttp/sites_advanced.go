package apihttp

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sigardapanel/internal/jobmodel"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/audit"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/nginx"
	"sigardapanel/internal/store"
)

const sshdConfigFile = "/etc/ssh/sshd_config.d/sigardapanel-ssh.conf"

var sshUsernameRegex = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,30}$`)

// runCmd is a simple helper to run a shell command
func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s failed: %s", name, strings.TrimSpace(string(out)))
	}
	return nil
}

// readFstab reads /etc/fstab and returns non-empty lines
func readFstab() ([]string, error) {
	b, err := os.ReadFile("/etc/fstab")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(b), "\n")
	var out []string
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out, nil
}

// writeFstab writes lines back to /etc/fstab
func writeFstab(lines []string) error {
	content := strings.Join(lines, "\n") + "\n"
	return os.WriteFile("/etc/fstab", []byte(content), 0644)
}

// sshUserExists checks if a Linux user already exists
func sshUserExists(username string) bool {
	return runCmd("id", "-u", username) == nil
}

// generateSSHConfig rebuilds the entire sshd Match config from all SSH users in the DB
func (h *siteHandler) generateSSHConfig(ctx echo.Context) error {
	users, err := h.store.SSH.ListAll(ctx.Request().Context())
	if err != nil {
		return err
	}
	var sb strings.Builder
	sb.WriteString("# Managed by SigardaPanel - do not edit manually\n")
	sb.WriteString("# ChrootDirectory jails for SSH/FTP users\n\n")
	for _, u := range users {
		if u.Username == "" || u.HomeDir == "" {
			continue
		}
		sb.WriteString(fmt.Sprintf("Match User %s\n", u.Username))
		sb.WriteString(fmt.Sprintf("    ChrootDirectory %s\n", u.HomeDir))
		sb.WriteString("    X11Forwarding no\n")
		sb.WriteString("    AllowTcpForwarding no\n")
		sb.WriteString("    PasswordAuthentication no\n")
		sb.WriteString("    AuthenticationMethods publickey\n")
		sb.WriteString("\n")
	}
	if err := os.MkdirAll("/etc/ssh/sshd_config.d", 0755); err != nil {
		return err
	}
	if err := os.WriteFile(sshdConfigFile, []byte(sb.String()), 0644); err != nil {
		return err
	}
	// Only reload if there are users configured
	if len(users) > 0 {
		if err := runCmd("systemctl", "reload", "ssh"); err != nil {
			// Non-fatal — config was written
			fmt.Printf("warning: sshd reload failed: %v\n", err)
		}
	}
	return nil
}

// provisionSSHUser creates the Linux user, generates SSH key pair, and sets up chroot jail
// sshPublicKey and sshPrivateKey are output parameters filled with generated keys.
func (h *siteHandler) provisionSSHUser(ctx echo.Context, username, password, sshKeys, homeDir, siteRoot string) (sshPublicKey, sshPrivateKey string, err error) {
	// 1. Check user doesn't already exist
	if sshUserExists(username) {
		return "", "", fmt.Errorf("user %s already exists on the system", username)
	}

	// 2. Create Linux user
	if err := runCmd("useradd", "-m", "-s", "/bin/bash", username); err != nil {
		return "", "", fmt.Errorf("create user: %w", err)
	}

	// 3. Lock password (no password auth — SSH keys only)
	_ = runCmd("passwd", "-l", username)

	// 4. Generate SSH key pair
	sshDir := filepath.Join(homeDir, ".ssh")
	_ = os.MkdirAll(sshDir, 0700)
	keyFile := filepath.Join(sshDir, "id_ed25519")

	// Generate ed25519 key (no passphrase)
	if err := runCmd("ssh-keygen", "-t", "ed25519", "-f", keyFile, "-N", "", "-C", username+"@sigardapanel"); err != nil {
		// Fallback to RSA 4096 if ed25519 not supported
		_ = runCmd("ssh-keygen", "-t", "rsa", "-b", "4096", "-f", keyFile, "-N", "", "-C", username+"@sigardapanel")
	}

	// Read generated keys
	if pkBytes, readErr := os.ReadFile(keyFile); readErr == nil {
		sshPrivateKey = string(pkBytes)
	}
	if pubBytes, readErr := os.ReadFile(keyFile + ".pub"); readErr == nil {
		sshPublicKey = string(pubBytes)
	}

	// Set authorized_keys
	if sshPublicKey != "" {
		_ = os.WriteFile(filepath.Join(sshDir, "authorized_keys"), []byte(strings.TrimSpace(sshPublicKey)+"\n"), 0600)
	}
	// Also add any user-provided SSH keys
	if sshKeys != "" {
		existing, _ := os.ReadFile(filepath.Join(sshDir, "authorized_keys"))
		// Split by newline and write each key on its own line
		for _, line := range strings.Split(strings.TrimSpace(sshKeys), "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				existing = append(existing, []byte(line+"\n")...)
			}
		}
		_ = os.WriteFile(filepath.Join(sshDir, "authorized_keys"), existing, 0600)
	}
	_ = runCmd("chown", "-R", username+":"+username, sshDir)

	// 5. Chroot setup: home dir must be owned by root for ChrootDirectory
	_ = runCmd("chown", "root:root", homeDir)
	_ = runCmd("chmod", "755", homeDir)

	// 6. Bind mount system directories (same as before)
	sysDirs := []string{"/bin", "/lib", "/lib64", "/usr"}
	for _, d := range sysDirs {
		dst := filepath.Join(homeDir, d)
		_ = os.MkdirAll(dst, 0755)
		_ = runCmd("mount", "--bind", d, dst)
		_ = runCmd("mount", "-o", "remount,bind,ro", dst)
		if lines, err := readFstab(); err == nil {
			fstabLine := fmt.Sprintf("%s %s none bind,ro 0 0  # sigardapanel:%s", d, dst, username)
			lines = append(lines, fstabLine)
			_ = writeFstab(lines)
		}
	}

	// 7. Create minimal /etc inside jail (same as before)
	etcDir := filepath.Join(homeDir, "etc")
	_ = os.MkdirAll(etcDir, 0755)
	uidOut, _ := exec.Command("id", "-u", username).CombinedOutput()
	gidOut, _ := exec.Command("id", "-g", username).CombinedOutput()
	uidStr := strings.TrimSpace(string(uidOut))
	gidStr := strings.TrimSpace(string(gidOut))
	_ = os.WriteFile(filepath.Join(etcDir, "passwd"),
		[]byte(fmt.Sprintf("root:x:0:0:root:/root:/sbin/nologin\n%s:x:%s:%s::/:/bin/bash\n", username, uidStr, gidStr)), 0644)
	_ = os.WriteFile(filepath.Join(etcDir, "group"),
		[]byte(fmt.Sprintf("root:x:0:\n%s:x:%s:\n", username, gidStr)), 0644)
	if _, err := os.Stat("/etc/resolv.conf"); err == nil {
		resolv, _ := os.ReadFile("/etc/resolv.conf")
		_ = os.WriteFile(filepath.Join(etcDir, "resolv.conf"), resolv, 0644)
	}

	// 8. Create mount point inside jail
	htdocsDir := filepath.Join(homeDir, "htdocs")
	if err := os.MkdirAll(htdocsDir, 0755); err != nil {
		return "", "", fmt.Errorf("create mount point: %w", err)
	}

	// 9. Bind mount the site root into the jail
	if siteRoot != "" {
		_ = runCmd("mount", "--bind", siteRoot, htdocsDir)
		_ = runCmd("chown", username+":"+username, htdocsDir)
		if lines, err := readFstab(); err == nil {
			fstabLine := fmt.Sprintf("%s %s none bind 0 0  # sigardapanel:%s", siteRoot, htdocsDir, username)
			lines = append(lines, fstabLine)
			if err := writeFstab(lines); err != nil {
				fmt.Printf("warning: cannot write fstab: %v\n", err)
			}
		}
	}

	return sshPublicKey, sshPrivateKey, nil
}

// deprovisionSSHUser removes the Linux user and cleans up
func (h *siteHandler) deprovisionSSHUser(ctx echo.Context, username, homeDir string) {
	if username == "" {
		return
	}

	// 1. Remove fstab entry
	htdocsDir := filepath.Join(homeDir, "htdocs")
	marker := fmt.Sprintf("# sigardapanel:%s", username)
	if lines, err := readFstab(); err == nil {
		var filtered []string
		for _, l := range lines {
			// Exact matches only: deleting "bo" must not drop "bob"'s mounts.
			if f := strings.Fields(l); strings.HasSuffix(strings.TrimSpace(l), marker) || (len(f) >= 2 && f[1] == htdocsDir) {
				continue
			}
			filtered = append(filtered, l)
		}
		_ = writeFstab(filtered)
	}

	// 2. Unmount bind mounts (site + system dirs)
	sysDirs := []string{"/usr", "/lib64", "/lib", "/bin"}
	for _, d := range sysDirs {
		_ = runCmd("umount", "-l", filepath.Join(homeDir, d))
	}
	_ = runCmd("umount", "-l", htdocsDir)

	// 3. Delete Linux user (home dir too)
	_ = runCmd("userdel", "-r", username)
}

// siteAgentCall calls the agent of the server that hosts site.
func (h *siteHandler) siteAgentCall(ctx context.Context, site *store.Site, method, path string, body any, timeout time.Duration) (int, map[string]any, error) {
	server, err := h.store.Servers.GetByID(ctx, site.ServerID)
	if err != nil || server == nil || !server.AgentURL.Valid || server.AgentURL.String == "" || server.AgentToken == "" {
		return 0, nil, fmt.Errorf("the site's server has no reachable agent")
	}
	return stackAgentRequest(ctx, method, agentBase(server)+path, server.AgentToken, body, timeout)
}

// vhostRootGuard: raw vhost editing can read and write any file through nginx
// directives, i.e. it is root on the site's server; on the panel host that is
// super_admin power.
func (h *siteHandler) vhostRootGuard(ctx context.Context, sess *store.Session, site *store.Site) error {
	server, _ := h.store.Servers.GetByID(ctx, site.ServerID)
	if server != nil && isLocalServer(*server) && !isSessionSuperAdmin(ctx, h.store, sess) {
		return echo.NewHTTPError(http.StatusForbidden, "editing raw vhosts on the panel host requires a super admin")
	}
	return nil
}

func (h *siteHandler) getVhost(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	site, err := h.store.Sites.GetByID(c.Request().Context(), id)
	if err != nil || site == nil || !h.canAccess(c.Request().Context(), sess, site) {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	// The live vhost is on the site's server; the DB copy is the fallback.
	config, source, exists, path := "", "db", false, ""
	code, out, err := h.siteAgentCall(c.Request().Context(), site, http.MethodGet, "/tasks/vhost/get?domain="+url.QueryEscape(site.Domain), nil, 15*time.Second)
	if err == nil && code == http.StatusOK {
		path, _ = out["path"].(string)
		if e, _ := out["exists"].(bool); e {
			config, _ = out["config"].(string)
			source, exists = "nginx", true
		}
	}
	if !exists {
		if vhost, dbErr := h.store.Vhosts.GetBySiteID(c.Request().Context(), id); dbErr == nil {
			config, exists = vhost.Config, true
		}
	}
	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{
		"site_id": id, "domain": site.Domain, "vhost_config": config,
		"source": source, "exists": exists, "config_path": path,
	}, RequestID: rid})
}

func (h *siteHandler) updateVhost(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	ctx := c.Request().Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	site, err := h.store.Sites.GetByID(ctx, id)
	if err != nil || site == nil || !h.checkSitePermission(ctx, sess, site, "write") {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}
	if err := h.vhostRootGuard(ctx, sess, site); err != nil {
		return err
	}
	if site.Status == "suspended" {
		return echo.NewHTTPError(http.StatusConflict, "the site is suspended; resume it first")
	}
	var body struct {
		Config string `json:"vhost_config"`
	}
	if err := c.Bind(&body); err != nil || strings.TrimSpace(body.Config) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "vhost_config required")
	}

	code, out, err := h.siteAgentCall(ctx, site, http.MethodPost, "/tasks/vhost/put", map[string]any{"domain": site.Domain, "config": body.Config}, 2*time.Minute)
	if err != nil || code >= 400 {
		msg := "the site's agent is unreachable"
		if err == nil {
			msg = agentMessage(out, "agent error")
		}
		return echo.NewHTTPError(http.StatusBadGateway, "vhost not saved: "+msg)
	}
	if status, _ := out["status"].(string); status != "success" {
		nginxErr, _ := out["nginx_error"].(string)
		return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{
			"status": "saved_nginx_invalid", "message": "nginx -t failed, previous config kept. Check syntax:\n" + nginxErr,
			"nginx_error": nginxErr, "config_path": out["path"], "reload_attempt": false,
		}, RequestID: rid})
	}
	// Only a vhost nginx accepted is kept as the stored copy.
	if err := h.store.Vhosts.Upsert(ctx, id, body.Config); err != nil {
		slog.Warn("vhost: store copy failed", "site_id", id, "error", err)
	}
	h.store.NotifySiteUsers(ctx, id, "vhost_updated", "Vhost Updated", "Nginx vhost configuration was updated", "info", "site", id)
	if sess != nil {
		h.audit.RecordAction(ctx, sess.UserID, audit.ActionSiteUpdate, audit.ResourceSite, id, map[string]any{"domain": site.Domain, "section": "vhost"})
	}
	reloadOK, _ := out["reload_ok"].(bool)
	msg := "Nginx config saved"
	if reloadOK {
		msg += " and nginx reloaded"
	} else {
		msg += " (nginx reload failed: " + fmt.Sprint(out["reload_output"]) + ")"
	}
	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{
		"status": "success", "message": msg, "config_path": out["path"], "reload_attempt": true, "reload_ok": reloadOK,
	}, RequestID: rid})
}

// validateVhost runs nginx -t with the candidate vhost on the site's server.
func (h *siteHandler) validateVhost(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	ctx := c.Request().Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	site, err := h.store.Sites.GetByID(ctx, id)
	// Validation briefly installs the candidate: same rights as saving.
	if err != nil || site == nil || !h.checkSitePermission(ctx, sess, site, "write") {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}
	if err := h.vhostRootGuard(ctx, sess, site); err != nil {
		return err
	}
	var body struct {
		Config string `json:"vhost_config"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}
	code, out, err := h.siteAgentCall(ctx, site, http.MethodPost, "/tasks/vhost/validate", map[string]any{"domain": site.Domain, "config": body.Config}, time.Minute)
	if err != nil || code >= 400 {
		return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{
			"valid": false, "warning": true, "message": "Could not reach the site's agent to validate the config.",
		}, RequestID: rid})
	}
	valid, _ := out["valid"].(bool)
	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{
		"valid": valid, "warning": false, "message": out["message"],
	}, RequestID: rid})
}

func (h *siteHandler) getVarnish(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	site, err := h.store.Sites.GetByID(c.Request().Context(), id)
	if err != nil || site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	// Access check: user must be admin, owner, or assigned to this site
	if !h.canAccess(c.Request().Context(), sess, site) {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	v, err := h.store.Varnish.GetBySiteID(c.Request().Context(), id)
	if err == sql.ErrNoRows {
		return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{"enabled": false, "cache_time": 3600, "hit_rate": "0%"}, RequestID: rid})
	}
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{"enabled": v.Enabled, "cache_time": v.CacheTime, "hit_rate": v.HitRate}, RequestID: rid})
}

func (h *siteHandler) purgeVarnish(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	site, err := h.store.Sites.GetByID(c.Request().Context(), id)
	if err != nil || site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	// Permission check: admin, owner, or user with admin permission on this site
	if !h.checkSitePermission(c.Request().Context(), sess, site, "admin") {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{"status": "success", "message": "Cache purged"}, RequestID: rid})
}

func (h *siteHandler) getSecurity(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	site, err := h.store.Sites.GetByID(c.Request().Context(), id)
	if err != nil || site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	// Access check: user must be admin, owner, or assigned to this site
	if !h.canAccess(c.Request().Context(), sess, site) {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	sec, err := h.store.Security.GetBySiteID(c.Request().Context(), id)
	if err == sql.ErrNoRows {
		return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{"basic_auth_enabled": false, "basic_auth_username": "", "blocked_ips": []string{}}, RequestID: rid})
	}
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{"basic_auth_enabled": sec.BasicAuthEnabled, "basic_auth_username": sec.BasicAuthUsername, "blocked_ips": sec.BlockedIPs}, RequestID: rid})
}

func (h *siteHandler) updateSecurity(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	var userID int64
	if sess != nil {
		userID = sess.UserID
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	site, err := h.store.Sites.GetByID(c.Request().Context(), id)
	if err != nil || site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	// Permission check: admin, owner, or user with write+ permission on this site
	if !h.checkSitePermission(c.Request().Context(), sess, site, "write") {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	var body struct {
		BasicAuthEnabled  bool     `json:"basic_auth_enabled"`
		BasicAuthUsername string   `json:"basic_auth_username"`
		BasicAuthPassword string   `json:"basic_auth_password"`
		BlockedIPs        []string `json:"blocked_ips"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}
	ctx := c.Request().Context()
	if site.Status == "suspended" {
		return echo.NewHTTPError(http.StatusConflict, "the site is suspended; resume it first")
	}
	// Every value ends up in the nginx vhost: only plain IPs/CIDRs and a
	// simple username are accepted.
	blocked, err := nginx.NormalizeBlockedIPs(body.BlockedIPs)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	body.BasicAuthUsername = strings.TrimSpace(body.BasicAuthUsername)
	if body.BasicAuthEnabled {
		if !nginx.ValidBasicAuthUsername(body.BasicAuthUsername) {
			return echo.NewHTTPError(http.StatusBadRequest, "basic auth username must be 1-64 characters of A-Z a-z 0-9 . _ -")
		}
		if len(body.BasicAuthPassword) > 72 {
			return echo.NewHTTPError(http.StatusBadRequest, "basic auth password must be at most 72 characters")
		}
		if body.BasicAuthPassword == "" {
			existing, _ := h.store.Security.GetBySiteID(ctx, id)
			if existing == nil || existing.BasicAuthPassword == "" {
				return echo.NewHTTPError(http.StatusBadRequest, "set a basic auth password")
			}
		} else if len(body.BasicAuthPassword) < 6 {
			return echo.NewHTTPError(http.StatusBadRequest, "basic auth password must be at least 6 characters")
		}
	}

	sec := &store.SiteSecurity{
		SiteID:            id,
		BasicAuthEnabled:  body.BasicAuthEnabled,
		BasicAuthUsername: body.BasicAuthUsername,
		BasicAuthPassword: body.BasicAuthPassword,
		BlockedIPs:        blocked,
	}
	if err := h.store.Security.Upsert(ctx, sec); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	// The site's own agent regenerates the vhost (with these settings, which
	// the worker adds to every site.create payload) on the server that hosts
	// the site, tests it with nginx -t and keeps the old vhost on failure.
	job, err := h.enqueueVhostRegen(ctx, site, "Apply security settings for "+site.Domain)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to enqueue the vhost update")
	}

	h.store.NotifySiteUsers(ctx, id, "security_changed", "Security Updated", "Site security settings were changed", "warning", "site", id)

	h.audit.RecordAction(c.Request().Context(), userID, audit.ActionSiteUpdate, audit.ResourceSite, id, map[string]any{
		"domain":  site.Domain,
		"section": "security",
	})

	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{"status": "queued", "message": "Security settings saved; applying them to the site", "job_id": job.ID}, RequestID: rid})
}

func (h *siteHandler) getSshFtp(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	site, err := h.store.Sites.GetByID(c.Request().Context(), id)
	if err != nil || site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}
	if err := requireSitePermission(c.Request().Context(), h.store, sessionFromEcho(c), site, sitePermRead); err != nil {
		return err
	}

	users, err := h.store.SSH.ListBySiteID(c.Request().Context(), id)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if users == nil {
		users = []store.SiteSSHUser{}
	}
	// Private keys are only served by the dedicated (write-guarded) key
	// endpoint, never in the listing.
	for i := range users {
		users[i].SSHPrivateKey = ""
		users[i].Password = ""
	}

	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{"users": users}, RequestID: rid})
}

func (h *siteHandler) createSshFtp(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	sess := sessionFromEcho(c)
	var userID int64
	if sess != nil {
		userID = sess.UserID
	}

	site, err := h.store.Sites.GetByID(c.Request().Context(), id)
	if err != nil || site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}
	if err := requireSitePermission(c.Request().Context(), h.store, sess, site, sitePermWrite); err != nil {
		return err
	}

	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		SSHKeys  string `json:"ssh_keys"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}
	if body.Username == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "username required")
	}
	if !sshUsernameRegex.MatchString(body.Username) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid username (lowercase, alphanumeric, _, -, 1-31 chars)")
	}

	homeDir := "/home/" + body.Username

	// Determine site root for bind mount
	siteRoot := site.RootPath
	if siteRoot == "" && site.SystemUser.Valid && site.Domain != "" {
		siteRoot = "/home/" + site.SystemUser.String + "/htdocs/" + site.Domain
	}

	// Provision Linux user with chroot — generates SSH key pair
	pubKey, privKey, err := h.provisionSSHUser(c, body.Username, body.Password, body.SSHKeys, homeDir, siteRoot)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "user provisioning failed: "+err.Error())
	}

	// Save to database
	user := &store.SiteSSHUser{
		SiteID:        id,
		Username:      body.Username,
		Password:      "", // password auth disabled
		SSHKeys:       body.SSHKeys,
		SSHPrivateKey: privKey,
		SSHPublicKey:  pubKey,
	}
	user.HomeDir = homeDir
	if err := h.store.SSH.Create(c.Request().Context(), user); err != nil {
		// Clean up the Linux user since DB save failed
		h.deprovisionSSHUser(c, body.Username, homeDir)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	// Regenerate sshd config (now includes the new user from DB)
	if err := h.generateSSHConfig(c); err != nil {
		fmt.Printf("warning: generateSSHConfig failed: %v\n", err)
	}

	h.audit.RecordAction(c.Request().Context(), userID, "ssh_user_create", audit.ResourceSite, id, map[string]any{
		"site_id":  id,
		"username": body.Username,
	})

	// The generated key pair is shown once (the UI displays it right after
	// creation; it used to read these fields from a response without them).
	return c.JSON(http.StatusCreated, httpjson.Success{Data: map[string]any{
		"status": "success", "message": "SSH/FTP user created",
		"username": body.Username, "ssh_public_key": pubKey, "ssh_private_key": privKey,
	}, RequestID: rid})
}

func (h *siteHandler) deleteSshFtp(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	var userID int64
	if sess != nil {
		userID = sess.UserID
	}
	sshID, err := strconv.ParseInt(c.Param("ssh_id"), 10, 64)
	if err != nil || sshID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid user id")
	}

	// The SSH user must belong to the site in the URL, and the caller needs
	// write permission on that site.
	user, err := h.siteSSHUser(c, sshID, sitePermWrite)
	if err != nil {
		return err
	}

	// Deprovision Linux user (unmount, delete user, cleanup)
	h.deprovisionSSHUser(c, user.Username, user.HomeDir)

	// Remove from database
	if err := h.store.SSH.Delete(c.Request().Context(), user.SiteID, sshID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "SSH user not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	// Regenerate sshd config
	if err := h.generateSSHConfig(c); err != nil {
		fmt.Printf("warning: generateSSHConfig failed: %v\n", err)
	}

	h.audit.RecordAction(c.Request().Context(), userID, "ssh_user_delete", audit.ResourceSite, user.SiteID, map[string]any{
		"site_id":  user.SiteID,
		"username": user.Username,
	})

	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{"status": "success", "message": "SSH/FTP user deleted"}, RequestID: rid})
}

// siteSSHUser loads SSH/FTP user sshID, verifying it belongs to the site in
// the :id URL parameter and that the caller holds the required permission on
// that site. Users of other sites are reported as not found.
func (h *siteHandler) siteSSHUser(c echo.Context, sshID int64, required string) (*store.SiteSSHUser, error) {
	ctx := c.Request().Context()
	siteID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || siteID <= 0 {
		return nil, echo.NewHTTPError(http.StatusBadRequest, "invalid site id")
	}
	site, err := h.store.Sites.GetByID(ctx, siteID)
	if err != nil || site == nil {
		return nil, echo.NewHTTPError(http.StatusNotFound, "site not found")
	}
	if err := requireSitePermission(ctx, h.store, sessionFromEcho(c), site, required); err != nil {
		return nil, err
	}
	user, err := h.store.SSH.GetByID(ctx, sshID)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return nil, echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if user == nil || user.SiteID != siteID {
		return nil, echo.NewHTTPError(http.StatusNotFound, "SSH user not found")
	}
	return user, nil
}

// getSSHPrivateKey returns the SSH private key for a given SSH/FTP user.
// GET /api/v1/sites/:id/ssh-ftp/:ssh_id/key
func (h *siteHandler) getSSHPrivateKey(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sshID, err := strconv.ParseInt(c.Param("ssh_id"), 10, 64)
	if err != nil || sshID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid user id")
	}

	// Downloading a private key grants shell access to the site: needs write
	// permission (or ownership) on the site the key belongs to.
	user, err := h.siteSSHUser(c, sshID, sitePermWrite)
	if err != nil {
		return err
	}

	if user.SSHPrivateKey == "" {
		return echo.NewHTTPError(http.StatusNotFound, "no private key found for this user")
	}

	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{
		"private_key": user.SSHPrivateKey,
		"public_key":  user.SSHPublicKey,
		"username":    user.Username,
	}, RequestID: rid})
}

func (h *siteHandler) listCron(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	site, err := h.store.Sites.GetByID(c.Request().Context(), id)
	if err != nil || site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	if !h.canAccess(c.Request().Context(), sess, site) {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	jobs, err := h.store.Cron.ListBySiteID(c.Request().Context(), id)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if jobs == nil {
		jobs = []store.SiteCronJob{}
	}

	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{"jobs": jobs}, RequestID: rid})
}

func (h *siteHandler) createCron(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	var userID int64
	if sess != nil {
		userID = sess.UserID
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	site, err := h.store.Sites.GetByID(c.Request().Context(), id)
	if err != nil || site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	if !h.checkSitePermission(c.Request().Context(), sess, site, "write") {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	var body struct {
		Schedule string `json:"schedule"`
		Command  string `json:"command"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}
	if body.Command == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "command required")
	}
	if body.Schedule == "" {
		body.Schedule = "* * * * *"
	}

	job := &store.SiteCronJob{SiteID: id, Schedule: body.Schedule, Command: body.Command}
	if err := h.store.Cron.Create(c.Request().Context(), job); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), userID, "cron_create", audit.ResourceSite, id, map[string]any{
		"site_id":  id,
		"schedule": body.Schedule,
		"command":  body.Command,
	})

	return c.JSON(http.StatusCreated, httpjson.Success{Data: map[string]any{"status": "success", "message": "Cron job created"}, RequestID: rid})
}

func (h *siteHandler) deleteCron(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	var userID int64
	if sess != nil {
		userID = sess.UserID
	}
	cronID, err := strconv.ParseInt(c.Param("cron_id"), 10, 64)
	if err != nil || cronID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid cron id")
	}

	// Get cron job to find site_id for access check
	siteID, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if siteID <= 0 {
		// Fallback: look up by cron job
		return echo.NewHTTPError(http.StatusBadRequest, "invalid site id")
	}

	site, err := h.store.Sites.GetByID(c.Request().Context(), siteID)
	if err != nil || site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	if !h.checkSitePermission(c.Request().Context(), sess, site, "write") {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	// Scoped to the site in the URL: a cron job of another site is not found.
	if err := h.store.Cron.Delete(c.Request().Context(), siteID, cronID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "cron job not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), userID, "cron_delete", audit.ResourceSite, siteID, map[string]any{
		"site_id": siteID,
		"cron_id": cronID,
	})

	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{"status": "success", "message": "Cron job deleted"}, RequestID: rid})
}

// enqueueVhostRegen queues a site.create job that only regenerates the
// site's vhost on its server. The worker fills the payload with the site's
// stored settings (runtime, PHP, app port, proxy URL, security).
func (h *siteHandler) enqueueVhostRegen(ctx context.Context, site *store.Site, desc string) (*jobmodel.Job, error) {
	payload, _ := json.Marshal(map[string]any{
		"site_id":     site.ID,
		"name":        site.Name,
		"domain":      site.Domain,
		"runtime":     site.Runtime,
		"root_path":   site.RootPath,
		"vhost_regen": true,
	})
	job := &jobmodel.Job{
		ServerID:    sql.NullInt64{Int64: site.ServerID, Valid: true},
		SiteID:      sql.NullInt64{Int64: site.ID, Valid: true},
		Type:        jobmodel.TypeSiteCreate,
		Description: desc,
		Payload:     string(payload),
	}
	if err := h.store.Jobs.Create(ctx, job); err != nil {
		return nil, err
	}
	return job, nil
}
