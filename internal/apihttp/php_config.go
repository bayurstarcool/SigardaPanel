package apihttp

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"regexp"
	"sort"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/jobmodel"
)

// phpVersionRe validates a PHP version like "8.1".
var phpVersionRe = regexp.MustCompile(`^\d\.\d$`)

// availablePHPVersions scans /etc/php for installed PHP versions.
func availablePHPVersions() []string {
	entries, err := os.ReadDir("/etc/php")
	if err != nil {
		return []string{}
	}
	var versions []string
	for _, e := range entries {
		if e.IsDir() && phpVersionRe.MatchString(e.Name()) {
			// Only include versions that have an fpm config
			if _, err := os.Stat("/etc/php/" + e.Name() + "/fpm"); err == nil {
				versions = append(versions, e.Name())
			}
		}
	}
	sort.Strings(versions)
	return versions
}

// getPHPConfig returns the site's PHP version, custom php.ini, and available versions.
func (h *siteHandler) getPHPConfig(c echo.Context) error {
	site, err := h.getSiteForUser(c)
	if err != nil {
		return err
	}
	if site.Runtime != "php" {
		return echo.NewHTTPError(http.StatusBadRequest, "site is not a PHP runtime")
	}
	return c.JSON(http.StatusOK, map[string]any{
		"data": map[string]any{
			"php_version":        site.PHPVersion.String,
			"php_ini":            site.PHPIni.String,
			"available_versions": availablePHPVersions(),
			"default_note":       "Empty version = CloudPanel default (clp-php-fpm.sock)",
		},
	})
}

// updatePHPConfig sets the per-site PHP version and custom php.ini directives,
// then regenerates the nginx vhost. Does NOT touch global PHP-FPM config.
func (h *siteHandler) updatePHPConfig(c echo.Context) error {
	site, err := h.getSiteForUser(c)
	if err != nil {
		return err
	}
	if site.Runtime != "php" {
		return echo.NewHTTPError(http.StatusBadRequest, "site is not a PHP runtime")
	}

	var body struct {
		Version string `json:"php_version"`
		Ini     string `json:"php_ini"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}

	// Validate version if provided (empty = use default socket)
	if body.Version != "" {
		if !phpVersionRe.MatchString(body.Version) {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid PHP version format")
		}
		valid := false
		for _, v := range availablePHPVersions() {
			if v == body.Version {
				valid = true
				break
			}
		}
		if !valid {
			return echo.NewHTTPError(http.StatusBadRequest, "PHP version not installed on server")
		}
	}

	// Sanitize php.ini directives: single line, no nginx-breaking chars.
	ini := sanitizePHPIni(body.Ini)

	if err := h.store.Sites.UpdatePHPConfig(c.Request().Context(), site.ID, body.Version, ini); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update PHP config")
	}

	// Regenerate vhost so the fastcgi_pass / PHP_VALUE takes effect.
	payload, _ := json.Marshal(map[string]any{
		"site_id":     site.ID,
		"name":        site.Name,
		"domain":      site.Domain,
		"runtime":     site.Runtime,
		"root_path":   site.RootPath,
		"php_version": body.Version,
		"php_ini":     ini,
	})
	job := &jobmodel.Job{
		ServerID: sql.NullInt64{Int64: site.ServerID, Valid: true},
		SiteID:   sql.NullInt64{Int64: site.ID, Valid: true},
		Type:     jobmodel.TypeSiteCreate,
		Payload:  string(payload),
	}
	if err := h.store.Jobs.Create(c.Request().Context(), job); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to enqueue vhost regen")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"data": map[string]any{
			"php_version": body.Version,
			"php_ini":     ini,
			"job_id":      job.ID,
			"status":      "queued",
		},
	})
}

// sanitizePHPIni cleans custom php.ini directives for safe injection into a
// single-line nginx fastcgi_param PHP_VALUE. Newlines become \n separators
// (nginx PHP_VALUE syntax), and quotes/backslashes that would break the
// nginx directive are stripped.
func sanitizePHPIni(raw string) string {
	if raw == "" {
		return ""
	}
	// Strip characters that would break the nginx string or inject config.
	cleaned := make([]rune, 0, len(raw))
	for _, r := range raw {
		switch r {
		case '"', '\\', ';', '{', '}':
			// drop nginx-breaking / injection chars
			continue
		case '\r':
			continue
		case '\n':
			cleaned = append(cleaned, '\n')
		default:
			cleaned = append(cleaned, r)
		}
	}
	// Convert newlines into nginx PHP_VALUE line separator (\n literal).
	out := ""
	lines := splitLines(string(cleaned))
	for i, ln := range lines {
		ln = trimSpace(ln)
		if ln == "" {
			continue
		}
		if i > 0 && out != "" {
			out += "\\n "
		}
		out += ln
	}
	return out
}

func splitLines(s string) []string {
	var lines []string
	cur := ""
	for _, r := range s {
		if r == '\n' {
			lines = append(lines, cur)
			cur = ""
		} else {
			cur += string(r)
		}
	}
	lines = append(lines, cur)
	return lines
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

// sshInfo returns read-only SSH connection details for the site.
// It does NOT expose credentials — just the connection command and metadata.
func (h *siteHandler) sshInfo(c echo.Context) error {
	site, err := h.getSiteForUser(c)
	if err != nil {
		return err
	}

	sysUser := site.SystemUser.String
	if sysUser == "" {
		sysUser = "root"
	}

	// Prefer public IP address over hostname for external connections
	host := "your-server-ip"
	if srv, err := h.store.Servers.GetByID(c.Request().Context(), site.ServerID); err == nil && srv != nil {
		if srv.IPAddress.Valid && srv.IPAddress.String != "" {
			host = srv.IPAddress.String
		} else if srv.Hostname != "" {
			host = srv.Hostname
		}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"data": map[string]any{
			"host":        host,
			"port":        22,
			"system_user": sysUser,
			"root_path":   site.RootPath,
			"ssh_command": "ssh " + sysUser + "@" + host,
			"scp_hint":    "scp -r ./local-folder " + sysUser + "@" + host + ":" + site.RootPath,
			"note":        "Use your existing SSH key or server password. SigardaPanel does not store SSH credentials.",
		},
	})
}
