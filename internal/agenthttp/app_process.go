package agenthttp

import (
	"fmt"
	"log/slog"
	"net/http"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"
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

	// Build EnvironmentFile line: load .env if present
	envFileLine := fmt.Sprintf("%s/.env", rootPath)

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
EnvironmentFile=-%s
Environment=PORT=%d
Environment=NODE_ENV=production
Environment=PATH=/root/.hermes/node/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin
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
`, domain, rootPath, systemdQuote("exec "+command), userLine, envFileLine, port, memoryMaxMB, memoryHighMB)
}

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
func (r *Router) appStartTask(c echo.Context) error {
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

	svc := appServiceName(payload.SiteID)
	unitPath := filepath.Join("/etc/systemd/system", svc+".service")
	unit := systemdUnitContent(payload.Domain, payload.SystemUser, payload.RootPath, payload.AppCommand, payload.AppPort, payload.AppMemoryMax)

	if err := r.executor.WriteFile(ctx, unitPath, unit, 0644); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "write unit failed: "+err.Error())
	}
	if out, err := r.executor.RunShell(ctx, "systemctl", "daemon-reload"); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "daemon-reload failed: "+strings.TrimSpace(out))
	}
	if out, err := r.executor.RunShell(ctx, "systemctl", "enable", svc); err != nil {
		slog.Warn("agent: app.start enable", "svc", svc, "error", err, "output", strings.TrimSpace(out))
	}
	if out, err := r.executor.RunShell(ctx, "systemctl", "restart", svc); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "start failed: "+strings.TrimSpace(out))
	}

	// Verify it's actually running
	statusOut, _ := r.executor.RunShell(ctx, "systemctl", "is-active", svc)
	active := strings.TrimSpace(statusOut) == "active"

	slog.Info("agent: app.start", "site_id", payload.SiteID, "domain", payload.Domain, "svc", svc, "active", active)
	if !active {
		// Grab last logs for diagnostics
		logs, _ := r.executor.RunShell(ctx, "journalctl", "-u", svc, "-n", "20", "--no-pager")
		return c.JSON(http.StatusOK, map[string]any{
			"ok":         false,
			"app_status": "errored",
			"message":    fmt.Sprintf("app %s failed to start", payload.Domain),
			"logs":       strings.TrimSpace(logs),
		})
	}
	return c.JSON(http.StatusOK, map[string]any{
		"ok":         true,
		"app_status": "running",
		"message":    fmt.Sprintf("app %s started (port %d)", payload.Domain, payload.AppPort),
	})
}

// appStopTask stops the app service.
func (r *Router) appStopTask(c echo.Context) error {
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
func (r *Router) appRestartTask(c echo.Context) error {
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

	// Rewrite unit in case command/port/memory changed
	if payload.AppCommand != "" && payload.AppPort > 0 && isValidDomain(payload.Domain) && r.isAllowedSitePath(payload.RootPath) &&
		validateAppUnitInput(payload.Domain, payload.SystemUser, payload.RootPath, payload.AppCommand) == nil {
		unitPath := filepath.Join("/etc/systemd/system", svc+".service")
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
