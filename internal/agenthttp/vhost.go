package agenthttp

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/labstack/echo/v4"
)

// vhostMu: raw vhost validation briefly swaps the live file and saving
// replaces it, so both hold it exclusively. Everything else that writes
// nginx config or reloads nginx holds it shared (readNginxLock), so no
// reload can make an unvalidated candidate live and no write is undone by a
// validation restoring the previous file.
var vhostMu sync.RWMutex

// readNginxLock takes the shared side of vhostMu; call the result to release.
func readNginxLock() func() {
	vhostMu.RLock()
	return vhostMu.RUnlock
}

// nginxTestMessage keeps the lines of nginx -t output that explain a failure.
func nginxTestMessage(raw string) string {
	var kept []string
	for _, l := range strings.Split(raw, "\n") {
		if strings.Contains(l, "[emerg]") || strings.Contains(l, "[error]") ||
			strings.Contains(l, "test failed") || strings.Contains(l, "syntax is") {
			kept = append(kept, l)
		}
	}
	if len(kept) == 0 {
		return strings.TrimSpace(raw)
	}
	return strings.Join(kept, "\n")
}

func (r *Router) vhostPath(domain string) string {
	return filepath.Join(r.nginxConfDir, domain+".conf")
}

// vhostGetTask answers GET /tasks/vhost/get?domain=.
func (r *Router) vhostGetTask(c echo.Context) error {
	domain := c.QueryParam("domain")
	if !isValidDomain(domain) {
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "invalid domain"})
	}
	path := r.vhostPath(domain)
	raw, err := os.ReadFile(path)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{"ok": true, "exists": false, "path": path})
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true, "exists": true, "path": path, "config": string(raw)})
}

// withCandidateVhost installs config as the domain's live vhost, runs fn
// (typically nginx -t), and restores the previous file unless keep returns
// true. The caller holds vhostMu.
func (r *Router) withCandidateVhost(ctx context.Context, domain, config string, keep func(testErr error) bool) (string, error) {
	path := r.vhostPath(domain)
	prev, prevErr := os.ReadFile(path)
	if err := os.WriteFile(path, []byte(config), 0o644); err != nil {
		return "", err
	}
	out, testErr := r.executor.RunShell(ctx, "nginx", "-t")
	if !keep(testErr) {
		if prevErr == nil {
			_ = os.WriteFile(path, prev, 0o644)
		} else {
			_ = os.Remove(path)
		}
	}
	return out, testErr
}

// vhostValidateTask answers POST /tasks/vhost/validate {domain, config}:
// nginx -t with the candidate in place, then the live file is restored.
func (r *Router) vhostValidateTask(c echo.Context) error {
	var p struct {
		Domain string `json:"domain"`
		Config string `json:"config"`
	}
	if err := c.Bind(&p); err != nil || !isValidDomain(p.Domain) {
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "invalid payload"})
	}
	vhostMu.Lock()
	defer vhostMu.Unlock()
	out, err := r.withCandidateVhost(c.Request().Context(), p.Domain, p.Config, func(error) bool { return false })
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{"ok": true, "valid": false, "message": nginxTestMessage(out)})
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true, "valid": true, "message": "nginx configuration syntax is valid ✓"})
}

// vhostPutTask answers POST /tasks/vhost/put {domain, config}: install the
// vhost if nginx -t accepts it (otherwise the previous one stays), reload.
func (r *Router) vhostPutTask(c echo.Context) error {
	var p struct {
		Domain string `json:"domain"`
		Config string `json:"config"`
	}
	if err := c.Bind(&p); err != nil || !isValidDomain(p.Domain) {
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "invalid payload"})
	}
	ctx := c.Request().Context()
	vhostMu.Lock()
	defer vhostMu.Unlock()
	out, err := r.withCandidateVhost(ctx, p.Domain, p.Config, func(testErr error) bool { return testErr == nil })
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{"ok": true, "status": "invalid", "nginx_error": nginxTestMessage(out), "path": r.vhostPath(p.Domain)})
	}
	// Keep sites-available in step when the server uses it.
	if avail := filepath.Join(filepath.Dir(r.nginxConfDir), "sites-available"); dirExists(avail) {
		if fi, err := os.Lstat(filepath.Join(avail, p.Domain+".conf")); err == nil && fi.Mode().IsRegular() {
			_ = os.WriteFile(filepath.Join(avail, p.Domain+".conf"), []byte(p.Config), 0o644)
		}
	}
	reloadOut, reloadErr := r.executor.RunShell(ctx, "nginx", "-s", "reload")
	return c.JSON(http.StatusOK, map[string]any{
		"ok": true, "status": "success", "path": r.vhostPath(p.Domain),
		"reload_ok": reloadErr == nil, "reload_output": strings.TrimSpace(reloadOut),
	})
}

func dirExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// siteUsageTask answers POST /tasks/site/usage {root_path}: disk usage in bytes.
func (r *Router) siteUsageTask(c echo.Context) error {
	var p struct {
		RootPath string `json:"root_path"`
	}
	if err := c.Bind(&p); err != nil || !r.isAllowedSitePath(p.RootPath) {
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "invalid root_path"})
	}
	if !dirExists(p.RootPath) {
		return c.JSON(http.StatusOK, map[string]any{"ok": true, "disk_used_bytes": 0, "exists": false})
	}
	out, err := r.executor.RunShell(c.Request().Context(), "du", "-sb", p.RootPath)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "du failed: " + strings.TrimSpace(out)})
	}
	size, _ := strconv.ParseInt(strings.Fields(out + " 0")[0], 10, 64)
	return c.JSON(http.StatusOK, map[string]any{"ok": true, "disk_used_bytes": size, "exists": true})
}
