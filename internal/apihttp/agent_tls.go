package apihttp

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v5"

	"sigardapanel/internal/agenttls"
	"sigardapanel/internal/audit"
	"sigardapanel/internal/httpjson"
)

// enableAgentTLS answers POST /api/v1/servers/:id/agent-tls (super_admin):
// switch a remote agent from plain http to pinned TLS now (the health
// checker also does it on its own for agents that support it).
func (h *agentUpdateHandler) enableAgentTLS(c *echo.Context) error {
	ctx := c.Request().Context()
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid server id")
	}
	srv, err := h.store.Servers.GetByID(ctx, id)
	if err != nil || srv == nil {
		return echo.NewHTTPError(http.StatusNotFound, "server not found")
	}
	if !srv.AgentURL.Valid || srv.AgentURL.String == "" {
		return echo.NewHTTPError(http.StatusConflict, "the server has no agent URL")
	}
	if srv.AgentTLSFingerprint != "" && !agenttls.InsecureTransport(srv.AgentURL.String) {
		return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{"agent_url": srv.AgentURL.String, "fingerprint": srv.AgentTLSFingerprint, "already": true}, RequestID: rid})
	}
	if !agenttls.InsecureTransport(srv.AgentURL.String) {
		return echo.NewHTTPError(http.StatusConflict, "this agent is not a remote plain-http agent")
	}
	newURL, fp, err := agenttls.Upgrade(ctx, agentClient(20*time.Second), srv.AgentURL.String, srv.AgentToken, srv.IPAddress.String)
	if err != nil {
		if errors.Is(err, agenttls.ErrAgentTooOld) {
			return echo.NewHTTPError(http.StatusConflict, err.Error())
		}
		return echo.NewHTTPError(http.StatusBadGateway, err.Error())
	}
	if err := h.store.Servers.SetAgentTLS(ctx, srv.ID, newURL, fp); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	var uid int64
	if sess := sessionFromEcho(c); sess != nil {
		uid = sess.UserID
	}
	h.audit.RecordAction(ctx, uid, "agent.tls_enabled", audit.ResourceServer, srv.ID, map[string]any{
		"server": srv.Name, "agent_url": newURL, "fingerprint": fp,
	})
	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{"agent_url": newURL, "fingerprint": fp}, RequestID: rid})
}
