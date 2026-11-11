package commands

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sigardapanel/internal/buildinfo"
	"sigardapanel/internal/cli"
)

func TestAPIKeyCreateListRevoke(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/api/v1/api-keys":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if body["name"] != "ci" || body["expires_in_days"] != float64(30) {
				t.Errorf("unexpected body %v", body)
			}
			w.WriteHeader(201)
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"key":   map[string]any{"id": 4, "name": "ci", "prefix": "abcd1234", "expires_at": "2026-12-11T00:00:00Z"},
				"token": "abcd1234rest",
			}})
		case r.Method == "GET" && r.URL.Path == "/api/v1/api-keys":
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"keys": []map[string]any{{"id": 4, "name": "ci", "prefix": "abcd1234", "created_at": "2026-11-11T00:00:00Z"}}, "count": 1, "limit": 20,
			}})
		case r.Method == "DELETE" && r.URL.Path == "/api/v1/api-keys/4":
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"revoked": true, "id": 4}})
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	if code := app.Run([]string{"apikey", "create", "--name", "ci", "--expires-in-days", "30"}); code != 0 {
		t.Fatalf("create exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "token: abcd1234rest") {
		t.Errorf("create output: %s", stdout.String())
	}
	stdout.Reset()
	if code := app.Run([]string{"api-key", "list"}); code != 0 {
		t.Fatalf("list exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "abcd1234...") || !strings.Contains(stdout.String(), "never") {
		t.Errorf("list output: %s", stdout.String())
	}
	if code := app.Run([]string{"apikey", "revoke", "4"}); code != 0 {
		t.Fatalf("revoke exit %d: %s", code, stderr.String())
	}
}

func TestAPIKeyList_WithAPIKeyTokenGetsHint(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer env-key" {
			t.Errorf("expected env token, got %q", r.Header.Get("Authorization"))
		}
		w.WriteHeader(403)
		json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "forbidden", "message": "api key cannot access this endpoint"}})
	})
	t.Setenv("SIGARDAPANEL_TOKEN", "env-key")
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	if code := app.Run([]string{"apikey", "list"}); code != 1 {
		t.Fatalf("expected 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "api key cannot access this endpoint") || !strings.Contains(stderr.String(), "interactive login") {
		t.Errorf("stderr: %s", stderr.String())
	}
}

func TestUnauthorizedHint(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "unauthorized", "message": "invalid or expired token"}})
	})
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	if code := app.Run([]string{"server", "list"}); code != 1 {
		t.Fatalf("expected 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "invalid or expired token") || !strings.Contains(stderr.String(), "sigardapanel login") {
		t.Errorf("stderr: %s", stderr.String())
	}
}

func TestAuditListAndPurge(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			q := r.URL.Query()
			if r.URL.Path != "/api/v1/audit-logs" || q.Get("action") != "site_suspend" || q.Get("limit") != "10" {
				t.Errorf("unexpected list %s?%s", r.URL.Path, r.URL.RawQuery)
			}
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"entries": []map[string]any{
				{"id": 9, "user_id": 1, "action": "site_suspend", "resource": "site", "resource_id": 5, "details": nil, "created_at": "2026-11-11T10:00:00Z"},
			}}})
		case "DELETE":
			if r.URL.Path != "/api/v1/audit-logs" || r.URL.Query().Get("older_than_days") != "90" {
				t.Errorf("unexpected purge %s?%s", r.URL.Path, r.URL.RawQuery)
			}
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"deleted": 12, "older_than_days": 90}})
		}
	})
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	if code := app.Run([]string{"audit", "list", "--action", "site_suspend", "--limit", "10"}); code != 0 {
		t.Fatalf("list exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "site_suspend") {
		t.Errorf("list output: %s", stdout.String())
	}
	if code := app.Run([]string{"audit", "purge"}); code != 2 {
		t.Fatalf("purge without days: expected 2, got %d", code)
	}
	if code := app.Run([]string{"audit", "purge", "--older-than-days", "90"}); code != 1 {
		t.Fatalf("purge without --yes on closed stdin: expected 1, got %d", code)
	}
	stdout.Reset()
	if code := app.Run([]string{"audit", "purge", "--older-than-days", "90", "--yes"}); code != 0 {
		t.Fatalf("purge exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "deleted 12") {
		t.Errorf("purge output: %s", stdout.String())
	}
}

func TestNotificationCommands(t *testing.T) {
	var calls []string
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/api/v1/notifications":
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"notifications": []map[string]any{
				{"id": 1, "title": "Old", "read": true}, {"id": 2, "title": "New", "read": false},
			}, "total": 2, "unread": 1}})
		case "/api/v1/notifications/unread-count":
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"count": 1}})
		default:
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"ok": true}})
		}
	})
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	if code := app.Run([]string{"notification", "list", "--unread"}); code != 0 {
		t.Fatalf("list exit %d: %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "Old") || !strings.Contains(stdout.String(), "New") {
		t.Errorf("unread filter: %s", stdout.String())
	}
	stdout.Reset()
	if code := app.Run([]string{"notifications", "count"}); code != 0 || strings.TrimSpace(stdout.String()) != "1" {
		t.Fatalf("count: %d %q", code, stdout.String())
	}
	app.Run([]string{"notification", "read", "--all"})
	app.Run([]string{"notification", "read", "2"})
	app.Run([]string{"notification", "delete", "--read"})
	app.Run([]string{"notification", "delete", "2"})
	want := []string{
		"POST /api/v1/notifications/read-all", "POST /api/v1/notifications/2/read",
		"DELETE /api/v1/notifications/read", "DELETE /api/v1/notifications/2",
	}
	got := strings.Join(calls[2:], ",")
	if got != strings.Join(want, ",") {
		t.Errorf("calls: %s", got)
	}
}

func TestChannelAdd_SendsConfigObject(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		cfg, ok := body["config"].(map[string]any)
		if !ok || cfg["url"] != "https://example.com/hook" {
			t.Errorf("config must be a JSON object, got %#v", body["config"])
		}
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": 6}})
	})
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	if code := app.Run([]string{"channels", "add", "--name", "hook", "--type", "webhook", "--config", `{"url":"https://example.com/hook"}`}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	if code := app.Run([]string{"channels", "add", "--name", "hook", "--type", "webhook", "--config", `not json`}); code != 2 {
		t.Fatalf("invalid config: expected 2, got %d", code)
	}
}

func TestWhoamiAndVersion(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/me":
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": 1, "username": "root", "role": "super_admin"}})
		case "/api/v1/version":
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"service": "sigardapanel-api", "version": "9.9.9"}})
		default:
			t.Errorf("unexpected %s", r.URL.Path)
		}
	})
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	if code := app.Run([]string{"whoami", "-o", "json"}); code != 0 {
		t.Fatalf("whoami exit %d: %s", code, stderr.String())
	}
	var me map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &me); err != nil || me["username"] != "root" || me["token_source"] == "" {
		t.Fatalf("whoami json: %s", stdout.String())
	}
	stdout.Reset()
	if code := app.Run([]string{"version"}); code != 0 {
		t.Fatalf("version exit %d", code)
	}
	if !strings.Contains(stdout.String(), "client: "+buildinfo.Version) || !strings.Contains(stdout.String(), "server: 9.9.9") {
		t.Errorf("version output: %s", stdout.String())
	}
	stdout.Reset()
	app.Run([]string{"version", "--client"})
	if strings.TrimSpace(stdout.String()) != buildinfo.Version {
		t.Errorf("version --client: %q", stdout.String())
	}
}

func TestVersion_ServerDown(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	if code := app.Run([]string{"version"}); code != 0 {
		t.Fatalf("version should not fail when the server is down, got %d", code)
	}
	if !strings.Contains(stdout.String(), "server: unavailable") {
		t.Errorf("output: %s", stdout.String())
	}
}

func TestLogin_TwoFactorPrompt(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["totp_code"] == "" {
			w.WriteHeader(401)
			json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "2fa_required", "message": "Two-factor authentication code required"}})
			return
		}
		if body["totp_code"] != "123456" {
			t.Errorf("unexpected code %q", body["totp_code"])
		}
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"token": "tfa-token", "user": map[string]any{"username": "u", "role": "admin"}}})
	}))
	defer srv.Close()
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader("123456\n"))
	code := app.Run([]string{"login", "--api-url", srv.URL, "--user", "u", "--password", "pw123456"})
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	if attempts != 2 {
		t.Errorf("expected 2 login attempts, got %d", attempts)
	}
	if tok, _ := cli.LoadToken(); tok != "tfa-token" {
		t.Errorf("token not saved: %q", tok)
	}
}

func TestLogin_RateLimitedShowsAPIMessage(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "rate_limited", "message": "too many login attempts"}})
	}))
	defer srv.Close()
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	if code := app.Run([]string{"login", "--api-url", srv.URL, "--user", "u", "--password", "x"}); code != 1 {
		t.Fatalf("expected 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "too many login attempts") {
		t.Errorf("stderr: %s", stderr.String())
	}
}

func TestHelpExitCodes(t *testing.T) {
	// If a help guard regressed, never let install/api touch a real DB.
	t.Setenv("SIGARDAPANEL_DB_PATH", t.TempDir()+"/guard.db")
	t.Setenv("SIGARDAPANEL_API_ADDR", "127.0.0.1:1")
	for _, args := range [][]string{
		{"install", "--help"}, {"api", "-h"}, {"agent", "--help"}, {"dev", "help"},
		{"docker", "compose", "--help"}, {"cloudpanel", "scan", "--help"}, {"cloudpanel", "import-site", "-h"},
		{"help"}, {"site", "--help"}, {"apikey", "help"}, {"audit", "purge", "--help"},
		{"job", "watch", "-h"}, {"db", "user", "create", "--help"}, {"login", "--help"},
	} {
		var stdout, stderr bytes.Buffer
		app := New(&stdout, &stderr, strings.NewReader(""))
		if code := app.Run(args); code != 0 {
			t.Errorf("%v: expected 0, got %d (stderr %s)", args, code, stderr.String())
		}
		if stdout.Len() == 0 {
			t.Errorf("%v: help should go to stdout", args)
		}
	}
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	if code := app.Run([]string{"site", "frobnicate"}); code != 2 {
		t.Errorf("unknown subcommand: expected 2, got %d", code)
	}
}
