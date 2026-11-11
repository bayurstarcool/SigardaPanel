package agenthttp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"
)

// Suspending a site takes it offline without deleting anything: the nginx
// vhost moves to a sibling "sites-disabled" directory and the certbot renewal
// config moves to "renewal-disabled", so neither nginx nor `certbot renew`
// touches the domain until it is resumed. Site files, databases and the
// certificate itself stay where they are.

type siteToggleRequest struct {
	SiteID int64  `json:"site_id"`
	Domain string `json:"domain"`
}

// fileMove is one rename performed by a suspend/resume, kept so a failed
// nginx config test can put everything back.
type fileMove struct{ from, to string }

func (r *Router) disabledNginxDir() string {
	return filepath.Join(filepath.Dir(r.nginxConfDir), "sites-disabled")
}

// POST /tasks/site/suspend
func (r *Router) siteSuspendTask(c echo.Context) error {
	return r.toggleSite(c, true)
}

// POST /tasks/site/resume
func (r *Router) siteResumeTask(c echo.Context) error {
	return r.toggleSite(c, false)
}

func (r *Router) toggleSite(c echo.Context, suspend bool) error {
	ctx := c.Request().Context()
	var payload siteToggleRequest
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}
	if !isValidDomain(payload.Domain) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid domain")
	}

	action := "resume"
	if suspend {
		action = "suspend"
	}
	slog.Info("agent: site."+action, "site_id", payload.SiteID, "domain", payload.Domain)

	file := payload.Domain + ".conf"
	enabledVhost := filepath.Join(r.nginxConfDir, file)
	disabledVhost := filepath.Join(r.disabledNginxDir(), file)
	enabledRenewal := filepath.Join(r.leDir, "renewal", file)
	disabledRenewal := filepath.Join(r.leDir, "renewal-disabled", file)

	pairs := []fileMove{{enabledVhost, disabledVhost}, {enabledRenewal, disabledRenewal}}
	if !suspend {
		for i := range pairs {
			pairs[i] = fileMove{pairs[i].to, pairs[i].from}
		}
	}

	var done []fileMove
	rollback := func() {
		for i := len(done) - 1; i >= 0; i-- {
			if err := os.Rename(done[i].to, done[i].from); err != nil {
				slog.Error("agent: site."+action+" rollback", "from", done[i].to, "to", done[i].from, "error", err)
			}
		}
	}
	for _, m := range pairs {
		moved, err := moveIfPresent(m.from, m.to)
		if err != nil {
			rollback()
			return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("%s: %v", action, err))
		}
		if moved {
			done = append(done, m)
		}
	}

	if out, err := r.executor.RunShell(ctx, "nginx", "-t"); err != nil {
		rollback()
		return echo.NewHTTPError(http.StatusInternalServerError, "nginx config test failed, changes rolled back: "+strings.TrimSpace(out))
	}
	if out, err := r.executor.RunShell(ctx, "systemctl", "reload", "nginx"); err != nil {
		slog.Warn("agent: nginx reload failed after site."+action, "site_id", payload.SiteID, "error", err, "output", strings.TrimSpace(out))
	}

	// A site with its own app process (node/python) goes down with it.
	appUnit := "/etc/systemd/system/" + appServiceName(payload.SiteID) + ".service"
	if _, err := os.Stat(appUnit); err == nil {
		verb := "start"
		if suspend {
			verb = "stop"
		}
		r.toggleAppService(ctx, verb, payload.SiteID)
	}

	msg := fmt.Sprintf("site %s %sed", payload.Domain, strings.TrimSuffix(action, "e"))
	if len(done) == 0 {
		msg += " (nothing to move — already in that state)"
	}
	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": msg,
	})
}

func (r *Router) toggleAppService(ctx context.Context, verb string, siteID int64) {
	if out, err := r.executor.RunShell(ctx, "systemctl", verb, appServiceName(siteID)); err != nil {
		slog.Warn("agent: app service "+verb, "site_id", siteID, "error", err, "output", strings.TrimSpace(out))
	}
}

// moveIfPresent renames from → to, creating to's directory. A missing source
// is not an error (the move already happened or there was nothing to move),
// but an existing destination is: we never overwrite a config.
func moveIfPresent(from, to string) (bool, error) {
	if _, err := os.Lstat(from); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if _, err := os.Lstat(to); err == nil {
		return false, fmt.Errorf("%s already exists", to)
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return false, err
	}
	if err := os.Rename(from, to); err != nil {
		return false, err
	}
	return true, nil
}
