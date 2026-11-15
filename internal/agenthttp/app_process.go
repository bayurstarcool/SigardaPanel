package agenthttp

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
	"sigardapanel/internal/sitefs"
	"sigardapanel/internal/siteinput"
)

// appServiceName returns the systemd unit name for a site app.
func appServiceName(siteID int64) string {
	return fmt.Sprintf("sigarda-app-%d", siteID)
}

// systemdUnitContent builds a systemd service unit for a site app.
// Resource-frugal defaults: memory cap, restart backoff, no swap thrash.
func systemdUnitContent(domain, systemUser, rootPath, command string, port, memoryMaxMB int64) string {
	if memoryMaxMB <= 0 {
		memoryMaxMB = 512
	}
	// MemoryHigh (soft throttle) set to 75% of hard cap.
	memoryHighMB := memoryMaxMB * 3 / 4

	// Build User= line: only set if a real system user exists
	userLine := ""
	if systemUser != "" {
		userLine = fmt.Sprintf("User=%s", systemUser)
	}

	return fmt.Sprintf(`[Unit]
Description=SigardaPanel App: %s
After=network.target postgresql.service redis-server.service

[Service]
Type=simple
WorkingDirectory=%s
ExecStart=/bin/sh -lc %s
Restart=always
RestartSec=5
%s
StandardOutput=append:%s
StandardError=append:%s
Environment=PORT=%d
Environment=NODE_ENV=production
Environment=PYTHONUNBUFFERED=1
Environment=PATH=/usr/local/bin:/usr/local/go/bin:/usr/local/node/bin:/usr/bin:/bin:/usr/sbin:/sbin
# Resource limits (hemat resource)
MemoryMax=%dM
MemoryHigh=%dM
CPUWeight=50
TasksMax=256
# Security hardening
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
`, domain, rootPath, systemdQuote(appEnvPrelude+"exec "+command), userLine, appLogPath(domain), appLogPath(domain), port, memoryMaxMB, memoryHighMB)
}

// appEnvPrelude loads the site's .env in the app's own shell, which already
// runs as the site user. systemd's EnvironmentFile= is read by PID 1 as root
// and follows symlinks: a tenant's .env -> /etc/sigardapanel/agent.env handed
// the agent token (root) to the app.
const appEnvPrelude = "if [ -f ./.env ] && [ -r ./.env ]; then set -a; . ./.env; set +a; fi; "

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// systemdQuote wraps s in double quotes for a systemd ExecStart= word. systemd
// unescapes \\ and \" itself and expands %-specifiers and $-variables, so those
// are escaped here; the shell therefore receives s byte-for-byte.
func systemdQuote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `%`, `%%`, `$`, `$$`, "\n", `\n`, "\r", `\r`)
	return `"` + r.Replace(s) + `"`
}

// validateAppUnitInput guards everything that ends up in the unit file. The
// unit is written as root, so the account must be a real, non-root user and no
// field may contain a newline (which would inject extra unit directives).
func validateAppUnitInput(domain, systemUser, rootPath, command string) error {
	if !isValidDomain(domain) {
		return fmt.Errorf("invalid domain")
	}
	if err := siteinput.RootPath(rootPath); err != nil {
		return err
	}
	if err := siteinput.SystemUser(systemUser); err != nil {
		return fmt.Errorf("a valid non-root system_user is required to run an app")
	}
	u, err := user.Lookup(systemUser)
	if err != nil || u.Uid == "0" {
		return fmt.Errorf("system_user %q does not exist or is root", systemUser)
	}
	if _, _, ok := siteUserIDs(systemUser); !ok {
		return fmt.Errorf("system_user %q is not a regular site account", systemUser)
	}
	if command == "" || len(command) > 1024 {
		return fmt.Errorf("invalid app command")
	}
	for _, r := range command {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("app command must be a single line without control characters")
		}
	}
	return nil
}

// appStartTask writes the systemd unit and starts the app service.
func (r *Router) appStartTask(c *echo.Context) error {
	ctx := c.Request().Context()
	var payload struct {
		SiteID       int64  `json:"site_id"`
		Domain       string `json:"domain"`
		RootPath     string `json:"root_path"`
		SystemUser   string `json:"system_user"`
		AppPort      int64  `json:"app_port"`
		AppCommand   string `json:"app_command"`
		AppMemoryMax int64  `json:"app_memory_max"`
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}
	if !isValidDomain(payload.Domain) || !r.isAllowedSitePath(payload.RootPath) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid domain or path")
	}
	if payload.AppCommand == "" || payload.AppPort <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "missing app command or port")
	}
	if err := validateAppUnitInput(payload.Domain, payload.SystemUser, payload.RootPath, payload.AppCommand); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	status, msg, logs, err := r.startApp(ctx, payload.SiteID, payload.Domain, payload.SystemUser, payload.RootPath, payload.AppCommand, payload.AppPort, payload.AppMemoryMax)
	if err != nil {
		return err
	}
	if status != "running" {
		return c.JSON(http.StatusOK, map[string]any{"ok": false, "app_status": status, "message": msg, "logs": logs})
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true, "app_status": status, "message": msg})
}

// startApp builds what the command needs (a Go binary), writes the app's
// systemd unit and (re)starts it. It returns the resulting status
// ("running" or "errored") with a message and, on failure, the last logs.
func (r *Router) startApp(ctx context.Context, siteID int64, domain, systemUser, rootPath, command string, port, memoryMax int64) (string, string, string, error) {
	r.upgradeOldStarter(ctx, systemUser, rootPath, domain, command, port)
	if out, err := r.prepareGoApp(ctx, systemUser, rootPath, command); err != nil {
		return "errored", err.Error(), out, nil
	}
	svc := appServiceName(siteID)
	unitPath := filepath.Join("/etc/systemd/system", svc+".service")
	r.ensureAppLogSetup(ctx)
	unit := systemdUnitContent(domain, systemUser, rootPath, command, port, memoryMax)
	if err := r.executor.WriteFile(ctx, unitPath, unit, 0644); err != nil {
		return "", "", "", echo.NewHTTPError(http.StatusInternalServerError, "write unit failed: "+err.Error())
	}
	if out, err := r.executor.RunShell(ctx, "systemctl", "daemon-reload"); err != nil {
		return "", "", "", echo.NewHTTPError(http.StatusInternalServerError, "daemon-reload failed: "+strings.TrimSpace(out))
	}
	if out, err := r.executor.RunShell(ctx, "systemctl", "enable", svc); err != nil {
		slog.Warn("agent: app.start enable", "svc", svc, "error", err, "output", strings.TrimSpace(out))
	}
	if out, err := r.executor.RunShell(ctx, "systemctl", "restart", svc); err != nil {
		return "", "", "", echo.NewHTTPError(http.StatusInternalServerError, "start failed: "+strings.TrimSpace(out))
	}
	// The process may exit right after starting (bad command, port taken):
	// check that it stays up and listens.
	active := false
	for i := 0; i < 10; i++ {
		time.Sleep(500 * time.Millisecond)
		statusOut, _ := r.executor.RunShell(ctx, "systemctl", "is-active", svc)
		active = strings.TrimSpace(statusOut) == "active"
		if active && portListening(int(port)) {
			break
		}
	}
	slog.Info("agent: app.start", "site_id", siteID, "domain", domain, "svc", svc, "active", active)
	if !active {
		logs, _ := r.executor.RunShell(ctx, "journalctl", "-u", svc, "-n", "20", "--no-pager")
		return "errored", fmt.Sprintf("app %s failed to start", domain), strings.TrimSpace(logs), nil
	}
	return "running", fmt.Sprintf("app %s started (port %d)", domain, port), "", nil
}

// portListening reports whether something listens on the TCP port.
func portListening(port int) bool {
	for _, p := range listeningTCPPorts() {
		if p == port {
			return true
		}
	}
	return false
}

// goBinary finds the go tool (often /usr/local/go/bin, outside PATH).
func goBinary() string {
	if p, err := exec.LookPath("go"); err == nil {
		return p
	}
	for _, p := range []string{"/usr/local/go/bin/go", "/usr/lib/go/bin/go", "/snap/bin/go"} {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	return ""
}

// prepareGoApp builds ./app (as the site user) when the command runs it and
// it does not exist yet but the folder holds a Go module.
func (r *Router) prepareGoApp(ctx context.Context, systemUser, rootPath, command string) (string, error) {
	bin := strings.Fields(command)
	if len(bin) == 0 || bin[0] != "./app" {
		return "", nil
	}
	// Checked inside the pinned root: the site user owns the tree, so a
	// path-based root lookup could be redirected by a swapped symlink.
	pin, err := sitefs.PinDir(rootPath)
	if err != nil {
		return "", err
	}
	defer pin.Close()
	root, err := os.OpenRoot(pin.Path)
	if err != nil {
		return "", err
	}
	defer root.Close()
	if fi, err := root.Lstat("app"); err == nil && fi.Mode().IsRegular() {
		return "", nil
	}
	if _, err := root.Lstat("go.mod"); err != nil {
		return "", nil
	}
	gobin := goBinary()
	if gobin == "" {
		return "", fmt.Errorf("Go is not installed on this server: install it on the Stack page, or upload a built ./app binary")
	}
	uid, gid, ok := deployUserIDs(systemUser)
	if !ok {
		return "", fmt.Errorf("the site has no usable Linux user to build the Go app as")
	}
	out, err := r.runAsSiteUser(ctx, uid, gid, pin.Real, gobin, "build", "-o", "app", ".")
	if err != nil {
		return strings.TrimSpace(out), fmt.Errorf("go build failed: %s", strings.TrimSpace(out))
	}
	return "", nil
}

// appStopTask stops the app service.
func (r *Router) appStopTask(c *echo.Context) error {
	ctx := c.Request().Context()
	var payload struct {
		SiteID int64  `json:"site_id"`
		Domain string `json:"domain"`
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}
	svc := appServiceName(payload.SiteID)
	if out, err := r.executor.RunShell(ctx, "systemctl", "stop", svc); err != nil {
		slog.Warn("agent: app.stop", "svc", svc, "error", err, "output", strings.TrimSpace(out))
	}
	slog.Info("agent: app.stop", "site_id", payload.SiteID, "svc", svc)
	return c.JSON(http.StatusOK, map[string]any{
		"ok":         true,
		"app_status": "stopped",
		"message":    fmt.Sprintf("app %s stopped", payload.Domain),
	})
}

// appRestartTask restarts the app service.
func (r *Router) appRestartTask(c *echo.Context) error {
	ctx := c.Request().Context()
	var payload struct {
		SiteID       int64  `json:"site_id"`
		Domain       string `json:"domain"`
		RootPath     string `json:"root_path"`
		SystemUser   string `json:"system_user"`
		AppPort      int64  `json:"app_port"`
		AppCommand   string `json:"app_command"`
		AppMemoryMax int64  `json:"app_memory_max"`
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}
	svc := appServiceName(payload.SiteID)

	// With the full app config, a restart is a start: rewrite the unit,
	// build a missing Go binary and wait for the port.
	if payload.AppCommand != "" && payload.AppPort > 0 && isValidDomain(payload.Domain) && r.isAllowedSitePath(payload.RootPath) &&
		validateAppUnitInput(payload.Domain, payload.SystemUser, payload.RootPath, payload.AppCommand) == nil {
		status, msg, logs, err := r.startApp(ctx, payload.SiteID, payload.Domain, payload.SystemUser, payload.RootPath, payload.AppCommand, payload.AppPort, payload.AppMemoryMax)
		if err != nil {
			return err
		}
		if status != "running" {
			return c.JSON(http.StatusOK, map[string]any{"ok": false, "app_status": status, "message": msg, "logs": logs})
		}
		return c.JSON(http.StatusOK, map[string]any{"ok": true, "app_status": status, "message": fmt.Sprintf("app %s restarted", payload.Domain)})
	}

	// Rewrite unit in case command/port/memory changed
	if payload.AppCommand != "" && payload.AppPort > 0 && isValidDomain(payload.Domain) && r.isAllowedSitePath(payload.RootPath) &&
		validateAppUnitInput(payload.Domain, payload.SystemUser, payload.RootPath, payload.AppCommand) == nil {
		unitPath := filepath.Join("/etc/systemd/system", svc+".service")
		r.ensureAppLogSetup(ctx)
		unit := systemdUnitContent(payload.Domain, payload.SystemUser, payload.RootPath, payload.AppCommand, payload.AppPort, payload.AppMemoryMax)
		if err := r.executor.WriteFile(ctx, unitPath, unit, 0644); err == nil {
			r.executor.RunShell(ctx, "systemctl", "daemon-reload")
		}
	}

	if out, err := r.executor.RunShell(ctx, "systemctl", "restart", svc); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "restart failed: "+strings.TrimSpace(out))
	}
	statusOut, _ := r.executor.RunShell(ctx, "systemctl", "is-active", svc)
	active := strings.TrimSpace(statusOut) == "active"
	slog.Info("agent: app.restart", "site_id", payload.SiteID, "svc", svc, "active", active)
	if !active {
		logs, _ := r.executor.RunShell(ctx, "journalctl", "-u", svc, "-n", "20", "--no-pager")
		return c.JSON(http.StatusOK, map[string]any{
			"ok":         false,
			"app_status": "errored",
			"message":    fmt.Sprintf("app %s failed to restart", payload.Domain),
			"logs":       strings.TrimSpace(logs),
		})
	}
	return c.JSON(http.StatusOK, map[string]any{
		"ok":         true,
		"app_status": "running",
		"message":    fmt.Sprintf("app %s restarted", payload.Domain),
	})
}
