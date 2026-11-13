package apihttp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/audit"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/selfupdate"
	"sigardapanel/internal/store"
)

// Remote agent updates: every remote server's agent can update itself to the
// panel's own version (agents and panel speak the same protocol). The agent
// runs the embedded agent-update.sh (signature check, binary backup,
// automatic restore) in its own systemd unit. The panel host's agent is the
// panel binary and is updated together with the panel.

const settingAgentAutoUpdate = "agent_auto_update"

func agentTriedKey(id int64) string    { return "agent_update_tried_" + strconv.FormatInt(id, 10) }
func agentReportedKey(id int64) string { return "agent_update_reported_" + strconv.FormatInt(id, 10) }

// agentTargetTag is the release remote agents are updated to: the panel's
// version ("" for development builds).
func agentTargetTag() string {
	if !selfupdate.IsRelease(panelVersion()) {
		return ""
	}
	return selfupdate.Tag(panelVersion())
}

var errAgentTooOld = errors.New("this agent is too old to update from the panel; update it once with the agent install command (Servers → Add server), later updates work from here")

type agentUpdateHandler struct {
	store *store.Store
	db    *sql.DB
	audit *audit.Recorder
}

func agentAutoUpdateEnabled(ctx context.Context, db *sql.DB) bool {
	v, _ := store.GetSetting(ctx, db, settingAgentAutoUpdate)
	return v == "true"
}

// remoteAgentStatus asks a remote agent for its updater status.
func remoteAgentStatus(ctx context.Context, srv *store.Server) (map[string]any, error) {
	if !srv.AgentURL.Valid || srv.AgentURL.String == "" {
		return nil, errors.New("the server has no agent URL")
	}
	code, out, err := stackAgentRequest(ctx, http.MethodGet, agentBase(srv)+"/tasks/agent/update", srv.AgentToken, nil, 8*time.Second)
	if code == http.StatusNotFound {
		return nil, errAgentTooOld // an agent without the updater (any 404 body)
	}
	if err != nil {
		return nil, fmt.Errorf("agent unreachable: %w", err)
	}
	if code != http.StatusOK {
		return nil, agentHTTPError(code, out)
	}
	return out, nil
}

type agentUpdateRow struct {
	ServerID        int64          `json:"server_id"`
	Name            string         `json:"name"`
	Hostname        string         `json:"hostname"`
	Status          string         `json:"status"`
	IsLocal         bool           `json:"is_local"`
	AgentVersion    string         `json:"agent_version"`
	UpdateAvailable bool           `json:"update_available"`
	Update          map[string]any `json:"update,omitempty"`
	Error           string         `json:"error,omitempty"`
}

func agentRow(ctx context.Context, srv store.Server, target string) agentUpdateRow {
	row := agentUpdateRow{ServerID: srv.ID, Name: srv.Name, Hostname: srv.Hostname, Status: srv.Status, IsLocal: isLocalServer(srv)}
	if srv.AgentVersion.Valid {
		row.AgentVersion = srv.AgentVersion.String
	}
	if row.IsLocal {
		row.AgentVersion = panelVersion()
		return row
	}
	st, err := remoteAgentStatus(ctx, &srv)
	if err != nil {
		row.Error = err.Error()
	} else {
		row.Update = st
		if v, _ := st["agent_version"].(string); v != "" {
			row.AgentVersion = v
		}
	}
	row.UpdateAvailable = target != "" && selfupdate.Newer(target, row.AgentVersion)
	return row
}

// forEachServer runs fn for every server with at most 6 at a time.
func forEachServer(servers []store.Server, fn func(i int, srv store.Server)) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)
	for i, srv := range servers {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, srv store.Server) {
			defer wg.Done()
			defer func() { <-sem }()
			fn(i, srv)
		}(i, srv)
	}
	wg.Wait()
}

// list answers GET /api/v1/system/agent-updates.
func (h *agentUpdateHandler) list(c echo.Context) error {
	ctx := c.Request().Context()
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	servers, err := h.store.Servers.List(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "could not list servers")
	}
	target := agentTargetTag()
	rows := make([]agentUpdateRow, len(servers))
	forEachServer(servers, func(i int, srv store.Server) { rows[i] = agentRow(ctx, srv, target) })
	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{
		"target_version": target,
		"auto_update":    agentAutoUpdateEnabled(ctx, h.db),
		"servers":        rows,
	}, RequestID: rid})
}

// startAgentUpdate asks srv's agent to update to the panel's version.
func startAgentUpdate(ctx context.Context, srv *store.Server, auto bool) (int, string) {
	if isLocalServer(*srv) {
		return http.StatusConflict, "the panel host's agent is updated together with the panel"
	}
	target := agentTargetTag()
	if target == "" {
		return http.StatusConflict, "the panel is a development build (" + panelVersion() + "); agents can only be updated to a release"
	}
	if !srv.AgentURL.Valid || srv.AgentURL.String == "" {
		return http.StatusConflict, "the server has no agent URL"
	}
	code, out, err := stackAgentRequest(ctx, http.MethodPost, agentBase(srv)+"/tasks/agent/update", srv.AgentToken,
		map[string]any{"tag": target, "auto": auto}, 25*time.Second)
	if code == http.StatusNotFound && (out == nil || agentMessage(out, "Not Found") == "Not Found") {
		return http.StatusConflict, errAgentTooOld.Error()
	}
	if err != nil {
		return http.StatusBadGateway, "agent unreachable: " + err.Error()
	}
	if code == http.StatusUnauthorized || code == http.StatusForbidden {
		return http.StatusBadGateway, "the server's agent rejected the panel's token (regenerate it under Servers)"
	}
	if code != http.StatusAccepted {
		return code, agentMessage(out, "agent error "+strconv.Itoa(code))
	}
	return http.StatusAccepted, "agent update to " + target + " started"
}

// startOne answers POST /api/v1/servers/:id/agent-update.
func (h *agentUpdateHandler) startOne(c echo.Context) error {
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
	code, msg := startAgentUpdate(ctx, srv, false)
	h.recordStart(c, srv, code, msg)
	if code != http.StatusAccepted {
		return echo.NewHTTPError(code, msg)
	}
	watchAgentUpdate(h.store, h.db, srv.ID)
	return c.JSON(http.StatusAccepted, httpjson.Success{Data: map[string]any{"message": msg, "tag": agentTargetTag()}, RequestID: rid})
}

func (h *agentUpdateHandler) recordStart(c echo.Context, srv *store.Server, code int, msg string) {
	var uid int64
	if sess := sessionFromEcho(c); sess != nil {
		uid = sess.UserID
	}
	action := "agent.update"
	if code != http.StatusAccepted {
		action = "agent.update_refused"
	}
	h.audit.RecordAction(c.Request().Context(), uid, action, audit.ResourceServer, srv.ID, map[string]any{
		"server": srv.Name, "to": agentTargetTag(), "result": msg,
	})
}

// startMany answers POST /api/v1/system/agent-updates
// {"server_ids": [1, 2]} or {"all": true} (every remote server whose agent
// is older than the panel).
func (h *agentUpdateHandler) startMany(c echo.Context) error {
	ctx := c.Request().Context()
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	var body struct {
		ServerIDs []int64 `json:"server_ids"`
		All       bool    `json:"all"`
	}
	if err := c.Bind(&body); err != nil || (!body.All && len(body.ServerIDs) == 0) {
		return echo.NewHTTPError(http.StatusBadRequest, "send server_ids or all: true")
	}
	target := agentTargetTag()
	if target == "" {
		return echo.NewHTTPError(http.StatusConflict, "the panel is a development build; agents can only be updated to a release")
	}
	servers, err := h.store.Servers.List(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "could not list servers")
	}
	want := map[int64]bool{}
	for _, id := range body.ServerIDs {
		want[id] = true
	}
	var picked []store.Server
	for _, srv := range servers {
		if isLocalServer(srv) {
			continue
		}
		if body.All {
			if srv.AgentVersion.Valid && !selfupdate.Newer(target, srv.AgentVersion.String) {
				continue // already current
			}
		} else if !want[srv.ID] {
			continue
		}
		picked = append(picked, srv)
	}
	type result struct {
		ServerID int64  `json:"server_id"`
		Name     string `json:"name"`
		OK       bool   `json:"ok"`
		Message  string `json:"message"`
	}
	results := make([]result, len(picked))
	forEachServer(picked, func(i int, srv store.Server) {
		code, msg := startAgentUpdate(ctx, &srv, false)
		h.recordStart(c, &srv, code, msg)
		if code == http.StatusAccepted {
			watchAgentUpdate(h.store, h.db, srv.ID)
		}
		results[i] = result{ServerID: srv.ID, Name: srv.Name, OK: code == http.StatusAccepted, Message: msg}
	})
	started := 0
	for _, r := range results {
		if r.OK {
			started++
		}
	}
	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{
		"target_version": target, "started": started, "results": results,
	}, RequestID: rid})
}

// settings answers PUT /api/v1/system/agent-updates/settings {"auto_update": bool}.
func (h *agentUpdateHandler) settings(c echo.Context) error {
	ctx := c.Request().Context()
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	var body struct {
		AutoUpdate *bool `json:"auto_update"`
	}
	if err := c.Bind(&body); err != nil || body.AutoUpdate == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "auto_update (true/false) is required")
	}
	val := "false"
	if *body.AutoUpdate {
		val = "true"
	}
	if err := store.SetSetting(ctx, h.db, settingAgentAutoUpdate, val); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "could not save the setting")
	}
	if sess := sessionFromEcho(c); sess != nil {
		h.audit.RecordAction(ctx, sess.UserID, "agent.auto_update", "panel", 0, map[string]any{"enabled": *body.AutoUpdate})
	}
	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{"auto_update": *body.AutoUpdate}, RequestID: rid})
}

// agentUpdateReport announces a finished agent update on srv once.
func agentUpdateReport(ctx context.Context, s *store.Store, db *sql.DB, srv *store.Server, st map[string]any) {
	state, _ := st["state"].(string)
	started, _ := st["started_at"].(string)
	if (state != "succeeded" && state != "failed") || started == "" {
		return
	}
	if done, _ := store.GetSetting(ctx, db, agentReportedKey(srv.ID)); done == started {
		return
	}
	_ = store.SetSetting(ctx, db, agentReportedKey(srv.ID), started)
	tag, _ := st["tag"].(string)
	if state == "succeeded" {
		notifySuperAdmins(ctx, s, "Agent on "+srv.Name+" updated to "+tag, "The agent restarted on the new version.", "success")
		return
	}
	msg, _ := st["message"].(string)
	notifySuperAdmins(ctx, s, "Agent update on "+srv.Name+" failed", msg+" Details: Settings → Panel update → Server agents.", "error")
}

var agentWatching sync.Map // server id -> struct{}

// watchAgentUpdate follows a started agent update and announces its result.
func watchAgentUpdate(s *store.Store, db *sql.DB, serverID int64) {
	if _, busy := agentWatching.LoadOrStore(serverID, struct{}{}); busy {
		return
	}
	base, interval := panelWatchCtx, panelUpdateWatchInterval
	panelWatchWG.Add(1)
	go func() {
		defer panelWatchWG.Done()
		defer agentWatching.Delete(serverID)
		deadline := time.Now().Add(50 * time.Minute)
		for time.Now().Before(deadline) {
			select {
			case <-base.Done():
				return
			case <-time.After(interval):
			}
			ctx, cancel := context.WithTimeout(base, 30*time.Second)
			srv, err := s.Servers.GetByID(ctx, serverID)
			if err != nil || srv == nil {
				cancel()
				return
			}
			if st, err := remoteAgentStatus(ctx, srv); err == nil {
				if state, _ := st["state"].(string); state != "running" {
					agentUpdateReport(ctx, s, db, srv, st)
					cancel()
					return
				}
			}
			cancel()
		}
	}()
}

// agentUpdateTick announces finished agent updates and, with automatic
// agent updates on, updates every remote agent older than the panel (one
// attempt per server and release).
func agentUpdateTick(ctx context.Context, s *store.Store, db *sql.DB, rec *audit.Recorder) {
	servers, err := s.Servers.List(ctx)
	if err != nil {
		return
	}
	target := agentTargetTag()
	auto := agentAutoUpdateEnabled(ctx, db)
	forEachServer(servers, func(_ int, srv store.Server) {
		if isLocalServer(srv) {
			return
		}
		sctx, cancel := context.WithTimeout(ctx, 40*time.Second)
		defer cancel()
		st, err := remoteAgentStatus(sctx, &srv)
		if err != nil {
			return
		}
		agentUpdateReport(sctx, s, db, &srv, st)
		if !auto || target == "" {
			return
		}
		version, _ := st["agent_version"].(string)
		if version == "" && srv.AgentVersion.Valid {
			version = srv.AgentVersion.String
		}
		if !selfupdate.Newer(target, version) {
			return
		}
		if supported, _ := st["supported"].(bool); !supported {
			return
		}
		if state, _ := st["state"].(string); state == "running" {
			return
		}
		if tried, _ := store.GetSetting(sctx, db, agentTriedKey(srv.ID)); tried == target {
			return
		}
		_ = store.SetSetting(sctx, db, agentTriedKey(srv.ID), target)
		code, msg := startAgentUpdate(sctx, &srv, true)
		if code != http.StatusAccepted {
			slog.Warn("agent auto-update: start failed", "server", srv.Name, "reason", msg)
			rec.RecordAction(sctx, 0, "agent.auto_update_failed", audit.ResourceServer, srv.ID, map[string]any{"server": srv.Name, "to": target, "reason": msg})
			notifySuperAdmins(sctx, s, "Automatic agent update on "+srv.Name+" did not start", msg, "warning")
			return
		}
		slog.Info("agent auto-update started", "server", srv.Name, "from", version, "to", target)
		rec.RecordAction(sctx, 0, "agent.auto_update", audit.ResourceServer, srv.ID, map[string]any{"server": srv.Name, "from": version, "to": target})
		watchAgentUpdate(s, db, srv.ID)
	})
}
