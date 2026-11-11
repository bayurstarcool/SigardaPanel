package agenthttp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/labstack/echo/v4"
)

// Suspending a site takes it offline without deleting anything: the nginx
// vhost moves to a sibling "sites-disabled" directory and the certbot renewal
// config moves to "renewal-disabled", so neither nginx nor `certbot renew`
// touches the domain until it is resumed. Site files, databases and the
// certificate itself stay where they are.
//
// So that visitors see the suspension (and nginx does not fall back to the
// default site with a 200), a generated placeholder vhost takes the original
// file name in sites-enabled. It starts with suspendedPlaceholderMarker,
// answers 503 with a small static page on port 80 and, when the domain has a
// Let's Encrypt certificate, on 443 with that certificate. Resume deletes the
// file only if it still carries the marker, then restores the original.

// suspendedPlaceholderMarker is the first line of every placeholder vhost.
const suspendedPlaceholderMarker = "# sigardapanel: suspended placeholder"

// suspendedPageHTML is served with the 503. It sits inside a double-quoted
// nginx string: it must contain no double quotes, no "$" (variables) and no
// backslashes.
const suspendedPageHTML = `<!doctype html><html lang=en><head><meta charset=utf-8>` +
	`<meta name=viewport content='width=device-width,initial-scale=1'><meta name=robots content=noindex>` +
	`<title>Site temporarily suspended</title><style>` +
	`body{margin:0;min-height:100vh;display:grid;place-items:center;font-family:system-ui,-apple-system,sans-serif;background:#f6f7f9;color:#1f2328}` +
	`main{text-align:center;padding:2rem}h1{font-size:1.5rem;margin:0 0 .5rem}p{margin:0;color:#57606a}` +
	`@media (prefers-color-scheme:dark){body{background:#0d1117;color:#e6edf3}p{color:#8b949e}}` +
	`</style></head><body><main><h1>This site is temporarily suspended</h1>` +
	`<p>Please check back later.</p></main></body></html>`

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

	// A placeholder in sites-enabled is ours: on suspend it means the vhost
	// was already moved away; on resume it is removed before the original
	// comes back.
	placeholderBefore, err := readSuspendedPlaceholder(enabledVhost)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("%s: %v", action, err))
	}

	var pairs []fileMove
	if suspend {
		if placeholderBefore == nil {
			pairs = append(pairs, fileMove{enabledVhost, disabledVhost})
		}
		pairs = append(pairs, fileMove{enabledRenewal, disabledRenewal})
	} else {
		pairs = []fileMove{{disabledVhost, enabledVhost}, {disabledRenewal, enabledRenewal}}
	}

	var done []fileMove
	placeholderTouched := false
	rollback := func() {
		// Reverse the moves first: a suspend's original vhost renames back
		// over the placeholder it wrote; a resume's original goes back to
		// sites-disabled, freeing the path for the placeholder below.
		for i := len(done) - 1; i >= 0; i-- {
			if err := os.Rename(done[i].to, done[i].from); err != nil {
				slog.Error("agent: site."+action+" rollback", "from", done[i].to, "to", done[i].from, "error", err)
			}
		}
		if !placeholderTouched {
			return
		}
		// Then put the placeholder back exactly as it was, or remove the one
		// this call wrote (only if it is still ours).
		if placeholderBefore != nil {
			if err := os.WriteFile(enabledVhost, placeholderBefore, 0o644); err != nil {
				slog.Error("agent: site."+action+" rollback: restore placeholder", "path", enabledVhost, "error", err)
			}
		} else if err := removeSuspendedPlaceholder(enabledVhost); err != nil {
			slog.Error("agent: site."+action+" rollback: remove placeholder", "path", enabledVhost, "error", err)
		}
	}

	if !suspend && placeholderBefore != nil {
		if err := os.Remove(enabledVhost); err != nil && !errors.Is(err, os.ErrNotExist) {
			return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("resume: remove suspended placeholder: %v", err))
		}
		placeholderTouched = true
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

	if suspend {
		names := suspendedServerNames(disabledVhost, payload.Domain)
		cert := filepath.Join(r.leDir, "live", payload.Domain, "fullchain.pem")
		key := filepath.Join(r.leDir, "live", payload.Domain, "privkey.pem")
		if !fileExists(cert) || !fileExists(key) {
			cert, key = "", ""
		}
		if err := os.MkdirAll(r.nginxConfDir, 0o755); err != nil {
			rollback()
			return echo.NewHTTPError(http.StatusInternalServerError, "suspend: "+err.Error())
		}
		placeholderTouched = true
		if err := os.WriteFile(enabledVhost, []byte(suspendedPlaceholderVhost(payload.Domain, names, cert, key, disabledVhost)), 0o644); err != nil {
			rollback()
			return echo.NewHTTPError(http.StatusInternalServerError, "suspend: write placeholder vhost: "+err.Error())
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
	if len(done) == 0 && (suspend || placeholderBefore == nil) {
		msg += " (nothing to move — already in that state)"
	}
	if suspend {
		msg += "; visitors get a 503 \"temporarily suspended\" page"
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

// readSuspendedPlaceholder returns the content of path when it is a
// placeholder this agent generated (first line is the marker), nil when the
// file is missing or anything else.
func readSuspendedPlaceholder(path string) ([]byte, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, nil // a symlinked vhost is never ours
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if !isSuspendedPlaceholder(data) {
		return nil, nil
	}
	return data, nil
}

func isSuspendedPlaceholder(data []byte) bool {
	first, _, _ := bytes.Cut(data, []byte("\n"))
	return strings.TrimSpace(string(first)) == suspendedPlaceholderMarker
}

// removeSuspendedPlaceholder deletes path only if it is a placeholder.
func removeSuspendedPlaceholder(path string) error {
	data, err := readSuspendedPlaceholder(path)
	if err != nil || data == nil {
		return err
	}
	return os.Remove(path)
}

var (
	serverNameDirective = regexp.MustCompile(`(?m)^\s*server_name\s+([^;]+);`)
	safeServerName      = regexp.MustCompile(`^(\*\.)?[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?$`)
)

// suspendedServerNames returns the domain plus the plain host names the
// original vhost answered for (aliases such as www.), so the placeholder
// covers the same names. Regex names, "_" and anything unusual are skipped.
func suspendedServerNames(originalVhost, domain string) []string {
	names := []string{domain}
	seen := map[string]bool{strings.ToLower(domain): true}
	data, err := os.ReadFile(originalVhost)
	if err != nil {
		return names
	}
	for _, m := range serverNameDirective.FindAllStringSubmatch(string(data), -1) {
		for _, n := range strings.Fields(m[1]) {
			if len(n) > 253 || !safeServerName.MatchString(n) || seen[strings.ToLower(n)] {
				continue
			}
			seen[strings.ToLower(n)] = true
			names = append(names, n)
		}
	}
	return names
}

// suspendedPlaceholderVhost renders the placeholder. cert/key are empty when
// the domain has no Let's Encrypt certificate (port 80 only).
func suspendedPlaceholderVhost(domain string, names []string, cert, key, originalVhost string) string {
	var b strings.Builder
	b.WriteString(suspendedPlaceholderMarker + "\n")
	fmt.Fprintf(&b, "# %s is suspended. Generated by the SigardaPanel agent and removed on resume.\n", domain)
	fmt.Fprintf(&b, "# The site's own vhost is kept in %s. Do not edit this file.\n", originalVhost)
	b.WriteString("server {\n")
	b.WriteString("    listen 80;\n")
	b.WriteString("    listen [::]:80;\n")
	if cert != "" {
		b.WriteString("    listen 443 ssl;\n")
		b.WriteString("    listen [::]:443 ssl;\n")
		fmt.Fprintf(&b, "    ssl_certificate %s;\n", cert)
		fmt.Fprintf(&b, "    ssl_certificate_key %s;\n", key)
	}
	fmt.Fprintf(&b, "    server_name %s;\n", strings.Join(names, " "))
	b.WriteString("    default_type text/html;\n")
	b.WriteString("    add_header Retry-After 3600 always;\n")
	b.WriteString("    add_header Cache-Control \"no-store\" always;\n")
	b.WriteString("    add_header X-Robots-Tag \"noindex\" always;\n")
	fmt.Fprintf(&b, "    return 503 \"%s\";\n", suspendedPageHTML)
	b.WriteString("}\n")
	return b.String()
}

func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
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
