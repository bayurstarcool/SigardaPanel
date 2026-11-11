package commands

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestLogsTail_Success(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/sites/1/logs" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"message": "ok",
				"logs":    []map[string]any{{"message": "line2"}, {"message": "line1"}},
				"exists":  true,
			},
		})
	})

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"logs", "tail", "--site", "1"})

	if code != 0 {
		t.Fatalf("expected 0, got %d, stderr=%s", code, stderr.String())
	}
	if stdout.String() != "line1\nline2\n" {
		t.Errorf("expected oldest-first lines, got: %q", stdout.String())
	}
}

func TestLogsTail_SendsLimitParam(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("limit") != "25" || r.URL.Query().Get("type") != "error" {
			t.Errorf("unexpected query: %s", r.URL.RawQuery)
		}
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"logs": []any{}, "exists": true}})
	})
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	if code := app.Run([]string{"logs", "tail", "--site", "1", "--type", "error", "--lines", "25"}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
}

func TestLogsTail_MissingSite(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"logs", "tail"})
	if code != 2 {
		t.Fatalf("expected 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), "--site is required") {
		t.Errorf("expected --site required error, got: %s", stderr.String())
	}
}

func TestLogsTail_FollowOnce(t *testing.T) {
	callCount := 0
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"message": "ok",
				"logs":    []map[string]any{{"message": "line1"}},
				"exists":  true,
			},
		})
	})

	origInterval := followInterval
	origTimeout := followTimeout
	followInterval = 50 * time.Millisecond
	followTimeout = 150 * time.Millisecond
	defer func() {
		followInterval = origInterval
		followTimeout = origTimeout
	}()

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"logs", "tail", "--site", "1", "--follow"})

	if code != 0 {
		t.Fatalf("expected 0, got %d, stderr=%s", code, stderr.String())
	}
	if callCount < 1 {
		t.Fatalf("expected at least 1 fetch call, got %d", callCount)
	}
	if !strings.Contains(stderr.String(), "following") {
		t.Errorf("expected following message, got: %s", stderr.String())
	}
}

func TestLogsTail_NotFound(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"code": "not_found", "message": "missing"},
		})
	})

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"logs", "tail", "--site", "999"})

	if code != 1 {
		t.Fatalf("expected 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "404") {
		t.Errorf("expected 404 error, got: %s", stderr.String())
	}
}
