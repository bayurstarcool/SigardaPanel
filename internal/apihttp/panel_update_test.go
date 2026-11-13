package apihttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"sigardapanel/internal/audit"
	"sigardapanel/internal/auth"
	"sigardapanel/internal/buildinfo"
	"sigardapanel/internal/selfupdate"
	"sigardapanel/internal/store"
)

type fakePanelAgent struct {
	mu         sync.Mutex
	started    []string
	rolledBack []string
	state      string
	startedAt  string
}

func (f *fakePanelAgent) set(state, startedAt string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state, f.startedAt = state, startedAt
}

func (f *fakePanelAgent) startedTags() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.started...)
}

func (f *fakePanelAgent) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer agent-tok" {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message":"invalid agent token"}`))
		return
	}
	switch {
	case r.URL.Path == "/tasks/panel/backups":
		json.NewEncoder(w).Encode(map[string]any{"backups": []any{map[string]any{"name": "20261101T020000Z", "version": "1.6.0", "kind": "update", "has_database": true}}})
		return
	case r.URL.Path == "/tasks/panel/rollback":
		var body struct{ Backup string }
		json.NewDecoder(r.Body).Decode(&body)
		if body.Backup != "20261101T020000Z" {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"ok":false,"message":"backup not found"}`))
			return
		}
		f.rolledBack = append(f.rolledBack, body.Backup)
		f.state = "running"
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "status": map[string]any{"state": "running", "action": "rollback", "tag": "v1.6.0"}})
		return
	}
	switch r.Method {
	case http.MethodGet:
		json.NewEncoder(w).Encode(map[string]any{"state": f.state, "supported": true, "tag": "v1.7.0", "started_at": f.startedAt, "message": "boom"})
	case http.MethodPost:
		var body struct{ Tag string }
		json.NewDecoder(r.Body).Decode(&body)
		f.started = append(f.started, body.Tag)
		f.state = "running"
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "status": map[string]any{"state": "running", "tag": body.Tag}})
	}
}

// setupPanelUpdate points the release checker and the local agent at fakes.
func setupPanelUpdate(t *testing.T, version, latestTag string) *fakePanelAgent {
	t.Helper()
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"tag_name":"` + latestTag + `","name":"SigardaPanel ` + latestTag + `","body":"Fixes","html_url":"https://example.invalid/r"}`))
	}))
	t.Cleanup(gh.Close)
	agent := &fakePanelAgent{state: "idle"}
	ag := httptest.NewServer(http.HandlerFunc(agent.handler))
	t.Cleanup(ag.Close)

	origChecker, origAgent, origVersion := panelReleases, panelUpdateAgent, buildinfo.Version
	origCtx, origInterval := panelWatchCtx, panelUpdateWatchInterval
	watchCtx, stopWatch := context.WithCancel(context.Background())
	panelReleases = &selfupdate.Checker{HTTP: gh.Client(), APIBase: gh.URL, Repo: "o/r"}
	panelUpdateAgent = func() (string, string) { return ag.URL, "agent-tok" }
	buildinfo.Version = version
	panelWatchCtx, panelUpdateWatchInterval = watchCtx, 5*time.Millisecond
	t.Cleanup(func() {
		stopWatch()
		panelWatchWG.Wait() // no watcher may outlive the test (it uses its DB)
		panelReleases, panelUpdateAgent, buildinfo.Version = origChecker, origAgent, origVersion
		panelWatchCtx, panelUpdateWatchInterval = origCtx, origInterval
	})
	return agent
}

func TestPanelUpdateStatusAndStart(t *testing.T) {
	agent := setupPanelUpdate(t, "1.6.2", "v1.7.0")
	e, s, ctx := newTestApp(t)
	tok := roleToken(t, s, ctx, "boss", auth.RoleSuperAdmin)

	rec := doRequest(t, e, http.MethodGet, "/api/v1/system/panel-update", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			Current         string             `json:"current_version"`
			UpdateAvailable bool               `json:"update_available"`
			Latest          selfupdate.Release `json:"latest"`
			Status          map[string]any     `json:"status"`
		} `json:"data"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if !resp.Data.UpdateAvailable || resp.Data.Latest.Tag != "v1.7.0" || resp.Data.Status["state"] != "idle" {
		t.Fatalf("unexpected status: %s", rec.Body.String())
	}

	rec = doRequest(t, e, http.MethodPost, "/api/v1/system/panel-update", map[string]any{}, tok)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("start: %d %s", rec.Code, rec.Body.String())
	}
	if len(agent.startedTags()) != 1 || agent.startedTags()[0] != "v1.7.0" {
		t.Fatalf("agent started %v", agent.startedTags())
	}

	rec = doRequest(t, e, http.MethodPut, "/api/v1/system/panel-update/settings", map[string]any{"auto_update": true}, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("settings: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(doRequest(t, e, http.MethodGet, "/api/v1/system/panel-update", nil, tok).Body.String(), `"auto_update":true`) {
		t.Fatal("auto_update was not saved")
	}
}

func TestPanelUpdateRequiresSuperAdmin(t *testing.T) {
	agent := setupPanelUpdate(t, "1.6.2", "v1.7.0")
	e, s, ctx := newTestApp(t)
	for _, role := range []string{auth.RoleAdmin, auth.RoleUser} {
		tok := roleToken(t, s, ctx, "u-"+role, role)
		for _, r := range []struct{ m, p string }{
			{http.MethodGet, "/api/v1/system/panel-update"},
			{http.MethodPost, "/api/v1/system/panel-update"},
			{http.MethodPut, "/api/v1/system/panel-update/settings"},
		} {
			if rec := doRequest(t, e, r.m, r.p, map[string]any{"auto_update": true}, tok); rec.Code != http.StatusForbidden {
				t.Fatalf("%s %s as %s = %d", r.m, r.p, role, rec.Code)
			}
		}
	}
	if len(agent.startedTags()) != 0 {
		t.Fatalf("agent started %v", agent.startedTags())
	}
}

func TestPanelUpdateRefusesWhenCurrentOrDev(t *testing.T) {
	for _, c := range []struct{ version, latest string }{{"1.7.0", "v1.7.0"}, {"dev", "v1.7.0"}} {
		agent := setupPanelUpdate(t, c.version, c.latest)
		e, s, ctx := newTestApp(t)
		tok := roleToken(t, s, ctx, "boss", auth.RoleSuperAdmin)
		if rec := doRequest(t, e, http.MethodPost, "/api/v1/system/panel-update", map[string]any{}, tok); rec.Code != http.StatusConflict {
			t.Fatalf("version %s: start = %d %s", c.version, rec.Code, rec.Body.String())
		}
		if len(agent.startedTags()) != 0 {
			t.Fatalf("agent started %v", agent.startedTags())
		}
	}
}

func TestPanelUpdateTickAutoInstallsOncePerRelease(t *testing.T) {
	agent := setupPanelUpdate(t, "1.6.2", "v1.7.0")
	_, s, ctx := newTestApp(t)
	roleToken(t, s, ctx, "boss", auth.RoleSuperAdmin)
	d := s.DB()

	// Auto-update off: notify once, install nothing.
	rec := audit.NewRecorder(d)
	panelUpdateTick(ctx, s, d, rec)
	panelUpdateTick(ctx, s, d, rec)
	if len(agent.startedTags()) != 0 {
		t.Fatalf("installed with auto-update off: %v", agent.startedTags())
	}
	if n := countNotifications(t, s, "panel_update"); n != 1 {
		t.Fatalf("expected 1 notification, got %d", n)
	}

	store.SetSetting(ctx, d, settingPanelAutoUpdate, "true")
	panelUpdateTick(ctx, s, d, rec)
	agent.set("failed", "") // the attempt failed; it must not be retried
	panelUpdateTick(ctx, s, d, rec)
	if len(agent.startedTags()) != 1 || agent.startedTags()[0] != "v1.7.0" {
		t.Fatalf("auto-update started %v, want exactly one v1.7.0", agent.startedTags())
	}
}

func countNotifications(t *testing.T, s *store.Store, typ string) int {
	t.Helper()
	var n int
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM notifications WHERE type = ?`, typ).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAutoWindow(t *testing.T) {
	at := func(h int) time.Time { return time.Date(2026, 1, 1, h, 30, 0, 0, time.Local) }
	day := parseAutoWindow("2-5")
	night := parseAutoWindow("22-3")
	for _, c := range []struct {
		w    *autoWindow
		h    int
		want bool
	}{{day, 2, true}, {day, 4, true}, {day, 5, false}, {day, 1, false}, {night, 23, true}, {night, 2, true}, {night, 3, false}, {night, 12, false}, {nil, 12, true}} {
		if got := c.w.contains(at(c.h)); got != c.want {
			t.Errorf("%v.contains(%d) = %v, want %v", c.w, c.h, got, c.want)
		}
	}
	for _, bad := range []string{"", "5-5", "24-1", "a-b", "-1-3"} {
		if parseAutoWindow(bad) != nil {
			t.Errorf("parseAutoWindow(%q) accepted", bad)
		}
	}
}

func TestPanelUpdateTickRespectsWindow(t *testing.T) {
	agent := setupPanelUpdate(t, "1.6.2", "v1.7.0")
	_, s, ctx := newTestApp(t)
	d := s.DB()
	store.SetSetting(ctx, d, settingPanelAutoUpdate, "true")
	store.SetSetting(ctx, d, settingPanelAutoWindow, "2-4")
	orig := panelUpdateNow
	t.Cleanup(func() { panelUpdateNow = orig })
	panelUpdateNow = func() time.Time { return time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local) }
	panelUpdateTick(ctx, s, d, audit.NewRecorder(d))
	if len(agent.startedTags()) != 0 {
		t.Fatalf("installed outside the window: %v", agent.startedTags())
	}
	panelUpdateNow = func() time.Time { return time.Date(2026, 1, 2, 3, 0, 0, 0, time.Local) }
	panelUpdateTick(ctx, s, d, audit.NewRecorder(d))
	if len(agent.startedTags()) != 1 {
		t.Fatalf("did not install inside the window: %v", agent.startedTags())
	}
}

func TestPanelUpdateReportsResultOnce(t *testing.T) {
	agent := setupPanelUpdate(t, "1.7.0", "v1.7.0")
	_, s, ctx := newTestApp(t)
	roleToken(t, s, ctx, "boss", auth.RoleSuperAdmin)
	agent.set("failed", "2026-11-13T05:00:00Z")
	panelUpdateReport(ctx, s, s.DB())
	panelUpdateReport(ctx, s, s.DB())
	if n := countNotifications(t, s, "panel_update"); n != 1 {
		t.Fatalf("expected one failure notification, got %d", n)
	}
	agent.set("succeeded", "2026-11-13T06:00:00Z")
	panelUpdateReport(ctx, s, s.DB())
	if n := countNotifications(t, s, "panel_update"); n != 2 {
		t.Fatalf("expected a success notification, got %d total", n)
	}
}

func TestPanelUpdateSettingsValidation(t *testing.T) {
	setupPanelUpdate(t, "1.6.2", "v1.7.0")
	e, s, ctx := newTestApp(t)
	tok := roleToken(t, s, ctx, "boss", auth.RoleSuperAdmin)
	for _, bad := range []map[string]any{
		{},
		{"auto_update": "yes"},
		{"auto_update_window": map[string]any{"start": 3, "end": 3}},
		{"auto_update_window": map[string]any{"start": 25, "end": 3}},
	} {
		if rec := doRequest(t, e, http.MethodPut, "/api/v1/system/panel-update/settings", bad, tok); rec.Code != http.StatusBadRequest {
			t.Errorf("%v accepted: %d", bad, rec.Code)
		}
	}
	rec := doRequest(t, e, http.MethodPut, "/api/v1/system/panel-update/settings", map[string]any{"auto_update_window": map[string]any{"start": 22, "end": 3}}, tok)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"start":22`) {
		t.Fatalf("window not saved: %d %s", rec.Code, rec.Body.String())
	}
	// Leaving auto_update out keeps it; null clears the window.
	rec = doRequest(t, e, http.MethodPut, "/api/v1/system/panel-update/settings", map[string]any{"auto_update_window": nil}, tok)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"auto_update_window":null`) {
		t.Fatalf("window not cleared: %d %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, e, http.MethodGet, "/api/v1/system/panel-update/summary", nil, tok)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"update_available":true`) {
		t.Fatalf("summary: %d %s", rec.Code, rec.Body.String())
	}
}

func TestPanelUpdateWatcherAnnouncesManualResult(t *testing.T) {
	agent := setupPanelUpdate(t, "1.6.2", "v1.7.0")
	orig := panelUpdateWatchInterval
	panelUpdateWatchInterval = 5 * time.Millisecond
	t.Cleanup(func() { panelUpdateWatchInterval = orig })
	e, s, ctx := newTestApp(t)
	tok := roleToken(t, s, ctx, "boss", auth.RoleSuperAdmin)
	if rec := doRequest(t, e, http.MethodPost, "/api/v1/system/panel-update", map[string]any{}, tok); rec.Code != http.StatusAccepted {
		t.Fatalf("start: %d", rec.Code)
	}
	agent.set("failed", "2026-11-13T07:00:00Z")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if countNotifications(t, s, "panel_update") > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the failed update was not announced")
}

func TestPanelRollbackEndpoints(t *testing.T) {
	agent := setupPanelUpdate(t, "1.7.0", "v1.7.0")
	e, s, ctx := newTestApp(t)
	tok := roleToken(t, s, ctx, "boss", auth.RoleSuperAdmin)
	admin := roleToken(t, s, ctx, "adm", auth.RoleAdmin)

	rec := doRequest(t, e, http.MethodGet, "/api/v1/system/panel-update/backups", nil, tok)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "20261101T020000Z") {
		t.Fatalf("backups: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doRequest(t, e, http.MethodPost, "/api/v1/system/panel-update/rollback", map[string]any{"backup": "20261101T020000Z"}, admin); rec.Code != http.StatusForbidden {
		t.Fatalf("admin rollback = %d", rec.Code)
	}
	if rec := doRequest(t, e, http.MethodPost, "/api/v1/system/panel-update/rollback", map[string]any{"backup": "20990101T000000Z"}, tok); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown backup = %d %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, e, http.MethodPost, "/api/v1/system/panel-update/rollback", map[string]any{"backup": "20261101T020000Z"}, tok)
	if rec.Code != http.StatusAccepted || !strings.Contains(rec.Body.String(), "v1.6.0") {
		t.Fatalf("rollback: %d %s", rec.Code, rec.Body.String())
	}
	agent.mu.Lock()
	n := len(agent.rolledBack)
	agent.mu.Unlock()
	if n != 1 {
		t.Fatalf("agent rollbacks = %d", n)
	}
}
