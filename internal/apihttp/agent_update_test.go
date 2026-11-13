package apihttp

import (
	"database/sql"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"sigardapanel/internal/audit"
	"sigardapanel/internal/auth"
	"sigardapanel/internal/store"
)

// fakeRemoteAgent is a remote server's agent with the agent updater.
type fakeRemoteAgent struct {
	mu        sync.Mutex
	version   string
	state     string
	startedAt string
	starts    []map[string]any
	old       bool // pre-update agent: no /tasks/agent/update
}

func (f *fakeRemoteAgent) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.old || r.URL.Path != "/tasks/agent/update" {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodPost {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		f.starts = append(f.starts, body)
		f.state = "running"
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{"ok":true}`))
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"kind": "agent", "supported": true, "agent_version": f.version,
		"state": f.state, "tag": "v1.7.0", "started_at": f.startedAt, "message": "agent boom"})
}

func (f *fakeRemoteAgent) set(state, startedAt string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state, f.startedAt = state, startedAt
}

func (f *fakeRemoteAgent) startCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.starts)
}

// remoteIP returns a non-loopback address of this machine, so a test
// agent is not taken for the panel host by isLocalServer.
func remoteIP(t *testing.T) string {
	t.Helper()
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok && !ipn.IP.IsLoopback() && ipn.IP.To4() != nil {
			return ipn.IP.String()
		}
	}
	t.Skip("no non-loopback IPv4 address to run a remote test agent on")
	return ""
}

func addAgentServer(t *testing.T, s *store.Store, name, url, version string) int64 {
	t.Helper()
	srv := &store.Server{Name: name, Hostname: name + ".example.com", Status: "online", AgentToken: "agent-tok",
		AgentURL: sql.NullString{String: url, Valid: true}}
	if err := s.Servers.Create(t.Context(), srv); err != nil {
		t.Fatal(err)
	}
	if version != "" {
		s.Servers.UpdateAgentVersion(t.Context(), srv.ID, version)
	}
	return srv.ID
}

func newRemoteAgent(t *testing.T, version string, old bool) (*fakeRemoteAgent, string) {
	t.Helper()
	f := &fakeRemoteAgent{version: version, state: "idle", old: old}
	ln, err := net.Listen("tcp", net.JoinHostPort(remoteIP(t), "0"))
	if err != nil {
		t.Skipf("cannot listen on a non-loopback address: %v", err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(f.handler))
	srv.Listener.Close()
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
	return f, srv.URL
}

func TestAgentUpdatesListAndStart(t *testing.T) {
	setupPanelUpdate(t, "1.7.0", "v1.7.0")
	e, s, ctx := newTestApp(t)
	tok := roleToken(t, s, ctx, "boss", auth.RoleSuperAdmin)
	localID := addAgentServer(t, s, "panel", "http://127.0.0.1:7710", "1.7.0")
	fresh, freshURL := newRemoteAgent(t, "1.6.0", false)
	freshID := addAgentServer(t, s, "web1", freshURL, "1.6.0")
	_, currentURL := newRemoteAgent(t, "1.7.0", false)
	addAgentServer(t, s, "web2", currentURL, "1.7.0")
	_, oldURL := newRemoteAgent(t, "1.4.0", true)
	oldID := addAgentServer(t, s, "legacy", oldURL, "1.4.0")

	rec := doRequest(t, e, http.MethodGet, "/api/v1/system/agent-updates", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			Target  string           `json:"target_version"`
			Servers []agentUpdateRow `json:"servers"`
		} `json:"data"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	byName := map[string]agentUpdateRow{}
	for _, r := range resp.Data.Servers {
		byName[r.Name] = r
	}
	if resp.Data.Target != "v1.7.0" || !byName["panel"].IsLocal || byName["panel"].UpdateAvailable ||
		!byName["web1"].UpdateAvailable || byName["web2"].UpdateAvailable || !strings.Contains(byName["legacy"].Error, "too old") {
		t.Fatalf("unexpected rows: %s", rec.Body.String())
	}

	if rec := doRequest(t, e, http.MethodPost, "/api/v1/servers/"+strconv.FormatInt(localID, 10)+"/agent-update", map[string]any{}, tok); rec.Code != http.StatusConflict {
		t.Fatalf("local agent update = %d", rec.Code)
	}
	if rec := doRequest(t, e, http.MethodPost, "/api/v1/servers/"+strconv.FormatInt(oldID, 10)+"/agent-update", map[string]any{}, tok); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "install command") {
		t.Fatalf("old agent update = %d %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, e, http.MethodPost, "/api/v1/servers/"+strconv.FormatInt(freshID, 10)+"/agent-update", map[string]any{}, tok)
	if rec.Code != http.StatusAccepted || fresh.startCount() != 1 || fresh.starts[0]["tag"] != "v1.7.0" {
		t.Fatalf("agent update = %d %s (%v)", rec.Code, rec.Body.String(), fresh.starts)
	}
	admin := roleToken(t, s, ctx, "adm", auth.RoleAdmin)
	if rec := doRequest(t, e, http.MethodPost, "/api/v1/system/agent-updates", map[string]any{"all": true}, admin); rec.Code != http.StatusForbidden {
		t.Fatalf("admin bulk update = %d", rec.Code)
	}
}

func TestAgentUpdatesBulkSkipsCurrentAgents(t *testing.T) {
	setupPanelUpdate(t, "1.7.0", "v1.7.0")
	e, s, ctx := newTestApp(t)
	tok := roleToken(t, s, ctx, "boss", auth.RoleSuperAdmin)
	a, aURL := newRemoteAgent(t, "1.6.0", false)
	addAgentServer(t, s, "a", aURL, "1.6.0")
	b, bURL := newRemoteAgent(t, "1.7.0", false)
	addAgentServer(t, s, "b", bURL, "1.7.0")
	rec := doRequest(t, e, http.MethodPost, "/api/v1/system/agent-updates", map[string]any{"all": true}, tok)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"started":1`) {
		t.Fatalf("bulk: %d %s", rec.Code, rec.Body.String())
	}
	if a.startCount() != 1 || b.startCount() != 0 {
		t.Fatalf("starts: a=%d b=%d", a.startCount(), b.startCount())
	}
}

func TestAgentUpdateTickAutoAndReport(t *testing.T) {
	setupPanelUpdate(t, "1.7.0", "v1.7.0")
	_, s, ctx := newTestApp(t)
	roleToken(t, s, ctx, "boss", auth.RoleSuperAdmin)
	d := s.DB()
	a, aURL := newRemoteAgent(t, "1.6.0", false)
	addAgentServer(t, s, "a", aURL, "1.6.0")
	rec := audit.NewRecorder(d)

	agentUpdateTick(ctx, s, d, rec)
	if a.startCount() != 0 {
		t.Fatal("updated an agent with automatic agent updates off")
	}
	store.SetSetting(ctx, d, settingAgentAutoUpdate, "true")
	agentUpdateTick(ctx, s, d, rec)
	a.set("failed", "2026-11-13T08:00:00Z")
	agentUpdateTick(ctx, s, d, rec)
	agentUpdateTick(ctx, s, d, rec)
	if a.startCount() != 1 || a.starts[0]["auto"] != true {
		t.Fatalf("auto starts = %v, want exactly one", a.starts)
	}
	if n := countNotifications(t, s, "panel_update"); n != 1 {
		t.Fatalf("expected one failure notification, got %d", n)
	}
}
