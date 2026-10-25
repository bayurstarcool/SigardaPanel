package apihttp

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/audit"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/store"
)

// ─── Threat Monitor ─────────────────────────────────────────────────────────
//
// Read-only compromise indicators collected by the agent (SSH brute-force
// activity, integrity of critical files, suspicious outbound connections,
// persistence entries, web-attack patterns) plus a baseline-compare flow for
// file integrity. All routes are admin-only.

type securityMonitorHandler struct {
	store *store.Store
	audit *audit.Recorder
}

// integrityEntry mirrors the agent's integrity collector JSON.
type integrityEntry struct {
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
	SHA256 string `json:"sha256"`
	Mode   string `json:"mode"`
	UID    int    `json:"uid"`
	Size   int64  `json:"size"`
}

func newSecurityMonitorHandler(s *store.Store, a *audit.Recorder) *securityMonitorHandler {
	return &securityMonitorHandler{store: s, audit: a}
}

// agentServer resolves the server and returns its agent base URL + token.
func (h *securityMonitorHandler) agentServer(c echo.Context) (string, string, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return "", "", echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	server, err := h.store.Servers.GetByID(c.Request().Context(), id)
	if err != nil || server == nil {
		return "", "", echo.NewHTTPError(http.StatusNotFound, "server not found")
	}
	if !server.AgentURL.Valid || server.AgentURL.String == "" || server.AgentToken == "" {
		return "", "", echo.NewHTTPError(http.StatusBadRequest, "server has no reachable agent")
	}
	return server.AgentURL.String, server.AgentToken, nil
}

func fetchAgentJSON(c echo.Context, agentURL, token, path string) (map[string]any, error) {
	resp, err := agentRequest(http.MethodGet, agentURL+path, token, nil)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusBadGateway, "agent unreachable")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusBadGateway, "agent response error")
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, echo.NewHTTPError(http.StatusBadGateway, "agent response parse error")
	}
	return out, nil
}

// GET /api/v1/servers/:id/security/monitor?section=all|ssh|integrity|connections|persistence|web
func (h *securityMonitorHandler) monitor(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	agentURL, token, err := h.agentServer(c)
	if err != nil {
		return err
	}

	section := c.QueryParam("section")
	if section == "" {
		section = "all"
	}
	sections := map[string]string{
		"ssh":         "/tasks/security/ssh-activity",
		"integrity":   "/tasks/security/integrity",
		"connections": "/tasks/security/connections",
		"persistence": "/tasks/security/persistence",
		"web":         "/tasks/security/web-attacks",
	}

	data := map[string]any{}
	if section == "all" {
		// Run collectors sequentially: this is an interactive dashboard
		// request, not a hot path, and it keeps agent load predictable.
		for name, path := range sections {
			out, err := fetchAgentJSON(c, agentURL, token, path)
			if err != nil {
				data[name] = map[string]any{"error": "collector failed"}
				continue
			}
			data[name] = out["data"]
		}
	} else {
		path, ok := sections[section]
		if !ok {
			return echo.NewHTTPError(http.StatusBadRequest, "unknown section")
		}
		out, err := fetchAgentJSON(c, agentURL, token, path)
		if err != nil {
			return err
		}
		data[section] = out["data"]
	}

	return c.JSON(http.StatusOK, httpjson.Success{Data: data, RequestID: rid})
}

type integrityDiff struct {
	Path    string `json:"path"`
	Status  string `json:"status"` // changed | new | missing | ok
	OldHash string `json:"old_sha256,omitempty"`
	NewHash string `json:"new_sha256,omitempty"`
}

type baselineCompare struct {
	BaselineCount int             `json:"baseline_count"`
	CurrentCount  int             `json:"current_count"`
	Diffs         []integrityDiff `json:"diffs"`
}

// GET /api/v1/servers/:id/security/baseline — compare live integrity against
// the stored baseline. Without a baseline the response says so.
func (h *securityMonitorHandler) getBaseline(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	serverID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || serverID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	baseline, err := h.store.SecurityBaselines.ListByServer(c.Request().Context(), serverID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if len(baseline) == 0 {
		return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{
			"has_baseline": false,
		}, RequestID: rid})
	}

	agentURL, token, err := h.agentServer(c)
	if err != nil {
		return err
	}
	out, err := fetchAgentJSON(c, agentURL, token, "/tasks/security/integrity")
	if err != nil {
		return err
	}
	current := parseIntegrityEntries(out["data"])

	baseByPath := map[string]store.SecurityBaseline{}
	for _, b := range baseline {
		baseByPath[b.Path] = b
	}
	curByPath := map[string]integrityEntry{}
	for _, e := range current {
		curByPath[e.Path] = e
	}

	compare := baselineCompare{BaselineCount: len(baseline), CurrentCount: len(current), Diffs: []integrityDiff{}}
	for path, b := range baseByPath {
		cur, ok := curByPath[path]
		switch {
		case !ok || !cur.Exists:
			compare.Diffs = append(compare.Diffs, integrityDiff{Path: path, Status: "missing", OldHash: b.SHA256})
		case cur.SHA256 != b.SHA256:
			compare.Diffs = append(compare.Diffs, integrityDiff{Path: path, Status: "changed", OldHash: b.SHA256, NewHash: cur.SHA256})
		}
	}
	for path := range curByPath {
		if _, ok := baseByPath[path]; !ok {
			compare.Diffs = append(compare.Diffs, integrityDiff{Path: path, Status: "new"})
		}
	}

	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{
		"has_baseline": true,
		"compare":      compare,
	}, RequestID: rid})
}

// POST /api/v1/servers/:id/security/baseline — snapshot current integrity as
// the known-good baseline.
func (h *securityMonitorHandler) saveBaseline(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	serverID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || serverID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	agentURL, token, err := h.agentServer(c)
	if err != nil {
		return err
	}
	out, err := fetchAgentJSON(c, agentURL, token, "/tasks/security/integrity")
	if err != nil {
		return err
	}
	current := parseIntegrityEntries(out["data"])
	if len(current) == 0 {
		return echo.NewHTTPError(http.StatusBadGateway, "agent returned no integrity data")
	}

	entries := make([]store.SecurityBaseline, 0, len(current))
	for _, e := range current {
		if !e.Exists {
			continue
		}
		entries = append(entries, store.SecurityBaseline{
			ServerID: serverID, Path: e.Path, SHA256: e.SHA256, Mode: e.Mode, UID: e.UID, Size: e.Size,
		})
	}
	if err := h.store.SecurityBaselines.ReplaceForServer(c.Request().Context(), serverID, entries); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	var userID int64
	if sess != nil {
		userID = sess.UserID
	}
	h.audit.RecordAction(c.Request().Context(), userID, "security_baseline_save", "server", serverID, map[string]any{
		"files": len(entries),
	})
	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{"saved": true, "files": len(entries)}, RequestID: rid,
	})
}

func parseIntegrityEntries(v any) []integrityEntry {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var out []integrityEntry
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}
