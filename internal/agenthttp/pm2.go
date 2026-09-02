package agenthttp

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/labstack/echo/v4"
)

// managePM2 handles PM2 operations (start, stop, restart, delete, list, logs).
func (r *Router) managePM2(c echo.Context) error {
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

		// Install deps if needed
		nodeModules := filepath.Join(payload.RootPath, "node_modules")
		if _, err := os.Stat(nodeModules); os.IsNotExist(err) {
			slog.Info("pm2: installing dependencies")
			r.executor.RunShell(ctx, "npm", "install", "--production", "--prefix", payload.RootPath)
		}

		args := []string{"start", payload.Script, "--name", payload.AppName, "--env", payload.Env}
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
		output, err := r.executor.RunShell(ctx, "pm2", "logs", payload.AppName, "--lines", "100")
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
