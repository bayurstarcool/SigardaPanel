package apihttp

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"sigardapanel/internal/auth"
	"sigardapanel/internal/db"
	"sigardapanel/internal/migrations"
	"sigardapanel/internal/store"
)

type mockAgent struct {
	mu       sync.Mutex
	lastPath string
}

func (m *mockAgent) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	m.lastPath = r.URL.Path
	m.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (m *mockAgent) getLastPath() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastPath
}

func setupDockerTest(t *testing.T, agent *mockAgent) (*httptest.Server, string) {
	t.Helper()

	agentSrv := httptest.NewServer(agent)
	t.Cleanup(agentSrv.Close)

	f, _ := os.CreateTemp("", "docker_test_*.db")
	t.Cleanup(func() { os.Remove(f.Name()) })
	d, err := db.Open(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	migrations.Up(d, migrations.AllMigrations)
	s := store.New(d)
	ctx := context.Background()

	user := &store.User{
		Username:     "admin",
		Email:        "admin@test.com",
		PasswordHash: "hash",
		Role:         auth.RoleAdmin,
	}
	if err := s.Users.Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	token, _ := auth.NewToken()
	s.Sessions.Create(ctx, &store.Session{
		UserID:    user.ID,
		TokenHash: auth.HashToken(token),
		ExpiresAt: time.Now().Add(time.Hour),
	})

	srv := &store.Server{
		Name:       "test",
		Hostname:   "test.local",
		IPAddress:  sql.NullString{String: "127.0.0.1", Valid: true},
		AgentURL:   sql.NullString{String: agentSrv.URL, Valid: true},
		AgentToken: "agent-token",
		Status:     "active",
	}
	if err := s.Servers.Create(ctx, srv); err != nil {
		t.Fatal(err)
	}

	router := NewRouter(s, d, nil)
	ts := httptest.NewServer(router)
	t.Cleanup(ts.Close)

	return ts, token
}

func doPost(t *testing.T, ts *httptest.Server, token, path string, body any) *http.Response {
	t.Helper()
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", ts.URL+path, bytes.NewReader(data))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestDockerImageRemove_Path(t *testing.T) {
	agent := &mockAgent{}
	ts, token := setupDockerTest(t, agent)
	resp := doPost(t, ts, token, "/api/v1/servers/1/docker/image/remove",
		map[string]any{"image": "nginx:alpine", "force": true})
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	if got := agent.getLastPath(); got != "/tasks/docker/image/remove" {
		t.Errorf("got %q, want /tasks/docker/image/remove", got)
	}
}

func TestDockerImagePull_Path(t *testing.T) {
	agent := &mockAgent{}
	ts, token := setupDockerTest(t, agent)
	resp := doPost(t, ts, token, "/api/v1/servers/1/docker/image/pull",
		map[string]any{"image": "redis:latest"})
	defer resp.Body.Close()
	if got := agent.getLastPath(); got != "/tasks/docker/image/pull" {
		t.Errorf("got %q, want /tasks/docker/image/pull", got)
	}
}

func TestDockerVolumeCreate_Path(t *testing.T) {
	agent := &mockAgent{}
	ts, token := setupDockerTest(t, agent)
	resp := doPost(t, ts, token, "/api/v1/servers/1/docker/volume/create",
		map[string]any{"name": "myvol"})
	defer resp.Body.Close()
	if got := agent.getLastPath(); got != "/tasks/docker/volume/create" {
		t.Errorf("got %q, want /tasks/docker/volume/create", got)
	}
}

func TestDockerVolumeRemove_Path(t *testing.T) {
	agent := &mockAgent{}
	ts, token := setupDockerTest(t, agent)
	resp := doPost(t, ts, token, "/api/v1/servers/1/docker/volume/remove",
		map[string]any{"name": "myvol"})
	defer resp.Body.Close()
	if got := agent.getLastPath(); got != "/tasks/docker/volume/remove" {
		t.Errorf("got %q, want /tasks/docker/volume/remove", got)
	}
}

func TestDockerNetworkCreate_Path(t *testing.T) {
	agent := &mockAgent{}
	ts, token := setupDockerTest(t, agent)
	resp := doPost(t, ts, token, "/api/v1/servers/1/docker/network/create",
		map[string]any{"name": "mynet"})
	defer resp.Body.Close()
	if got := agent.getLastPath(); got != "/tasks/docker/network/create" {
		t.Errorf("got %q, want /tasks/docker/network/create", got)
	}
}

func TestDockerNetworkRemove_Path(t *testing.T) {
	agent := &mockAgent{}
	ts, token := setupDockerTest(t, agent)
	resp := doPost(t, ts, token, "/api/v1/servers/1/docker/network/remove",
		map[string]any{"name": "mynet"})
	defer resp.Body.Close()
	if got := agent.getLastPath(); got != "/tasks/docker/network/remove" {
		t.Errorf("got %q, want /tasks/docker/network/remove", got)
	}
}

func TestDockerContainerStart_Path(t *testing.T) {
	agent := &mockAgent{}
	ts, token := setupDockerTest(t, agent)
	resp := doPost(t, ts, token, "/api/v1/servers/1/docker/container/start",
		map[string]any{"container_id": "abc123"})
	defer resp.Body.Close()
	if got := agent.getLastPath(); got != "/tasks/docker/container/start" {
		t.Errorf("got %q, want /tasks/docker/container/start", got)
	}
}

func TestDockerContainerRemove_Path(t *testing.T) {
	agent := &mockAgent{}
	ts, token := setupDockerTest(t, agent)
	resp := doPost(t, ts, token, "/api/v1/servers/1/docker/container/remove",
		map[string]any{"container_id": "abc123", "force": true})
	defer resp.Body.Close()
	if got := agent.getLastPath(); got != "/tasks/docker/container/remove" {
		t.Errorf("got %q, want /tasks/docker/container/remove", got)
	}
}

func TestDockerComposeUp_Path(t *testing.T) {
	agent := &mockAgent{}
	ts, token := setupDockerTest(t, agent)
	resp := doPost(t, ts, token, "/api/v1/servers/1/docker/compose/up",
		map[string]any{"site_id": 1})
	defer resp.Body.Close()
	if got := agent.getLastPath(); got != "/tasks/docker/compose/up" {
		t.Errorf("got %q, want /tasks/docker/compose/up", got)
	}
}
