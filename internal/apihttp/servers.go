package apihttp

import (
	"database/sql"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/audit"
	"sigardapanel/internal/auth"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/store"
)

var (
	hostnameRegex = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?)*$`)
	nameRegex     = regexp.MustCompile(`^[a-zA-Z0-9_\- ]{1,64}$`)
)

func isValidAgentURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

type serverHandler struct {
	store *store.Store
	audit *audit.Recorder
}

func newServerHandler(s *store.Store, a *audit.Recorder) *serverHandler {
	return &serverHandler{store: s, audit: a}
}

// list returns all servers (without agent_token).
func (h *serverHandler) list(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	servers, err := h.store.Servers.List(c.Request().Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	out := make([]map[string]any, 0, len(servers))
	for _, s := range servers {
		out = append(out, serverToMap(s, false))
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"servers": out, "count": len(out)},
		RequestID: rid,
	})
}

// get returns a single server (without agent_token).
func (h *serverHandler) get(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	server, err := h.store.Servers.GetByID(c.Request().Context(), id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if server == nil {
		return echo.NewHTTPError(http.StatusNotFound, "server not found")
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      serverToMap(*server, false),
		RequestID: rid,
	})
}

// create creates a new server. Returns agent_token ONLY on creation.
func (h *serverHandler) create(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	var userID int64
	if sess != nil {
		userID = sess.UserID
	}

	var body struct {
		Name        string `json:"name"`
		Hostname    string `json:"hostname"`
		AgentURL    string `json:"agent_url,omitempty"`
		IPAddress   string `json:"ip_address,omitempty"`
		SSHPort     *int   `json:"ssh_port,omitempty"`
		Description string `json:"description,omitempty"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	body.Name = strings.TrimSpace(body.Name)
	body.Hostname = strings.TrimSpace(body.Hostname)
	body.AgentURL = strings.TrimSpace(body.AgentURL)

	if !nameRegex.MatchString(body.Name) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid name (1-64 chars, alphanumeric/_- )")
	}
	if !hostnameRegex.MatchString(body.Hostname) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid hostname")
	}
	if body.IPAddress != "" && net.ParseIP(body.IPAddress) == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid ip_address")
	}
	if body.AgentURL != "" && !isValidAgentURL(body.AgentURL) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid agent_url")
	}

	token, err := auth.NewToken()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	server := &store.Server{
		Name:       body.Name,
		Hostname:   body.Hostname,
		Status:     "pending",
		AgentToken: token,
	}
	if body.AgentURL != "" {
		server.AgentURL = sql.NullString{String: body.AgentURL, Valid: true}
	}
	if body.IPAddress != "" {
		server.IPAddress = sql.NullString{String: body.IPAddress, Valid: true}
	}
	if body.SSHPort != nil && *body.SSHPort >= 1 && *body.SSHPort <= 65535 {
		server.SSHPort = *body.SSHPort
	} else {
		server.SSHPort = 22
	}
	server.Description = body.Description

	if err := h.store.Servers.Create(c.Request().Context(), server); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), userID, audit.ActionServerCreate, audit.ResourceServer, server.ID, map[string]any{
		"name":      server.Name,
		"hostname":  server.Hostname,
		"agent_url": server.AgentURL.String,
	})

	return c.JSON(http.StatusCreated, httpjson.Success{
		Data:      serverToMap(*server, true),
		RequestID: rid,
	})
}

// update modifies an existing server's editable fields.
func (h *serverHandler) update(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	var userID int64
	if sess != nil {
		userID = sess.UserID
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	server, err := h.store.Servers.GetByID(c.Request().Context(), id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if server == nil {
		return echo.NewHTTPError(http.StatusNotFound, "server not found")
	}

	var body struct {
		Name        *string `json:"name"`
		Hostname    *string `json:"hostname"`
		IPAddress   *string `json:"ip_address"`
		SSHPort     *int    `json:"ssh_port"`
		Description *string `json:"description"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if body.Name != nil {
		n := strings.TrimSpace(*body.Name)
		if n == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "name cannot be empty")
		}
		if !nameRegex.MatchString(n) {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid name (1-64 chars, alphanumeric/_- )")
		}
		server.Name = n
	}
	if body.Hostname != nil {
		hn := strings.TrimSpace(*body.Hostname)
		if hn == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "hostname cannot be empty")
		}
		if !hostnameRegex.MatchString(hn) {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid hostname")
		}
		server.Hostname = hn
	}
	if body.IPAddress != nil {
		ip := strings.TrimSpace(*body.IPAddress)
		if ip != "" && net.ParseIP(ip) == nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid ip_address")
		}
		if ip == "" {
			server.IPAddress = sql.NullString{}
		} else {
			server.IPAddress = sql.NullString{String: ip, Valid: true}
		}
	}
	if body.SSHPort != nil {
		if *body.SSHPort < 1 || *body.SSHPort > 65535 {
			return echo.NewHTTPError(http.StatusBadRequest, "ssh_port must be 1-65535")
		}
		server.SSHPort = *body.SSHPort
	}
	if body.Description != nil {
		server.Description = *body.Description
	}

	if err := h.store.Servers.Update(c.Request().Context(), server); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), userID, audit.ActionServerUpdate, audit.ResourceServer, id, map[string]any{
		"name": server.Name,
	})

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      serverToMap(*server, true),
		RequestID: rid,
	})
}

// delete removes a server.
func (h *serverHandler) delete(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	var userID int64
	if sess != nil {
		userID = sess.UserID
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	server, err := h.store.Servers.GetByID(c.Request().Context(), id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if server == nil {
		return echo.NewHTTPError(http.StatusNotFound, "server not found")
	}

	if err := h.store.Servers.Delete(c.Request().Context(), id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), userID, audit.ActionServerDelete, audit.ResourceServer, id, map[string]any{
		"name": server.Name,
	})

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"deleted": true, "id": id},
		RequestID: rid,
	})
}

// getHealth returns the health status of a single server.
func (h *serverHandler) getHealth(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	server, err := h.store.Servers.GetByID(c.Request().Context(), id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if server == nil {
		return echo.NewHTTPError(http.StatusNotFound, "server not found")
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"id":                server.ID,
			"name":              server.Name,
			"health_status":     server.HealthStatus,
			"health_latency_ms": server.HealthLatencyMs,
			"last_health_check": server.LastHealthCheck,
		},
		RequestID: rid,
	})
}

// getHealthAll returns the health status of all servers.
func (h *serverHandler) getHealthAll(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	servers, err := h.store.Servers.List(c.Request().Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	out := make([]map[string]any, 0, len(servers))
	for _, s := range servers {
		out = append(out, map[string]any{
			"id":                s.ID,
			"name":              s.Name,
			"hostname":          s.Hostname,
			"health_status":     s.HealthStatus,
			"health_latency_ms": s.HealthLatencyMs,
			"last_health_check": s.LastHealthCheck,
		})
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"servers": out, "count": len(out)},
		RequestID: rid,
	})
}

// regenerateToken creates a new agent token for a server.
func (h *serverHandler) regenerateToken(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	server, err := h.store.Servers.GetByID(c.Request().Context(), id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if server == nil {
		return echo.NewHTTPError(http.StatusNotFound, "server not found")
	}

	token, err := auth.NewToken()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	server.AgentToken = token
	if err := h.store.Servers.Update(c.Request().Context(), server); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"agent_token": token},
		RequestID: rid,
	})
}

// serverToMap converts Server to a JSON-friendly map.
// includeToken=true only for create response.
func serverToMap(s store.Server, includeToken bool) map[string]any {
	m := map[string]any{
		"id":          s.ID,
		"name":        s.Name,
		"hostname":    s.Hostname,
		"ssh_port":    s.SSHPort,
		"description": s.Description,
		"status":      s.Status,
		"created_at":  s.CreatedAt,
		"updated_at":  s.UpdatedAt,
	}
	if s.AgentURL.Valid {
		m["agent_url"] = s.AgentURL.String
	}
	if s.IPAddress.Valid {
		m["ip_address"] = s.IPAddress.String
	}
	if s.IPv6.Valid {
		m["ipv6"] = s.IPv6.String
	}
	if s.Provider.Valid {
		m["provider"] = s.Provider.String
	} else {
		m["provider"] = detectProvider()
	}
	if includeToken {
		m["agent_token"] = s.AgentToken
	}
	// Health fields
	m["health_status"] = s.HealthStatus
	m["health_latency_ms"] = s.HealthLatencyMs
	if s.LastHealthCheck.Valid {
		m["last_health_check"] = s.LastHealthCheck.Time
	}
	return m
}

// detectProvider reads the virtualisation / cloud provider from the system.
func detectProvider() string {
	// Try DMI (most cloud VMs expose this)
	data, err := os.ReadFile("/sys/class/dmi/id/sys_vendor")
	if err == nil {
		v := strings.TrimSpace(string(data))
		if v != "" && !strings.Contains(strings.ToLower(v), "default") && !strings.Contains(strings.ToLower(v), "unknown") {
			// Map known vendor strings to clean names
			switch {
			case strings.Contains(strings.ToLower(v), "tencent"):
				return "Tencent Cloud"
			case strings.Contains(strings.ToLower(v), "alibaba") || strings.Contains(strings.ToLower(v), "alicloud"):
				return "Alibaba Cloud"
			case strings.Contains(strings.ToLower(v), "amazon") || strings.Contains(strings.ToLower(v), "aws"):
				return "AWS"
			case strings.Contains(strings.ToLower(v), "google"):
				return "Google Cloud"
			case strings.Contains(strings.ToLower(v), "microsoft") || strings.Contains(strings.ToLower(v), "azure"):
				return "Azure"
			case strings.Contains(strings.ToLower(v), "digitalocean") || strings.Contains(strings.ToLower(v), "digital ocean"):
				return "DigitalOcean"
			case strings.Contains(strings.ToLower(v), "linode") || strings.Contains(strings.ToLower(v), "akamai"):
				return "Linode"
			case strings.Contains(strings.ToLower(v), "vultr"):
				return "Vultr"
			case strings.Contains(strings.ToLower(v), "kvm") || strings.Contains(strings.ToLower(v), "qemu") || strings.Contains(strings.ToLower(v), "bhyve"):
				return v // Return raw for generic hypervisors
			}
			return v
		}
	}

	// Fallback: try product_name
	data, err = os.ReadFile("/sys/class/dmi/id/product_name")
	if err == nil {
		v := strings.TrimSpace(string(data))
		if v != "" && !strings.Contains(strings.ToLower(v), "default") && !strings.Contains(strings.ToLower(v), "standard") {
			return v
		}
	}

	return "Unknown"
}
