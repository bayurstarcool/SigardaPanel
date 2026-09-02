package apihttp

import (
	"bytes"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

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
			if strings.Contains(l, marker) || strings.Contains(l, htdocsDir) {
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

func (h *siteHandler) getVhost(c echo.Context) error {
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

	nginxConfDir := "/etc/nginx/sites-enabled"
	nginxConfigPath := filepath.Join(nginxConfDir, site.Domain+".conf")

	// Try reading from live nginx config first
	liveConfig, readErr := os.ReadFile(nginxConfigPath)
	var config string
	var source string = "db"
	var exists bool = false

	if readErr == nil {
		config = string(liveConfig)
		source = "nginx"
		exists = true
	} else {
		// Fall back to DB
		vhost, dbErr := h.store.Vhosts.GetBySiteID(c.Request().Context(), id)
		if dbErr == nil {
			config = vhost.Config
			exists = true
		}
	}

	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{
		"site_id":      id,
		"domain":       site.Domain,
		"vhost_config": config,
		"source":       source,
		"exists":       exists,
		"config_path":  nginxConfigPath,
	}, RequestID: rid})
}

func (h *siteHandler) updateVhost(c echo.Context) error {
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

	// Permission check: admin, owner, or user with write+ permission on this site
	if !h.checkSitePermission(c.Request().Context(), sess, site, "write") {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	var body struct {
		Config string `json:"vhost_config"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}

	// 1. Save to DB as backup
	if err := h.store.Vhosts.Upsert(c.Request().Context(), id, body.Config); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.store.NotifySiteUsers(c.Request().Context(), id, "vhost_updated", "Vhost Updated", "Nginx vhost configuration was updated", "info", "site", id)

	// 2. Check if nginx is available and write to nginx conf dir
	nginxPath, nginxErr := exec.LookPath("nginx")
	nginxConfDir := "/etc/nginx/sites-enabled"
	nginxConfAvailableDir := "/etc/nginx/sites-available"

	if nginxErr == nil {
		// Ensure dirs exist
		os.MkdirAll(nginxConfDir, 0755)
		os.MkdirAll(nginxConfAvailableDir, 0755)

		// Write config BOTH to sites-available AND sites-enabled
		nginxConfigPath := filepath.Join(nginxConfDir, site.Domain+".conf")
		nginxAvailablePath := filepath.Join(nginxConfAvailableDir, site.Domain+".conf")

		// Write to sites-enabled directly
		if writeErr := os.WriteFile(nginxConfigPath, []byte(body.Config), 0644); writeErr != nil {
			return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{
				"status":  "saved_db_only",
				"message": "Config saved to DB, but failed to write nginx config: " + writeErr.Error(),
				"db_only": true,
			}, RequestID: rid})
		}

		// Also write to sites-available (for proper management)
		os.WriteFile(nginxAvailablePath, []byte(body.Config), 0644)

		// Run nginx -t
		cmd := exec.Command(nginxPath, "-t")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{
				"status":         "saved_nginx_invalid",
				"message":        "Config saved but nginx -t failed. Check syntax:\n" + stderr.String(),
				"nginx_error":    stderr.String(),
				"config_path":    nginxConfigPath,
				"reload_attempt": false,
			}, RequestID: rid})
		}

		// Try reload nginx
		reloadCmd := exec.Command("systemctl", "reload", "nginx")
		var reloadOut bytes.Buffer
		reloadCmd.Stderr = &reloadOut
		reloadErr := reloadCmd.Run()
		reloadOK := reloadErr == nil

		msg := "✅ Nginx config written to " + nginxConfigPath
		if reloadOK {
			msg += " and nginx reloaded"
		} else {
			msg += " (nginx reload skipped: " + strings.TrimSpace(reloadOut.String()) + ")"
		}

		return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{
			"status":         "success",
			"message":        msg,
			"config_path":    nginxConfigPath,
			"reload_attempt": true,
			"reload_ok":      reloadOK,
		}, RequestID: rid})
	}

	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{
		"status":  "saved_db_only",
		"message": "Config saved to DB. nginx not available on this server.",
		"db_only": true,
	}, RequestID: rid})
}

// validateVhost validates nginx config by writing to temp file and running nginx -t
func (h *siteHandler) validateVhost(c echo.Context) error {
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

	var body struct {
		Config string `json:"vhost_config"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}

	nginxPath, err := exec.LookPath("nginx")
	if err != nil {
		return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{
			"valid":   false,
			"warning": true,
			"message": "nginx binary not found on this server. Config will be saved to DB.",
		}, RequestID: rid})
	}

	// Strategy: temporarily SWAP the live config with the new one, run nginx -t (which has full
	// system context — modules, log_formats, SSL certs), then restore. No domain rewriting needed.
	livePath := filepath.Join("/etc/nginx/sites-enabled", site.Domain+".conf")
	backupPath := livePath + ".sigarda-bak"
	hadOriginal := false
	if _, err := os.Stat(livePath); err == nil {
		if err := os.Rename(livePath, backupPath); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to backup live config: "+err.Error())
		}
		hadOriginal = true
	}
	// Write new config in place of the live one
	if err := os.WriteFile(livePath, []byte(body.Config), 0644); err != nil {
		if hadOriginal {
			os.Rename(backupPath, livePath)
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to write test config: "+err.Error())
	}
	// Always restore original after test
	defer func() {
		os.Remove(livePath)
		if hadOriginal {
			os.Rename(backupPath, livePath)
		}
	}()

	// Run real nginx -t
	cmd := exec.Command(nginxPath, "-t")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Filter: keep only emerg/error lines + test result; strip warn noise from other sites
		raw := stderr.String()
		var kept []string
		for _, l := range strings.Split(raw, "\n") {
			if strings.Contains(l, "[emerg]") || strings.Contains(l, "[error]") ||
				strings.Contains(l, "test failed") || strings.Contains(l, "syntax is") {
				kept = append(kept, l)
			}
		}
		msg := strings.Join(kept, "\n")
		if msg == "" {
			msg = raw
		}
		return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{
			"valid":   false,
			"warning": false,
			"message": msg,
		}, RequestID: rid})
	}

	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{
		"valid":   true,
		"warning": false,
		"message": "nginx configuration syntax is valid ✓",
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
	if body.BlockedIPs == nil {
		body.BlockedIPs = []string{}
	}

	sec := &store.SiteSecurity{
		SiteID:            id,
		BasicAuthEnabled:  body.BasicAuthEnabled,
		BasicAuthUsername: body.BasicAuthUsername,
		BasicAuthPassword: body.BasicAuthPassword,
		BlockedIPs:        body.BlockedIPs,
	}
	if err := h.store.Security.Upsert(c.Request().Context(), sec); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.store.NotifySiteUsers(c.Request().Context(), id, "security_changed", "Security Updated", "Site security settings were changed", "warning", "site", id)

	// Write or remove htpasswd file based on basic auth state
	if body.BasicAuthEnabled && body.BasicAuthUsername != "" && body.BasicAuthPassword != "" {
		// Re-read to get the hashed password
		stored, err := h.store.Security.GetBySiteID(c.Request().Context(), id)
		if err == nil && stored.BasicAuthPassword != "" {
			nginx.WriteHtpasswd(site.Domain, stored.BasicAuthUsername, stored.BasicAuthPassword)
		}
	} else {
		nginx.RemoveHtpasswd(site.Domain)
	}

	// Regenerate nginx vhost with security settings
	if nginxPath, err := exec.LookPath("nginx"); err == nil {
		var port int
		if site.AppPort.Valid {
			port = int(site.AppPort.Int64)
		}
		sec, _ := h.store.Security.GetBySiteID(c.Request().Context(), id)
		if vhostCfg, err := nginx.GenerateVhost(site, port, sec); err == nil {
			nginxConfDir := "/etc/nginx/sites-enabled"
			nginxAvailableDir := "/etc/nginx/sites-available"
			os.MkdirAll(nginxConfDir, 0755)
			os.MkdirAll(nginxAvailableDir, 0755)
			enabledPath := filepath.Join(nginxConfDir, site.Domain+".conf")
			availablePath := filepath.Join(nginxAvailableDir, site.Domain+".conf")
			os.WriteFile(enabledPath, []byte(vhostCfg), 0644)
			os.WriteFile(availablePath, []byte(vhostCfg), 0644)
			// Validate and reload
			cmd := exec.Command(nginxPath, "-t")
			if err := cmd.Run(); err == nil {
				exec.Command("systemctl", "reload", "nginx").Run()
			}
		}
	}

	h.audit.RecordAction(c.Request().Context(), userID, audit.ActionSiteUpdate, audit.ResourceSite, id, map[string]any{
		"domain":  site.Domain,
		"section": "security",
	})

	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{"status": "success", "message": "Security updated"}, RequestID: rid})
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

	users, err := h.store.SSH.ListBySiteID(c.Request().Context(), id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if users == nil {
		users = []store.SiteSSHUser{}
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

	return c.JSON(http.StatusCreated, httpjson.Success{Data: map[string]any{"status": "success", "message": "SSH/FTP user created"}, RequestID: rid})
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

	// Get user info before deleting
	user, err := h.store.SSH.GetByID(c.Request().Context(), sshID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if user == nil {
		return echo.NewHTTPError(http.StatusNotFound, "SSH user not found")
	}

	// Deprovision Linux user (unmount, delete user, cleanup)
	h.deprovisionSSHUser(c, user.Username, user.HomeDir)

	// Remove from database
	if err := h.store.SSH.Delete(c.Request().Context(), sshID); err != nil {
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

// getSSHPrivateKey returns the SSH private key for a given SSH/FTP user.
// GET /api/v1/sites/:id/ssh-ftp/:ssh_id/key
func (h *siteHandler) getSSHPrivateKey(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sshID, err := strconv.ParseInt(c.Param("ssh_id"), 10, 64)
	if err != nil || sshID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid user id")
	}

	user, err := h.store.SSH.GetByID(c.Request().Context(), sshID)
	if err != nil || user == nil {
		return echo.NewHTTPError(http.StatusNotFound, "SSH user not found")
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

	if err := h.store.Cron.Delete(c.Request().Context(), cronID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), userID, "cron_delete", audit.ResourceSite, siteID, map[string]any{
		"site_id":  siteID,
		"cron_id":  cronID,
	})

	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{"status": "success", "message": "Cron job deleted"}, RequestID: rid})
}
