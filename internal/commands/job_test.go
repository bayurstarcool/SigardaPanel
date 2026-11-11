package commands

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestJobList_WithJobs(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/jobs" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"jobs": []map[string]any{
					{"id": 1, "type": "site_create", "status": "succeeded", "attempts": 1, "created_at": "2024-01-01T00:00:00Z"},
				},
			},
		})
	})

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"job", "list"})
	if code != 0 {
		t.Fatalf("expected 0, got %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "site_create") {
		t.Errorf("expected job type in output: %s", stdout.String())
	}
}

func TestJobList_FilterByStatus(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("status")
		if q != "running" {
			t.Errorf("expected status=running, got %s", q)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"jobs": []map[string]any{}},
		})
	})

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"job", "list", "--status", "running"})
	if code != 0 {
		t.Fatalf("expected 0, got %d, stderr=%s", code, stderr.String())
	}
}

func TestJobWatch_TerminalStatus(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/jobs/10" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"status": "succeeded"},
		})
	})

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"job", "watch", "--id", "10"})
	if code != 0 {
		t.Fatalf("expected 0, got %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "succeeded") {
		t.Errorf("expected status in output: %s", stdout.String())
	}
}

func TestJobWatch_FailedExitsNonZero(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"id": 11, "status": "failed", "error": "agent unreachable"},
		})
	})
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	if code := app.Run([]string{"job", "watch", "11"}); code != 1 {
		t.Fatalf("expected exit 1 for failed job, got %d", code)
	}
	if !strings.Contains(stderr.String(), "agent unreachable") {
		t.Errorf("expected job error on stderr: %s", stderr.String())
	}
}

func TestJobGet_AndCancelConflict(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/v1/jobs/12":
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": 12, "type": "site.create", "status": "failed", "error": "boom"}})
		case r.Method == "POST" && r.URL.Path == "/api/v1/jobs/12/cancel":
			w.WriteHeader(409)
			json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "conflict", "message": "cannot cancel job in status failed"}})
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	if code := app.Run([]string{"job", "get", "12"}); code != 0 {
		t.Fatalf("get exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "boom") {
		t.Errorf("expected error field: %s", stdout.String())
	}
	if code := app.Run([]string{"job", "cancel", "12"}); code != 1 {
		t.Fatalf("cancel conflict: expected 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "cannot cancel job in status failed") {
		t.Errorf("expected API message: %s", stderr.String())
	}
}
