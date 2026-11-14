package apihttp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/publicip"
	"sigardapanel/internal/rbac"
	"sigardapanel/internal/store"
)

// A server's public IPv4 for DNS. The address stored on the server is often
// the private one a cloud VM sees (10.x, 172.16/12), so the server's agent
// detects the public one (publicip.Detect) and the panel keeps it in
// app_settings (server_public_ip_<id>).

type serverPublicIP struct {
	IP        string    `json:"ip"`
	Source    string    `json:"source"`
	CheckedAt time.Time `json:"checked_at"`
}

const publicIPMaxAge = 6 * time.Hour

func serverPublicIPKey(id int64) string { return "server_public_ip_" + strconv.FormatInt(id, 10) }

func cachedServerPublicIP(ctx context.Context, s *store.Store, id int64) (serverPublicIP, bool) {
	raw, _ := store.GetSetting(ctx, s.DB(), serverPublicIPKey(id))
	var v serverPublicIP
	if raw == "" || json.Unmarshal([]byte(raw), &v) != nil || !publicip.IsPublicIPv4(net.ParseIP(v.IP)) {
		return serverPublicIP{}, false
	}
	return v, true
}

// fetchAgentPublicIP asks the server's agent (replaced in tests).
var fetchAgentPublicIP = func(ctx context.Context, srv *store.Server, refresh bool) (publicip.Result, error) {
	if srv == nil || !srv.AgentURL.Valid || srv.AgentURL.String == "" {
		return publicip.Result{}, fmt.Errorf("the server has no agent")
	}
	url := strings.TrimRight(srv.AgentURL.String, "/") + "/tasks/system/public-ip"
	if refresh {
		url += "?refresh=1"
	}
	code, out, err := stackAgentRequest(ctx, http.MethodGet, url, srv.AgentToken, nil, 20*time.Second)
	if err != nil {
		return publicip.Result{}, fmt.Errorf("agent unreachable: %w", err)
	}
	if code >= 400 {
		return publicip.Result{}, agentHTTPError(code, out)
	}
	d, _ := out["data"].(map[string]any)
	ip, _ := d["ip"].(string)
	src, _ := d["source"].(string)
	if !publicip.IsPublicIPv4(net.ParseIP(ip)) {
		return publicip.Result{}, fmt.Errorf("the agent reported %q, not a public IPv4", ip)
	}
	return publicip.Result{IP: net.ParseIP(ip).To4().String(), Source: src}, nil
}

// detectServerPublicIP asks the agent and remembers the answer.
func detectServerPublicIP(ctx context.Context, s *store.Store, srv *store.Server, refresh bool) (serverPublicIP, error) {
	res, err := fetchAgentPublicIP(ctx, srv, refresh)
	if err != nil {
		return serverPublicIP{}, err
	}
	v := serverPublicIP{IP: res.IP, Source: res.Source, CheckedAt: time.Now().UTC()}
	if b, err := json.Marshal(v); err == nil {
		if err := store.SetSetting(ctx, s.DB(), serverPublicIPKey(srv.ID), string(b)); err != nil {
			slog.Warn("public ip: store", "server_id", srv.ID, "error", err)
		}
	}
	return v, nil
}

// dnsTargetIP is the IPv4 a site's A record should point at, and where it
// came from: the server's address when it is public (set by hand or at
// registration), otherwise the public address its agent detected.
func dnsTargetIP(ctx context.Context, s *store.Store, srv *store.Server) (string, string, error) {
	if srv == nil {
		return "", "", fmt.Errorf("the site's server was not found")
	}
	raw := strings.TrimSpace(srv.IPAddress.String)
	if ip := net.ParseIP(raw); srv.IPAddress.Valid && ip != nil && publicip.IsPublicIPv4(ip) {
		return ip.To4().String(), "server address", nil
	}
	cached, ok := cachedServerPublicIP(ctx, s, srv.ID)
	if ok && time.Since(cached.CheckedAt) < publicIPMaxAge {
		return cached.IP, "detected (" + cached.Source + ")", nil
	}
	fresh, err := detectServerPublicIP(ctx, s, srv, false)
	if err == nil {
		return fresh.IP, "detected (" + fresh.Source + ")", nil
	}
	if ok { // detection failed now: the last known address is still the best guess
		return cached.IP, "detected earlier (" + cached.Source + ")", nil
	}
	msg := "could not detect the server's public IP (" + err.Error() + ")"
	if raw != "" {
		msg = fmt.Sprintf("the server's address %s is private and %s", raw, msg)
	}
	return "", "", fmt.Errorf("%s; set its public IP on the Servers page", msg)
}

// GET /api/v1/servers/:id/public-ip[?refresh=1]: the address site DNS uses
// for this server. refresh (a new detection) needs server.manage.
func (h *serverHandler) publicIP(c *echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	ctx := c.Request().Context()
	srv, err := h.store.Servers.GetByID(ctx, id)
	if err != nil || srv == nil {
		return echo.NewHTTPError(http.StatusNotFound, "server not found")
	}
	if c.QueryParam("refresh") == "1" {
		if !sessionCan(ctx, h.store, sessionFromEcho(c), rbac.ServerManage) {
			return echo.NewHTTPError(http.StatusForbidden, "only a super admin can re-detect a server's IP")
		}
		if _, err := detectServerPublicIP(ctx, h.store, srv, true); err != nil {
			return echo.NewHTTPError(http.StatusBadGateway, "detection failed: "+err.Error())
		}
	}
	ip, source, err := dnsTargetIP(ctx, h.store, srv)
	data := map[string]any{"ip": ip, "source": source}
	if err != nil {
		data["error"] = err.Error()
	}
	return c.JSON(http.StatusOK, httpjson.Success{Data: data, RequestID: c.Response().Header().Get(echo.HeaderXRequestID)})
}
