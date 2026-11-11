package commands

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"sigardapanel/internal/cli"
)

// setupMockAPI creates a mock API server and saves it as the configured URL
func setupMockAPI(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	// Per-test config dir: never read or delete the real ~/.config/sigardapanel.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	// Override config to point to mock server
	cfg := &cli.Config{APIURL: server.URL}
	if err := cli.SaveConfig(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	t.Cleanup(func() {
		if p, _ := cli.ConfigPath(); p != "" {
			os.Remove(p)
		}
	})

	// Save test token
	if err := cli.SaveToken("test-token"); err != nil {
		t.Fatalf("save token: %v", err)
	}
	t.Cleanup(func() {
		if p, _ := cli.TokenPath(); p != "" {
			os.Remove(p)
		}
	})

	return server
}

func TestServerAdd_Success(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/servers" || r.Method != "POST" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("missing auth header")
		}
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["name"] != "test" {
			t.Errorf("expected name=test, got %s", body["name"])
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"id": 1, "name": "test", "hostname": "test.example.com",
				"agent_token": "secret-abc123",
			},
			"request_id": "r",
		})
	})

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"server", "add", "--name", "test", "--hostname", "test.example.com"})

	if code != 0 {
		t.Fatalf("expected 0, got %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "agent_token") {
		t.Errorf("expected token in output, got: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "secret-abc123") {
		t.Errorf("expected token value, got: %s", stdout.String())
	}
}

func TestServerAdd_MissingFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"server", "add"})
	if code != 2 {
		t.Errorf("expected 2, got %d", code)
	}
}

func TestServerList_WithServers(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/servers" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"servers": []map[string]any{
					{"id": 1, "name": "alpha", "hostname": "a.example.com", "status": "pending"},
					{"id": 2, "name": "beta", "hostname": "b.example.com", "status": "online"},
				},
			},
			"request_id": "r",
		})
	})

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"server", "list"})

	if code != 0 {
		t.Fatalf("expected 0, got %d, stderr=%s", code, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, "alpha") || !strings.Contains(output, "beta") {
		t.Errorf("missing servers in output: %s", output)
	}
	if !strings.Contains(output, "NAME") {
		t.Errorf("missing table header: %s", output)
	}
}

func TestServerList_JSON(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"servers": []map[string]any{{"id": 1, "name": "test"}},
			},
		})
	})

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"server", "list", "--output", "json"})

	if code != 0 {
		t.Fatalf("expected 0, got %d", code)
	}
	var data []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &data); err != nil {
		t.Errorf("output not valid JSON: %v, output=%s", err, stdout.String())
	}
}

func TestServerList_Empty(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"servers": []any{}},
		})
	})

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"server", "list"})
	if code != 0 {
		t.Errorf("expected 0, got %d", code)
	}
}

func TestServerRemove_Success(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/servers/1" || r.Method != "DELETE" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"deleted": true, "id": 1},
		})
	})

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"server", "remove", "1"})
	if code != 0 {
		t.Fatalf("expected 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), "deleted") {
		t.Errorf("expected confirmation: %s", stdout.String())
	}
}

func TestServerRemove_NotFound(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"code": "not_found", "message": "missing"},
		})
	})

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"server", "remove", "999"})
	if code != 1 {
		t.Errorf("expected 1, got %d", code)
	}
}

func TestServerDoctor_Healthy(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/health" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"status": "ok"},
		})
	})

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"server", "doctor"})
	if code != 0 {
		t.Errorf("expected 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), "ok") {
		t.Errorf("expected ok in output: %s", stdout.String())
	}
}
func TestServerGetHidesTokenAndHealth(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/servers/1":
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": 1, "name": "local", "agent_token": "SECRET", "status": "online"}})
		case "/api/v1/servers/health":
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"servers": []map[string]any{
				{"id": 1, "name": "local", "health_status": "healthy", "health_latency_ms": 3, "last_health_check": map[string]any{"Time": "2026-11-11T10:00:00Z", "Valid": true}},
			}}})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	if code := app.Run([]string{"server", "get", "1", "-o", "json"}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "SECRET") {
		t.Errorf("agent token leaked: %s", stdout.String())
	}
	stdout.Reset()
	if code := app.Run([]string{"server", "health"}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "healthy") || !strings.Contains(stdout.String(), "2026-11-11T10:00:00Z") {
		t.Errorf("health output: %s", stdout.String())
	}
}

func TestServerUpdate_SendsOnlySupportedFields(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if r.Method != "PATCH" || body["ssh_port"] != float64(2222) || body["name"] != "web1" {
			t.Errorf("unexpected %s body %v", r.Method, body)
		}
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": 2, "name": "web1"}})
	})
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	if code := app.Run([]string{"server", "update", "2", "--name", "web1", "--ssh-port", "2222"}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	if code := app.Run([]string{"server", "update", "2", "--status", "active"}); code != 2 {
		t.Fatalf("--status is not supported by the API; expected 2, got %d", code)
	}
}
