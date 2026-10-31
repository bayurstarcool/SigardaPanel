package apihttp

import (
	"bufio"
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/store"
)

type logsHandler struct {
	store *store.Store
}

func newLogsHandler(s *store.Store) *logsHandler {
	return &logsHandler{store: s}
}

type logEntry struct {
	Level     string `json:"level,omitempty"`
	Message   string `json:"message"`
	CreatedAt string `json:"created_at,omitempty"`
}

// nginx access log format: IP - user [time] "REQ" status bytes "ref" "ua"
var nginxAccessRe = regexp.MustCompile(`^(\S+) \S+ \S+ \[([^\]]+)\] "([^"]*)" (\d+) (\d+)`)

// nginx error log: YYYY/MM/DD HH:MM:SS [level] pid#tid: message
var nginxErrorRe = regexp.MustCompile(`^(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}) \[(\w+)\]\s+(.+)$`)

// php-fpm log: [DD-Mon-YYYY HH:MM:SS] LEVEL: message
var phpFpmRe = regexp.MustCompile(`^\[([^\]]+)\] (\w+):\s+(.+)$`)

func (h *logsHandler) tail(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	site, err := h.store.Sites.GetByID(c.Request().Context(), id)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	if !h.canAccess(c.Request().Context(), sess, site) {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	logType := c.QueryParam("type")
	if logType == "" {
		logType = "access"
	}
	validTypes := map[string]bool{"access": true, "error": true, "php": true}
	if !validTypes[logType] {
		return echo.NewHTTPError(http.StatusBadRequest, "type must be access, error, or php")
	}

	// Determine log path
	logPath := resolveLogPath(site.Domain, logType)

	// Default: tail 200 lines
	limit := 200
	if l := c.QueryParam("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 5000 {
			limit = n
		}
	}

	entries, exists, fileErr := readLogTail(logPath, limit, logType)

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"site_id": site.ID,
			"domain":  site.Domain,
			"type":    logType,
			"path":    logPath,
			"exists":  exists,
			"logs":    entries,
			"error":   fileErr,
		},
		RequestID: rid,
	})
}

// resolveLogPath returns the canonical log file path for a domain + log type.
func resolveLogPath(domain, logType string) string {
	switch logType {
	case "access":
		return filepath.Join("/var/log/nginx", domain+".access.log")
	case "error":
		return filepath.Join("/var/log/nginx", domain+".error.log")
	case "php":
		// Try common php-fpm log locations — first existing wins
		candidates := []string{
			"/var/log/php8.3-fpm.log",
			"/var/log/php8.2-fpm.log",
			"/var/log/php8.1-fpm.log",
			"/var/log/php8.0-fpm.log",
			"/var/log/php7.4-fpm.log",
			"/var/log/php7.1-fpm.log",
		}
		for _, p := range candidates {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
		return candidates[0]
	}
	return ""
}

// readLogTail reads the last N lines of a log file and parses them.
// Returns parsed entries, whether file exists, and any error string.
func readLogTail(path string, limit int, logType string) ([]logEntry, bool, string) {
	if path == "" {
		return []logEntry{}, false, "log path not configured"
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []logEntry{}, false, ""
		}
		return []logEntry{}, false, "cannot open log: " + err.Error()
	}
	defer f.Close()

	// Read all lines into ring buffer of size `limit`
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 4*1024*1024)
	lines := make([]string, 0, limit)
	for scanner.Scan() {
		if len(lines) >= limit {
			lines = lines[1:]
		}
		lines = append(lines, scanner.Text())
	}

	// Reverse so newest first
	entries := make([]logEntry, 0, len(lines))
	for i := len(lines) - 1; i >= 0; i-- {
		entries = append(entries, parseLogLine(lines[i], logType))
	}
	return entries, true, ""
}

func parseLogLine(line, logType string) logEntry {
	switch logType {
	case "access":
		if m := nginxAccessRe.FindStringSubmatch(line); m != nil {
			status, _ := strconv.Atoi(m[4])
			level := "info"
			if status >= 500 {
				level = "error"
			} else if status >= 400 {
				level = "warn"
			} else if status >= 300 {
				level = "debug"
			}
			return logEntry{
				Level:     level,
				Message:   line,
				CreatedAt: parseNginxTime(m[2]),
			}
		}
	case "error":
		if m := nginxErrorRe.FindStringSubmatch(line); m != nil {
			return logEntry{
				Level:     normalizeLevel(m[2]),
				Message:   m[3],
				CreatedAt: strings.Replace(m[1], "/", "-", 2),
			}
		}
	case "php":
		if m := phpFpmRe.FindStringSubmatch(line); m != nil {
			return logEntry{
				Level:     normalizeLevel(m[2]),
				Message:   m[3],
				CreatedAt: m[1],
			}
		}
	}
	return logEntry{Message: line}
}

func normalizeLevel(l string) string {
	l = strings.ToLower(l)
	switch l {
	case "emerg", "alert", "crit", "critical", "error", "err":
		return "error"
	case "warn", "warning", "notice":
		return "warn"
	case "info":
		return "info"
	case "debug":
		return "debug"
	}
	return l
}

// parseNginxTime: "29/Jun/2026:14:23:45 +0700" -> readable
func parseNginxTime(s string) string {
	// Just return as-is; frontend will display
	return s
}

// canAccess reports whether the session may read the site's logs: owner,
// super_admin or any user_sites assignment (read and up). Being a non-super
// admin does not grant access to other tenants' logs.
func (h *logsHandler) canAccess(ctx context.Context, sess *store.Session, site *store.Site) bool {
	return hasSitePermission(ctx, h.store, sess, site, sitePermRead)
}
