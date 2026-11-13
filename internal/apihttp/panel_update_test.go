package apihttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"sigardapanel/internal/audit"
	"sigardapanel/internal/auth"
	"sigardapanel/internal/buildinfo"
	"sigardapanel/internal/selfupdate"
	"sigardapanel/internal/store"
)

type fakePanelAgent struct {
	mu      sync.Mutex
	started []string
	state   string
}

func (f *fakePanelAgent) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer agent-tok" {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message":"invalid agent token"}`))
		return
	}
	switch r.Method {
	case http.MethodGet:
		json.NewEncoder(w).Encode(map[string]any{"state": f.state, "supported": true})
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
	panelReleases = &selfupdate.Checker{HTTP: gh.Client(), APIBase: gh.URL, Repo: "o/r"}
	panelUpdateAgent = func() (string, string) { return ag.URL, "agent-tok" }
	buildinfo.Version = version
	t.Cleanup(func() { panelReleases, panelUpdateAgent, buildinfo.Version = origChecker, origAgent, origVersion })
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
	if len(agent.started) != 1 || agent.started[0] != "v1.7.0" {
		t.Fatalf("agent started %v", agent.started)
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
	if len(agent.started) != 0 {
		t.Fatalf("agent started %v", agent.started)
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
		if len(agent.started) != 0 {
			t.Fatalf("agent started %v", agent.started)
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
	if len(agent.started) != 0 {
		t.Fatalf("installed with auto-update off: %v", agent.started)
	}
	if n := countNotifications(t, s, "panel_update"); n != 1 {
		t.Fatalf("expected 1 notification, got %d", n)
	}

	store.SetSetting(ctx, d, settingPanelAutoUpdate, "true")
	panelUpdateTick(ctx, s, d, rec)
	agent.state = "failed" // the attempt failed; it must not be retried
	panelUpdateTick(ctx, s, d, rec)
	if len(agent.started) != 1 || agent.started[0] != "v1.7.0" {
		t.Fatalf("auto-update started %v, want exactly one v1.7.0", agent.started)
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
