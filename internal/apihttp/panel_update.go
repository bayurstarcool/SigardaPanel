package apihttp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/labstack/echo/v5"
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
	// "start-end" hours (server local time, end exclusive, may wrap past
	// midnight); empty = any time.
	settingPanelAutoWindow = "panel_auto_update_window"
	// started_at of the last run whose result was announced.
	settingPanelReportedRun = "panel_update_reported_run"
)

// panelUpdateNow is the clock of the auto-update window. Tests replace it.
var panelUpdateNow = time.Now

type autoWindow struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

func parseAutoWindow(v string) *autoWindow {
	var w autoWindow
	if _, err := fmt.Sscanf(v, "%d-%d", &w.Start, &w.End); err != nil {
		return nil
	}
	if w.Start < 0 || w.Start > 23 || w.End < 0 || w.End > 23 || w.Start == w.End {
		return nil
	}
	return &w
}

func loadAutoWindow(ctx context.Context, db *sql.DB) *autoWindow {
	v, _ := store.GetSetting(ctx, db, settingPanelAutoWindow)
	return parseAutoWindow(v)
}

// contains reports whether the hour of t is inside the window.
func (w *autoWindow) contains(t time.Time) bool {
	if w == nil {
		return true
	}
	h := t.Hour()
	if w.Start < w.End {
		return h >= w.Start && h < w.End
	}
	return h >= w.Start || h < w.End // wraps past midnight
}

// latestRelease bounds the GitHub lookup and detaches it from the caller,
// so a closed page does not abort (and cannot poison) the shared lookup.
func latestRelease(ctx context.Context, maxAge time.Duration) (*selfupdate.Release, time.Time, error) {
	lctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 12*time.Second)
	defer cancel()
	return panelReleases.Latest(lctx, maxAge)
}

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
	code, out, err := stackAgentRequest(ctx, http.MethodGet, base+"/tasks/panel/update", token, nil, 10*time.Second)
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

func panelAgentStart(ctx context.Context, tag string, auto bool) (int, map[string]any, error) {
	return panelAgentCall(ctx, http.MethodPost, "/tasks/panel/update", map[string]any{"tag": tag, "auto": auto})
}

// panelAgentCall calls the panel host's own agent.
func panelAgentCall(ctx context.Context, method, path string, body any) (int, map[string]any, error) {
	base, token := panelUpdateAgent()
	if !isLoopbackAgentURL(base) {
		return 0, nil, fmt.Errorf("The panel's agent (%s) is not on this host; self-update needs the local agent.", base)
	}
	return stackAgentRequest(ctx, method, base+path, token, body, 25*time.Second)
}

func autoUpdateEnabled(ctx context.Context, db *sql.DB) bool {
	v, _ := store.GetSetting(ctx, db, settingPanelAutoUpdate)
	return v == "true"
}

// status answers GET /api/v1/system/panel-update[?refresh=1].
func (h *panelUpdateHandler) status(c *echo.Context) error {
	ctx := c.Request().Context()
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	maxAge := 6 * time.Hour
	if c.QueryParam("refresh") == "1" {
		maxAge = 0
	}
	current := panelVersion()
	rel, checkedAt, checkErr := latestRelease(ctx, maxAge)

	data := map[string]any{
		"auto_update_window": loadAutoWindow(ctx, h.db),
		"server_time":        panelUpdateNow().Format(time.RFC3339),
		"current_version":    current,
		"is_release_build":   selfupdate.IsRelease(current),
		"latest":             rel,
		"update_available":   rel != nil && selfupdate.Newer(rel.Tag, current),
		"auto_update":        autoUpdateEnabled(ctx, h.db),
		"repository":         "https://github.com/" + selfupdate.Repo,
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
func (h *panelUpdateHandler) start(c *echo.Context) error {
	ctx := c.Request().Context()
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	current := panelVersion()
	if !selfupdate.IsRelease(current) {
		return echo.NewHTTPError(http.StatusConflict, "this panel is a development build ("+current+"); only release installs can update themselves")
	}
	rel, _, err := latestRelease(ctx, 10*time.Minute)
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
	code, out, err := panelAgentStart(ctx, rel.Tag, false)
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
	watchPanelUpdate(h.store, h.db)
	return c.JSON(http.StatusAccepted, httpjson.Success{Data: map[string]any{
		"message": "Updating to " + rel.Tag + ". The panel restarts during the update; this page reconnects by itself.",
		"tag":     rel.Tag,
		"status":  out["status"],
	}, RequestID: rid})
}

// settings answers PUT /api/v1/system/panel-update/settings
// {"auto_update": bool, "auto_update_window": {"start": 2, "end": 5} | null}.
// Fields left out keep their value.
func (h *panelUpdateHandler) settings(c *echo.Context) error {
	ctx := c.Request().Context()
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	var body map[string]json.RawMessage
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	rawAuto, hasAuto := body["auto_update"]
	rawWin, hasWin := body["auto_update_window"]
	if !hasAuto && !hasWin {
		return echo.NewHTTPError(http.StatusBadRequest, "nothing to change: send auto_update and/or auto_update_window")
	}
	var auto bool
	if hasAuto {
		if err := json.Unmarshal(rawAuto, &auto); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "auto_update must be true or false")
		}
	}
	winVal := ""
	if hasWin && string(rawWin) != "null" {
		var w autoWindow
		if err := json.Unmarshal(rawWin, &w); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "auto_update_window must be {start, end} or null")
		}
		winVal = fmt.Sprintf("%d-%d", w.Start, w.End)
		if parseAutoWindow(winVal) == nil {
			return echo.NewHTTPError(http.StatusBadRequest, "auto_update_window hours must be 0-23 and start must differ from end")
		}
	}
	details := map[string]any{}
	if hasAuto {
		val := "false"
		if auto {
			val = "true"
		}
		if err := store.SetSetting(ctx, h.db, settingPanelAutoUpdate, val); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "could not save the setting")
		}
		details["enabled"] = auto
	}
	if hasWin {
		if err := store.SetSetting(ctx, h.db, settingPanelAutoWindow, winVal); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "could not save the setting")
		}
		details["window"] = winVal
	}
	if sess := sessionFromEcho(c); sess != nil {
		h.audit.RecordAction(ctx, sess.UserID, "panel.auto_update", "panel", 0, details)
	}
	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{
		"auto_update":        autoUpdateEnabled(ctx, h.db),
		"auto_update_window": loadAutoWindow(ctx, h.db),
	}, RequestID: rid})
}

// summary answers GET /api/v1/system/panel-update/summary: a cheap answer
// for the header badge (cached release lookup, no agent call).
func (h *panelUpdateHandler) summary(c *echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	current := panelVersion()
	rel, _, _ := latestRelease(c.Request().Context(), 6*time.Hour)
	data := map[string]any{"current_version": current, "update_available": false}
	if rel != nil {
		data["latest_tag"] = rel.Tag
		data["update_available"] = selfupdate.Newer(rel.Tag, current)
	}
	return c.JSON(http.StatusOK, httpjson.Success{Data: data, RequestID: rid})
}

// backups answers GET /api/v1/system/panel-update/backups.
func (h *panelUpdateHandler) backups(c *echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	code, out, err := panelAgentCall(c.Request().Context(), http.MethodGet, "/tasks/panel/backups", nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, err.Error())
	}
	if code == http.StatusNotFound {
		return echo.NewHTTPError(http.StatusBadGateway, "the local agent does not support rollback yet; update the panel first")
	}
	if code != http.StatusOK {
		return agentHTTPError(code, out)
	}
	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{"backups": out["backups"]}, RequestID: rid})
}

// rollback answers POST /api/v1/system/panel-update/rollback {"backup": "..."}:
// restore the binary, web app and database saved before an update.
func (h *panelUpdateHandler) rollback(c *echo.Context) error {
	ctx := c.Request().Context()
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	var body struct {
		Backup string `json:"backup"`
	}
	if err := c.Bind(&body); err != nil || strings.TrimSpace(body.Backup) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "backup is required")
	}
	code, out, err := panelAgentCall(ctx, http.MethodPost, "/tasks/panel/rollback", map[string]string{"backup": strings.TrimSpace(body.Backup)})
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, err.Error())
	}
	var uid int64
	if sess := sessionFromEcho(c); sess != nil {
		uid = sess.UserID
	}
	if code == http.StatusNotFound && agentMessage(out, "") == "Not Found" {
		return echo.NewHTTPError(http.StatusBadGateway, "the local agent does not support rollback yet; update the panel first")
	}
	if code != http.StatusAccepted {
		h.audit.RecordAction(ctx, uid, "panel.rollback_refused", "panel", 0, map[string]any{"backup": body.Backup, "reason": agentMessage(out, "agent error")})
		return agentHTTPError(code, out)
	}
	st, _ := out["status"].(map[string]any)
	tag, _ := st["tag"].(string)
	h.audit.RecordAction(ctx, uid, "panel.rollback", "panel", 0, map[string]any{"from": panelVersion(), "to": tag, "backup": body.Backup})
	watchPanelUpdate(h.store, h.db)
	return c.JSON(http.StatusAccepted, httpjson.Success{Data: map[string]any{
		"message": "Rolling back to " + tag + ". The panel restarts; this page reconnects by itself.",
		"tag":     tag,
		"status":  st,
	}, RequestID: rid})
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

// panelUpdateReport announces the result of the latest update run (manual
// or automatic) to the super admins, once per run.
func panelUpdateReport(ctx context.Context, s *store.Store, db *sql.DB) {
	st, err := panelAgentStatus(ctx)
	if err != nil {
		return
	}
	state, _ := st["state"].(string)
	started, _ := st["started_at"].(string)
	if (state != "succeeded" && state != "failed") || started == "" {
		return
	}
	if done, _ := store.GetSetting(ctx, db, settingPanelReportedRun); done == started {
		return
	}
	_ = store.SetSetting(ctx, db, settingPanelReportedRun, started)
	tag, _ := st["tag"].(string)
	action, _ := st["action"].(string)
	msg, _ := st["message"].(string)
	switch {
	case state == "succeeded" && action == "rollback":
		notifySuperAdmins(ctx, s, "SigardaPanel rolled back to "+tag, "The panel is running "+panelVersion()+" again. Automatic updates skip the release it was rolled back from.", "warning")
	case state == "succeeded":
		notifySuperAdmins(ctx, s, "SigardaPanel updated to "+tag, "The update finished and the panel is running "+panelVersion()+".", "success")
	case action == "rollback":
		notifySuperAdmins(ctx, s, "Rollback to "+tag+" failed", msg+" Details: Settings → Panel update.", "error")
	default:
		notifySuperAdmins(ctx, s, "Update to "+tag+" failed", msg+" Details: Settings → Panel update.", "error")
	}
}

// panelUpdateWatching is set while a goroutine follows a started update.
var (
	panelUpdateWatching atomic.Bool
	// panelWatchCtx ends the watcher (tests cancel it; production never does).
	panelWatchCtx = context.Background()
	panelWatchWG  sync.WaitGroup
)

// watchPanelUpdate follows an update this process started and announces its
// result as soon as the agent reports one. When the update restarts the API
// this goroutine dies with it; the checker's first tick after the restart
// announces the result instead.
func watchPanelUpdate(s *store.Store, db *sql.DB) {
	if !panelUpdateWatching.CompareAndSwap(false, true) {
		return
	}
	base, interval := panelWatchCtx, panelUpdateWatchInterval
	panelWatchWG.Add(1)
	go func() {
		defer panelWatchWG.Done()
		defer panelUpdateWatching.Store(false)
		deadline := time.Now().Add(50 * time.Minute)
		for time.Now().Before(deadline) {
			select {
			case <-base.Done():
				return
			case <-time.After(interval):
			}
			ctx, cancel := context.WithTimeout(base, 30*time.Second)
			st, err := panelAgentStatus(ctx)
			if err == nil {
				if state, _ := st["state"].(string); state != "running" {
					panelUpdateReport(ctx, s, db)
					cancel()
					return
				}
			}
			cancel()
		}
	}()
}

var panelUpdateWatchInterval = 20 * time.Second

// panelUpdateTick checks for a new release once: it notifies the super
// admins about it (once per release) and, with auto-update on and inside the
// update window, installs it (one attempt per release, so a failing release
// is not retried forever).
func panelUpdateTick(ctx context.Context, s *store.Store, db *sql.DB, rec *audit.Recorder) {
	panelUpdateReport(ctx, s, db)
	current := panelVersion()
	if !selfupdate.IsRelease(current) {
		return
	}
	rel, _, err := latestRelease(ctx, time.Hour)
	if err != nil || rel == nil || !selfupdate.Newer(rel.Tag, current) {
		return
	}
	notify := func() {
		if notified, _ := store.GetSetting(ctx, db, settingPanelNotifiedTag); notified != rel.Tag {
			_ = store.SetSetting(ctx, db, settingPanelNotifiedTag, rel.Tag)
			notifySuperAdmins(ctx, s, "SigardaPanel "+rel.Tag+" is available",
				"You are on "+current+". Install it from Settings → Panel update.", "info")
		}
	}
	if !autoUpdateEnabled(ctx, db) {
		notify()
		return
	}
	if tried, _ := store.GetSetting(ctx, db, settingPanelAutoTriedTag); tried == rel.Tag {
		return
	}
	if w := loadAutoWindow(ctx, db); !w.contains(panelUpdateNow()) {
		return // try again at the next tick inside the window
	}
	st, err := panelAgentStatus(ctx)
	if err != nil {
		slog.Warn("panel auto-update: agent status", "error", err)
		notify()
		return
	}
	if supported, _ := st["supported"].(bool); !supported {
		slog.Warn("panel auto-update: not supported on this host", "reason", st["unsupported_reason"])
		notify()
		return
	}
	if state, _ := st["state"].(string); state == "running" {
		return
	}
	if hold, _ := st["hold_tag"].(string); hold != "" && !selfupdate.Newer(rel.Tag, hold) {
		// The panel was rolled back from this release: wait for a newer one.
		notify()
		return
	}
	// Record the attempt first: the update restarts this process.
	_ = store.SetSetting(ctx, db, settingPanelAutoTriedTag, rel.Tag)
	code, out, err := panelAgentStart(ctx, rel.Tag, true)
	if err != nil || code != http.StatusAccepted {
		reason := agentMessage(out, fmt.Sprint(err))
		slog.Warn("panel auto-update: start failed", "tag", rel.Tag, "reason", reason)
		rec.RecordAction(ctx, 0, "panel.auto_update_failed", "panel", 0, map[string]any{"from": current, "to": rel.Tag, "reason": reason})
		notifySuperAdmins(ctx, s, "Automatic update to "+rel.Tag+" did not start", reason, "warning")
		return
	}
	slog.Info("panel auto-update started", "from", current, "to", rel.Tag)
	watchPanelUpdate(s, db)
	rec.RecordAction(ctx, 0, "panel.auto_update", "panel", 0, map[string]any{"from": current, "to": rel.Tag})
	notifySuperAdmins(ctx, s, "Installing SigardaPanel "+rel.Tag+" automatically",
		"Updating from "+current+". The panel restarts for a moment; it rolls back by itself if the new version is not healthy.", "info")
}

// StartPanelUpdateChecker runs panelUpdateTick shortly after start and then
// every hour until ctx ends.
func StartPanelUpdateChecker(ctx context.Context, s *store.Store, db *sql.DB) {
	rec := audit.NewRecorder(db)
	go func() {
		// Soon after start, so the result of an update that just restarted
		// the API is announced quickly; then hourly.
		timer := time.NewTimer(time.Minute)
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
					agentUpdateTick(tctx, s, db, rec)
				}()
				timer.Reset(time.Hour)
			}
		}
	}()
}
