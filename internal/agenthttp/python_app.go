package agenthttp

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/labstack/echo/v4"
)

// managePythonApp handles Python application management (Gunicorn/WSGI).
func (r *Router) managePythonApp(c echo.Context) error {

	var payload struct {
		Action       string `json:"action"`
		RootPath     string `json:"root_path"`
		AppName      string `json:"app_name"`
		Script       string `json:"script"` // e.g., "app:app" for Flask, "main:app" for FastAPI
		Workers      int    `json:"workers"`
		Bind         string `json:"bind"` // e.g., "127.0.0.1:8000"
		VenvPath     string `json:"venv_path"`
		Requirements string `json:"requirements"` // requirements.txt content
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}

	// Sanitize inputs
	payload.RootPath = sanitizeInput(payload.RootPath)
	payload.AppName = sanitizeInput(payload.AppName)
	payload.Script = sanitizeInput(payload.Script)
	payload.Bind = sanitizeInput(payload.Bind)

	switch payload.Action {
	case "install":
		return r.installPythonApp(c, payload.RootPath, payload.AppName, payload.Script, payload.Workers, payload.Bind, payload.VenvPath, payload.Requirements)
	case "start":
		return r.startPythonApp(c, payload.RootPath, payload.AppName, payload.Script, payload.Workers, payload.Bind, payload.VenvPath)
	case "stop":
		return r.stopPythonApp(c, payload.AppName)
	case "restart":
		return r.restartPythonApp(c, payload.AppName)
	case "status":
		return r.pythonAppStatus(c, payload.AppName)
	case "logs":
		return r.pythonAppLogs(c, payload.AppName)
	default:
		return echo.NewHTTPError(http.StatusBadRequest, "unknown action: "+payload.Action)
	}
}

// installPythonApp sets up a Python application with Gunicorn.
func (r *Router) installPythonApp(c echo.Context, rootPath, appName, script string, workers int, bind, venvPath, requirements string) error {
	ctx := c.Request().Context()

	// Validate inputs
	if rootPath == "" || appName == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "root_path and app_name are required")
	}
	if !isValidDBIdentifier(appName) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid app_name (use lowercase, numbers, hyphens)")
	}
	if workers == 0 {
		workers = 4
	}
	if bind == "" {
		bind = "127.0.0.1:8000"
	}
	if venvPath == "" {
		venvPath = filepath.Join(rootPath, "venv")
	}

	// Create root directory
	if err := os.MkdirAll(rootPath, 0755); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create root directory")
	}

	// Create virtual environment
	slog.Info("python: creating virtual environment", "path", venvPath)
	if _, err := r.executor.RunShell(ctx, "python3", "-m", "venv", venvPath); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create virtual environment: "+err.Error())
	}

	// Install Gunicorn
	pipPath := filepath.Join(venvPath, "bin", "pip")
	slog.Info("python: installing Gunicorn")
	if _, err := r.executor.RunShell(ctx, pipPath, "install", "gunicorn"); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to install Gunicorn: "+err.Error())
	}

	// Install requirements if provided
	if requirements != "" {
		reqFile := filepath.Join(rootPath, "requirements.txt")
		if err := writeFileSecure(reqFile, requirements, 0644); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to write requirements.txt")
		}
		slog.Info("python: installing requirements")
		if _, err := r.executor.RunShell(ctx, pipPath, "install", "-r", reqFile); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to install requirements: "+err.Error())
		}
	}

	// Create systemd service
	serviceContent := fmt.Sprintf(`[Unit]
Description=Gunicorn for %s
After=network.target

[Service]
User=www-data
Group=www-data
WorkingDirectory=%s
ExecStart=%s/gunicorn --workers %d --bind %s %s
Restart=always
RestartSec=5
Environment="PATH=%s/bin"

[Install]
WantedBy=multi-user.target
`, appName, rootPath, filepath.Join(venvPath, "bin"), workers, bind, script, venvPath)

	servicePath := fmt.Sprintf("/etc/systemd/system/gunicorn-%s.service", appName)
	if err := writeFileSecure(servicePath, serviceContent, 0644); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create systemd service")
	}

	// Set permissions
	r.executor.RunShell(ctx, "chown", "-R", "www-data:www-data", rootPath)

	// Enable and start service
	r.executor.RunShell(ctx, "systemctl", "daemon-reload")
	r.executor.RunShell(ctx, "systemctl", "enable", fmt.Sprintf("gunicorn-%s", appName))

	slog.Info("python: app installed", "app", appName)

	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": fmt.Sprintf("Python app %s installed with Gunicorn", appName),
		"service": fmt.Sprintf("gunicorn-%s", appName),
		"bind":    bind,
		"workers": workers,
	})
}

// startPythonApp starts a Python application.
func (r *Router) startPythonApp(c echo.Context, rootPath, appName, script string, workers int, bind, venvPath string) error {
	ctx := c.Request().Context()

	serviceName := fmt.Sprintf("gunicorn-%s", appName)
	if _, err := r.executor.RunShell(ctx, "systemctl", "start", serviceName); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to start app: "+err.Error())
	}

	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": fmt.Sprintf("App %s started", appName),
	})
}

// stopPythonApp stops a Python application.
func (r *Router) stopPythonApp(c echo.Context, appName string) error {
	ctx := c.Request().Context()

	serviceName := fmt.Sprintf("gunicorn-%s", appName)
	if _, err := r.executor.RunShell(ctx, "systemctl", "stop", serviceName); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to stop app: "+err.Error())
	}

	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": fmt.Sprintf("App %s stopped", appName),
	})
}

// restartPythonApp restarts a Python application.
func (r *Router) restartPythonApp(c echo.Context, appName string) error {
	ctx := c.Request().Context()

	serviceName := fmt.Sprintf("gunicorn-%s", appName)
	if _, err := r.executor.RunShell(ctx, "systemctl", "restart", serviceName); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to restart app: "+err.Error())
	}

	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": fmt.Sprintf("App %s restarted", appName),
	})
}

// pythonAppStatus returns the status of a Python application.
func (r *Router) pythonAppStatus(c echo.Context, appName string) error {
	ctx := c.Request().Context()

	serviceName := fmt.Sprintf("gunicorn-%s", appName)
	status, _ := r.executor.RunShell(ctx, "systemctl", "is-active", serviceName)

	return c.JSON(http.StatusOK, map[string]any{
		"app":    appName,
		"status": status,
	})
}

// pythonAppLogs returns logs for a Python application.
func (r *Router) pythonAppLogs(c echo.Context, appName string) error {
	ctx := c.Request().Context()

	serviceName := fmt.Sprintf("gunicorn-%s", appName)
	logs, err := r.executor.RunShell(ctx, "journalctl", "-u", serviceName, "-n", "100", "--no-pager")
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get logs")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"app":  appName,
		"logs": logs,
	})
}
