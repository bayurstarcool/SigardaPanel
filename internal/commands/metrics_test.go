package commands

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func TestMetricsLatest_Success(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/metrics/latest") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"id":           1,
				"server_id":    1,
				"cpu_percent":  50.0,
				"collected_at": "2025-01-01T00:00:00Z",
			},
		})
	})

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"metrics", "latest", "--server", "1"})
	if code != 0 {
		t.Errorf("expected 0, got %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "cpu_percent") {
		t.Errorf("expected cpu_percent in output, got: %s", stdout.String())
	}
}

func TestMetricsLatest_MissingServer(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {})

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"metrics", "latest"})
	if code != 2 {
		t.Errorf("expected 2, got %d", code)
	}
}

func TestMetricsLatest_InvalidServerID(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {})

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"metrics", "latest", "--server", "abc"})
	if code != 2 {
		t.Errorf("expected 2, got %d", code)
	}
}

func TestMetricsLatest_ZeroServerID(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {})

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"metrics", "latest", "--server", "0"})
	if code != 2 {
		t.Errorf("expected 2, got %d", code)
	}
}

func TestMetricsLatest_NotFoundResponse(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"code": "not_found", "message": "no metrics yet"},
		})
	})

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"metrics", "latest", "--server", "99"})
	if code != 1 {
		t.Errorf("expected 1, got %d", code)
	}
}

func TestMetricsList_Success(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/metrics") || strings.Contains(r.URL.Path, "/latest") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if !strings.Contains(r.URL.RawQuery, "range=1h") {
			t.Errorf("expected range=1h, got: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"metrics": []any{},
				"count":   0,
				"range":   "1h",
			},
		})
	})

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"metrics", "list", "--server", "1"})
	if code != 0 {
		t.Errorf("expected 0, got %d", code)
	}
}

func TestMetricsList_CustomRange(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "range=24h") {
			t.Errorf("expected range=24h, got: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"metrics": []any{},
				"count":   0,
				"range":   "24h",
			},
		})
	})

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"metrics", "list", "--server", "1", "--range", "24h"})
	if code != 0 {
		t.Errorf("expected 0, got %d", code)
	}
}

func TestMetricsList_MissingServer(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {})

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"metrics", "list"})
	if code != 2 {
		t.Errorf("expected 2, got %d", code)
	}
}

func TestMetricsList_WithMetricsData(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"metrics": []map[string]any{
					{"id": 1, "server_id": 1, "cpu_percent": 10.0},
					{"id": 2, "server_id": 1, "cpu_percent": 20.0},
				},
				"count": 2,
				"range": "1h",
			},
		})
	})

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"metrics", "list", "--server", "1"})
	if code != 0 {
		t.Errorf("expected 0, got %d", code)
	}
}

func TestMetrics_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"metrics", "help"})
	if code != 0 {
		t.Errorf("expected 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), "latest") {
		t.Errorf("expected help to contain latest: %s", stdout.String())
	}
}

func TestMetrics_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"metrics", "unknown"})
	if code != 2 {
		t.Errorf("expected 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), "unknown metrics subcommand") {
		t.Errorf("expected error about unknown subcommand: %s", stderr.String())
	}
}

func TestMetricsServerIDFromArgs(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(r.URL.Path, "/")
		var serverID string
		for i, p := range parts {
			if p == "servers" && i+1 < len(parts) {
				serverID = parts[i+1]
				break
			}
		}
		if serverID != strconv.FormatInt(42, 10) {
			t.Errorf("expected server ID 42 in path, got: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": 1}})
	})

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"metrics", "latest", "--server", "42"})
	if code != 0 {
		t.Errorf("expected 0, got %d", code)
	}
}