package agenthttp

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"

	"github.com/labstack/echo/v5"
	"sigardapanel/internal/sitefs"
)

// pm2NameRe: PM2 app names (also never an option such as "--name").
var pm2NameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// managePM2 handles PM2 operations (start, stop, restart, delete, list, logs).
func (r *Router) managePM2(c *echo.Context) error {
	ctx := c.Request().Context()

	var payload struct {
		Action    string `json:"action"`
		AppName   string `json:"app_name"`
		RootPath  string `json:"root_path"`
		Script    string `json:"script"`
		Env       string `json:"env"`
		Instances int    `json:"instances"`
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}
	if payload.AppName != "" && !pm2NameRe.MatchString(payload.AppName) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid app_name (letters, digits, '.', '_' and '-')")
	}
	if payload.Env != "" && !pm2NameRe.MatchString(payload.Env) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid env")
	}
	if payload.Instances < 0 || payload.Instances > 64 {
		return echo.NewHTTPError(http.StatusBadRequest, "instances must be 1-64")
	}

	// Check if PM2 is installed
	if _, err := r.executor.RunShell(ctx, "which", "pm2"); err != nil {
		slog.Info("pm2: installing PM2")
		if _, err := r.executor.RunShell(ctx, "npm", "install", "-g", "pm2"); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to install PM2: "+err.Error())
		}
	}

	switch payload.Action {
	case "start":
		if payload.AppName == "" || payload.RootPath == "" || payload.Script == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "app_name, root_path, and script are required")
		}
		if payload.Env == "" {
			payload.Env = "production"
		}
		if payload.Instances == 0 {
			payload.Instances = 1
		}

		// The app must be a site's folder (the agent runs as root: never an
		// arbitrary path), opened and verified before use; the script is a
		// file inside it.
		if !r.isAllowedSitePath(payload.RootPath) {
			return echo.NewHTTPError(http.StatusBadRequest, "root_path must be a site folder on this server")
		}
		if !filepath.IsLocal(payload.Script) {
			return echo.NewHTTPError(http.StatusBadRequest, "script must be a path inside root_path")
		}
		pin, err := sitefs.PinDir(payload.RootPath)
		if err != nil {
			return echo.NewHTTPError(sitefs.StatusOf(err), err.Error())
		}
		defer pin.Close()
		root, err := os.OpenRoot(pin.Path)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "cannot open root_path")
		}
		defer root.Close()
		if fi, err := root.Lstat(payload.Script); err != nil || !fi.Mode().IsRegular() {
			return echo.NewHTTPError(http.StatusBadRequest, "script not found in root_path")
		}

		// Install deps if needed
		if _, err := root.Lstat("node_modules"); os.IsNotExist(err) {
			slog.Info("pm2: installing dependencies")
			r.executor.RunShell(ctx, "npm", "install", "--production", "--prefix", pin.Real)
		}

		args := []string{"start", filepath.Join(pin.Real, payload.Script), "--cwd", pin.Real, "--name", payload.AppName, "--env", payload.Env}
		if payload.Instances > 1 {
			args = append(args, "-i", fmt.Sprintf("%d", payload.Instances))
		}
		output, err := r.executor.RunShell(ctx, "pm2", args...)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "start failed: "+err.Error())
		}
		r.executor.RunShell(ctx, "pm2", "save")
		return c.JSON(http.StatusOK, map[string]any{"ok": true, "message": fmt.Sprintf("App %s started", payload.AppName), "output": output})

	case "stop":
		if payload.AppName == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "app_name required")
		}
		output, err := r.executor.RunShell(ctx, "pm2", "stop", payload.AppName)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "stop failed: "+err.Error())
		}
		return c.JSON(http.StatusOK, map[string]any{"ok": true, "message": fmt.Sprintf("App %s stopped", payload.AppName), "output": output})

	case "restart":
		if payload.AppName == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "app_name required")
		}
		output, err := r.executor.RunShell(ctx, "pm2", "restart", payload.AppName)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "restart failed: "+err.Error())
		}
		return c.JSON(http.StatusOK, map[string]any{"ok": true, "message": fmt.Sprintf("App %s restarted", payload.AppName), "output": output})

	case "delete":
		if payload.AppName == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "app_name required")
		}
		output, err := r.executor.RunShell(ctx, "pm2", "delete", payload.AppName)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "delete failed: "+err.Error())
		}
		r.executor.RunShell(ctx, "pm2", "save")
		return c.JSON(http.StatusOK, map[string]any{"ok": true, "message": fmt.Sprintf("App %s deleted", payload.AppName), "output": output})

	case "list":
		output, err := r.executor.RunShell(ctx, "pm2", "list")
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "list failed: "+err.Error())
		}
		return c.JSON(http.StatusOK, map[string]any{"ok": true, "output": output})

	case "logs":
		if payload.AppName == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "app_name required")
		}
		output, err := r.executor.RunShell(ctx, "pm2", "logs", payload.AppName, "--lines", "100", "--nostream")
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "logs failed: "+err.Error())
		}
		return c.JSON(http.StatusOK, map[string]any{"ok": true, "output": output})

	case "save":
		output, err := r.executor.RunShell(ctx, "pm2", "save")
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "save failed: "+err.Error())
		}
		return c.JSON(http.StatusOK, map[string]any{"ok": true, "message": "PM2 process list saved", "output": output})

	case "startup":
		output, err := r.executor.RunShell(ctx, "pm2", "startup")
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "startup failed: "+err.Error())
		}
		return c.JSON(http.StatusOK, map[string]any{"ok": true, "message": "PM2 startup configured", "output": output})

	default:
		return echo.NewHTTPError(http.StatusBadRequest, "unknown action: "+payload.Action)
	}
}
