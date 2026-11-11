package commands

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestUserUpdate_ResolvesUsernameAndPatches(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/v1/users":
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"users": []map[string]any{
				{"id": 7, "username": "bayu", "email": "bayu@example.com", "role": "user"},
			}}})
		case r.Method == "PATCH" && r.URL.Path == "/api/v1/users/7":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if body["role"] != "admin" || body["max_sites"] != float64(5) {
				t.Errorf("unexpected body %v", body)
			}
			if _, ok := body["disk_limit"]; ok {
				t.Errorf("disk_limit must not be sent when not given")
			}
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"updated": true, "id": 7}})
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	if code := app.Run([]string{"user", "update", "bayu", "--role", "admin", "--max-sites", "5"}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	if code := app.Run([]string{"user", "update", "bayu"}); code != 2 {
		t.Fatalf("no fields: expected 2, got %d", code)
	}
	if code := app.Run([]string{"user", "update", "bayu", "--role", "root"}); code != 2 {
		t.Fatalf("bad role: expected 2, got %d", code)
	}
}

func TestUserList_YAMLWithQuotas(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"users": []map[string]any{
			{"id": 1, "username": "admin", "role": "super_admin", "max_sites": 3},
		}, "count": 1}})
	})
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	if code := app.Run([]string{"user", "list", "-o", "yaml"}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "max_sites: 3") || !strings.Contains(stdout.String(), "role: super_admin") {
		t.Errorf("yaml: %s", stdout.String())
	}
	stdout.Reset()
	app.Run([]string{"user", "list"})
	if !strings.Contains(stdout.String(), "unlimited") {
		t.Errorf("table should show unlimited disk: %s", stdout.String())
	}
}

func TestUserResetPassword_FlagBeforeTarget(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PATCH" || r.URL.Path != "/api/v1/users/3" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"updated": true}})
	})
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	if code := app.Run([]string{"user", "reset-password", "--password", "n3w-passw0rd", "3"}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
}
