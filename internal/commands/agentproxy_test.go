package commands

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestFirewallReset_RequiresConfirmation(t *testing.T) {
	calls := 0
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/api/v1/servers/1/firewall/reset" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"ok": true}})
	})
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	if code := app.Run([]string{"firewall", "reset", "--server", "1"}); code != 1 || calls != 0 {
		t.Fatalf("without --yes: expected exit 1 and no request, got %d / %d calls", code, calls)
	}
	if code := app.Run([]string{"firewall", "reset", "--server", "1", "--yes"}); code != 0 || calls != 1 {
		t.Fatalf("with --yes: expected exit 0 and one request, got %d / %d calls (%s)", code, calls, stderr.String())
	}
}

func TestRedisStats_YAML(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"stats": map[string]any{"connected_clients": "3"}}})
	})
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	if code := app.Run([]string{"redis", "stats", "--server", "1", "-o", "yaml"}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "connected_clients: \"3\"") {
		t.Errorf("yaml: %s", stdout.String())
	}
}

func TestDockerLogs_EscapesContainer(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("container_id") != "web app&x" {
			t.Errorf("container_id not escaped: %s", r.URL.RawQuery)
		}
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"logs": "hello"}})
	})
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	if code := app.Run([]string{"docker", "logs", "--server", "1", "--id", "web app&x"}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
}
