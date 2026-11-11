package commands

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoginExplainsPasswordChangeRequired(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("SIGARDAPANEL_TOKEN", "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
			"token": strings.Repeat("a", 64),
			"user":  map[string]any{"username": "admin", "role": "super_admin", "password_change_required": true},
		}})
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	code := New(&stdout, &stderr, strings.NewReader("")).Login([]string{"--api-url", server.URL, "--user", "admin", "--password", "generated-pw"})
	if code != 0 {
		t.Fatalf("login: exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "password change required") || !strings.Contains(stderr.String(), "sigardapanel passwd") {
		t.Fatalf("login should explain the required password change, stderr=%q", stderr.String())
	}
}

func TestPasswdCallsProfileEndpoint(t *testing.T) {
	t.Setenv("SIGARDAPANEL_TOKEN", "")
	var got map[string]string
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v1/auth/profile" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("missing session token")
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"updated": true}})
	})

	for _, args := range [][]string{{"passwd"}, {"user", "passwd"}} {
		got = nil
		var stdout, stderr bytes.Buffer
		full := append(args, "--current", "old-password", "--new", "brand-new-pw")
		if code := New(&stdout, &stderr, strings.NewReader("")).Run(full); code != 0 {
			t.Fatalf("%v: exit %d: %s", args, code, stderr.String())
		}
		if got["old_password"] != "old-password" || got["new_password"] != "brand-new-pw" {
			t.Fatalf("%v: body %v", args, got)
		}
		if !strings.Contains(stdout.String(), "password changed") {
			t.Fatalf("%v: stdout %q", args, stdout.String())
		}
	}
}

func TestPasswdPromptsAndValidates(t *testing.T) {
	t.Setenv("SIGARDAPANEL_TOKEN", "")
	called := false
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"updated": true}})
	})
	var stdout, stderr bytes.Buffer
	// Mismatched confirmation: nothing is sent.
	code := New(&stdout, &stderr, strings.NewReader("old-password\nnew-password-1\nnew-password-2\n")).Passwd(nil)
	if code == 0 || called {
		t.Fatalf("mismatch must fail without calling the API (code %d, called %v)", code, called)
	}
	// Too short.
	code = New(&stdout, &stderr, strings.NewReader("")).Passwd([]string{"--current", "old-password", "--new", "short"})
	if code != exitUsage || called {
		t.Fatalf("short password: code %d, called %v", code, called)
	}
	// Prompted happy path.
	code = New(&stdout, &stderr, strings.NewReader("old-password\nnew-password-1\nnew-password-1\n")).Passwd(nil)
	if code != 0 || !called {
		t.Fatalf("prompted change: code %d, called %v: %s", code, called, stderr.String())
	}
}

func TestPasswordChangeRequiredErrorHint(t *testing.T) {
	t.Setenv("SIGARDAPANEL_TOKEN", "")
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{
			"code": "password_change_required", "message": "you must change your password before using the panel",
		}})
	})
	var stdout, stderr bytes.Buffer
	if code := New(&stdout, &stderr, strings.NewReader("")).Run([]string{"server", "list"}); code == 0 {
		t.Fatal("expected failure")
	}
	if !strings.Contains(stderr.String(), "sigardapanel passwd") {
		t.Fatalf("expected a passwd hint, got %q", stderr.String())
	}
}
