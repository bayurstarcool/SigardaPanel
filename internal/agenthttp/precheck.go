package agenthttp

import (
	"net/http"
	"os/exec"
	"strings"

	"github.com/labstack/echo/v5"
)

// precheckTask answers POST /tasks/precheck {kind, version}: whether what a
// new site or database needs is installed on this server, so the panel can
// say so before it creates anything (kind: php | mysql | postgres).
func (r *Router) precheckTask(c *echo.Context) error {
	var p struct {
		Kind    string `json:"kind"`
		Version string `json:"version"`
	}
	if err := c.Bind(&p); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "invalid payload"})
	}
	ctx := c.Request().Context()
	fail := func(msg string) error { return c.JSON(http.StatusOK, map[string]any{"ok": false, "message": msg}) }
	switch p.Kind {
	case "php":
		if err := r.checkPHPFPM(strings.TrimSpace(p.Version)); err != nil {
			return fail(err.Error())
		}
	case "mysql":
		if _, err := exec.LookPath("mysql"); err != nil {
			return fail("MariaDB/MySQL is not installed on this server: install MariaDB under Servers -> Stack, then try again")
		}
		if out, err := r.executor.RunWithInput(ctx, "SELECT 1;\n", "mysql", "-N", "-B"); err != nil {
			return fail("MariaDB/MySQL is installed but not reachable on this server (is the service running? Servers -> Stack): " + strings.TrimSpace(out))
		}
	case "postgres":
		if out, err := r.postgresAdmin(ctx, "postgres", "SELECT 1;\n"); err != nil {
			return fail("PostgreSQL is not installed or not running on this server: " + pgErr(out, err))
		}
	default:
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "unknown check"})
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true})
}
