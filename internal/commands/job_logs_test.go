package commands

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestJobLogs_Success(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/jobs/7/logs" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"logs": []map[string]any{
					{"line_number": 1, "message": "line one", "stream": "stdout"},
					{"line_number": 2, "message": "line two", "stream": "stderr"},
				},
				"count": 2,
				"since": 0,
			},
		})
	})

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"job", "logs", "--id", "7"})
	if code != 0 {
		t.Fatalf("expected 0, got %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "line one") {
		t.Errorf("expected line one in output: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "[stderr] line two") {
		t.Errorf("expected [stderr] line two in output: %s", stdout.String())
	}
}

func TestJobLogs_MissingID(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {})

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"job", "logs"})
	if code != 2 {
		t.Fatalf("expected 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), "--id is required") {
		t.Errorf("expected --id required: %s", stderr.String())
	}
}
