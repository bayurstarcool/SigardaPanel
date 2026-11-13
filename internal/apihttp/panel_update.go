package apihttp

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/audit"
	"sigardapanel/internal/auth"
	"sigardapanel/internal/buildinfo"
	"sigardapanel/internal/config"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/selfupdate"
	"sigardapanel/internal/store"
)

// Panel self-update: the API finds the latest release and asks the panel
// host's own agent (root) to install it. The agent runs the installer that is
// embedded in the binary, which verifies the release signature, backs up the
// binary, web bundle and database and rolls back on failure. The API, web and
// agent restart during the update, so clients poll the status until the
// reported version changes.

const (
	settingPanelAutoUpdate   = "panel_auto_update"
	settingPanelNotifiedTag  = "panel_update_notified_tag"
	settingPanelAutoTriedTag = "panel_update_auto_tag"
)

// panelReleases caches the latest-release lookup for the whole process.
var panelReleases = selfupdate.NewChecker()

// panelUpdateAgent returns the panel host's agent (the one in panel.env).
// Tests replace it.
var panelUpdateAgent = func() (string, string) {
	return strings.TrimRight(config.BackendAgentURL(), "/"), config.BackendAgentToken()
}

func panelVersion() string { return buildinfo.Version }

type panelUpdateHandler struct {
	store *store.Store
	db    *sql.DB
	audit *audit.Recorder
}

// agentStatus asks the local agent for the update status.
func panelAgentStatus(ctx context.Context) (map[string]any, error) {
	base, token := panelUpdateAgent()
	if !isLoopbackAgentURL(base) {
		return nil, fmt.Errorf("The panel's agent (%s) is not on this host; self-update needs the local agent.", base)
	}
	code, out, err := stackAgentRequest(ctx, http.MethodGet, base+"/tasks/panel/update", token, nil, 15*time.Second)
	if err != nil {
		return nil, fmt.Errorf("The local agent is unreachable: %w", err)
	}
	if code == http.StatusNotFound {
		return nil, fmt.Errorf("The local agent does not support self-update yet; update once with: sudo bash install.sh update")
	}
	if code != http.StatusOK {
		return nil, agentHTTPError(code, out)
	}
	return out, nil
}

func panelAgentStart(ctx context.Context, tag string) (int, map[string]any, error) {
	base, token := panelUpdateAgent()
	if !isLoopbackAgentURL(base) {
		return 0, nil, fmt.Errorf("The panel's agent (%s) is not on this host; self-update needs the local agent.", base)
	}
	return stackAgentRequest(ctx, http.MethodPost, base+"/tasks/panel/update", token, map[string]string{"tag": tag}, 90*time.Second)
}

func autoUpdateEnabled(ctx context.Context, db *sql.DB) bool {
	v, _ := store.GetSetting(ctx, db, settingPanelAutoUpdate)
	return v == "true"
}

// status answers GET /api/v1/system/panel-update[?refresh=1].
func (h *panelUpdateHandler) status(c echo.Context) error {
	ctx := c.Request().Context()
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	maxAge := 6 * time.Hour
	if c.QueryParam("refresh") == "1" {
		maxAge = 0
	}
	current := panelVersion()
	rel, checkedAt, checkErr := panelReleases.Latest(ctx, maxAge)

	data := map[string]any{
		"current_version":  current,
		"is_release_build": selfupdate.IsRelease(current),
		"latest":           rel,
		"update_available": rel != nil && selfupdate.Newer(rel.Tag, current),
		"auto_update":      autoUpdateEnabled(ctx, h.db),
		"repository":       "https://github.com/" + selfupdate.Repo,
	}
	if !checkedAt.IsZero() {
		data["checked_at"] = checkedAt.UTC().Format(time.RFC3339)
	}
	if checkErr != nil {
		data["check_error"] = checkErr.Error()
	}
	if st, err := panelAgentStatus(ctx); err != nil {
		data["agent_error"] = err.Error()
	} else {
		data["status"] = st
	}
	return c.JSON(http.StatusOK, httpjson.Success{Data: data, RequestID: rid})
}

// start answers POST /api/v1/system/panel-update: install the latest release.
func (h *panelUpdateHandler) start(c echo.Context) error {
	ctx := c.Request().Context()
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	current := panelVersion()
	if !selfupdate.IsRelease(current) {
		return echo.NewHTTPError(http.StatusConflict, "this panel is a development build ("+current+"); only release installs can update themselves")
	}
	rel, _, err := panelReleases.Latest(ctx, 10*time.Minute)
	if rel == nil {
		msg := "could not find the latest release"
		if err != nil {
			msg += ": " + err.Error()
		}
		return echo.NewHTTPError(http.StatusBadGateway, msg)
	}
	if !selfupdate.Newer(rel.Tag, current) {
		return echo.NewHTTPError(http.StatusConflict, "already on the latest release ("+current+")")
	}
	code, out, err := panelAgentStart(ctx, rel.Tag)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, err.Error())
	}
	var uid int64
	if sess != nil {
		uid = sess.UserID
	}
	if code != http.StatusAccepted {
		h.audit.RecordAction(ctx, uid, "panel.update_refused", "panel", 0, map[string]any{
			"from": current, "to": rel.Tag, "reason": agentMessage(out, "agent error"),
		})
		return agentHTTPError(code, out)
	}
	h.audit.RecordAction(ctx, uid, "panel.update", "panel", 0, map[string]any{"from": current, "to": rel.Tag})
	return c.JSON(http.StatusAccepted, httpjson.Success{Data: map[string]any{
		"message": "Updating to " + rel.Tag + ". The panel restarts during the update; this page reconnects by itself.",
		"tag":     rel.Tag,
		"status":  out["status"],
	}, RequestID: rid})
}

// settings answers PUT /api/v1/system/panel-update/settings {"auto_update": bool}.
func (h *panelUpdateHandler) settings(c echo.Context) error {
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
	if err := store.SetSetting(ctx, h.db, settingPanelAutoUpdate, val); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "could not save the setting")
	}
	if sess := sessionFromEcho(c); sess != nil {
		h.audit.RecordAction(ctx, sess.UserID, "panel.auto_update", "panel", 0, map[string]any{"enabled": *body.AutoUpdate})
	}
	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{"auto_update": *body.AutoUpdate}, RequestID: rid})
}

// notifySuperAdmins sends an in-app notification to every super_admin.
func notifySuperAdmins(ctx context.Context, s *store.Store, title, message, severity string) {
	users, err := s.Users.List(ctx, store.UserListFilter{})
	if err != nil {
		return
	}
	for _, u := range users {
		if u.Role != auth.RoleSuperAdmin {
			continue
		}
		_ = s.Notifications.Create(ctx, &store.Notification{
			UserID: u.ID, Type: "panel_update", Title: title, Message: message, Severity: severity, EntityType: "panel",
		})
	}
}

// panelUpdateTick checks for a new release once: it notifies the super
// admins about it (once per release) and, with auto-update on, installs it
// (one attempt per release, so a failing release is not retried forever).
func panelUpdateTick(ctx context.Context, s *store.Store, db *sql.DB, rec *audit.Recorder) {
	current := panelVersion()
	if !selfupdate.IsRelease(current) {
		return
	}
	rel, _, err := panelReleases.Latest(ctx, time.Hour)
	if err != nil || rel == nil || !selfupdate.Newer(rel.Tag, current) {
		return
	}
	auto := autoUpdateEnabled(ctx, db)
	if notified, _ := store.GetSetting(ctx, db, settingPanelNotifiedTag); notified != rel.Tag && !auto {
		_ = store.SetSetting(ctx, db, settingPanelNotifiedTag, rel.Tag)
		notifySuperAdmins(ctx, s, "SigardaPanel "+rel.Tag+" is available",
			"You are on "+current+". Install it from Settings → Panel update.", "info")
	}
	if !auto {
		return
	}
	if tried, _ := store.GetSetting(ctx, db, settingPanelAutoTriedTag); tried == rel.Tag {
		return
	}
	st, err := panelAgentStatus(ctx)
	if err != nil {
		slog.Warn("panel auto-update: agent status", "error", err)
		return
	}
	if supported, _ := st["supported"].(bool); !supported {
		slog.Warn("panel auto-update: not supported on this host", "reason", st["unsupported_reason"])
		return
	}
	if state, _ := st["state"].(string); state == "running" {
		return
	}
	// Record the attempt first: the update restarts this process.
	_ = store.SetSetting(ctx, db, settingPanelAutoTriedTag, rel.Tag)
	code, out, err := panelAgentStart(ctx, rel.Tag)
	if err != nil || code != http.StatusAccepted {
		reason := agentMessage(out, fmt.Sprint(err))
		slog.Warn("panel auto-update: start failed", "tag", rel.Tag, "reason", reason)
		rec.RecordAction(ctx, 0, "panel.auto_update_failed", "panel", 0, map[string]any{"from": current, "to": rel.Tag, "reason": reason})
		notifySuperAdmins(ctx, s, "Automatic update to "+rel.Tag+" did not start", reason, "warning")
		return
	}
	slog.Info("panel auto-update started", "from", current, "to", rel.Tag)
	rec.RecordAction(ctx, 0, "panel.auto_update", "panel", 0, map[string]any{"from": current, "to": rel.Tag})
	notifySuperAdmins(ctx, s, "Installing SigardaPanel "+rel.Tag+" automatically",
		"Updating from "+current+". The panel restarts for a moment; it rolls back by itself if the new version is not healthy.", "info")
}

// StartPanelUpdateChecker runs panelUpdateTick shortly after start and then
// every hour until ctx ends.
func StartPanelUpdateChecker(ctx context.Context, s *store.Store, db *sql.DB) {
	rec := audit.NewRecorder(db)
	go func() {
		timer := time.NewTimer(3 * time.Minute)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				func() {
					defer func() {
						if r := recover(); r != nil {
							slog.Error("panel update checker panic", "error", r)
						}
					}()
					tctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
					defer cancel()
					panelUpdateTick(tctx, s, db, rec)
				}()
				timer.Reset(time.Hour)
			}
		}
	}()
}
