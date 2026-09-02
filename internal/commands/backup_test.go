package commands

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestBackupCreate_Success(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/sites/5/backups" || r.Method != "POST" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(202)
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"backup_id": 7, "job_id": 21, "status": "pending"},
		})
	})

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"backup", "create", "--site", "5"})

	if code != 0 {
		t.Fatalf("expected 0, got %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "backup_id=7") {
		t.Errorf("expected backup_id in output: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "job_id=21") {
		t.Errorf("expected job_id in output: %s", stdout.String())
	}
}

func TestBackupCreate_MissingSite(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"backup", "create"})
	if code != 2 {
		t.Errorf("expected 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), "--site is required") {
		t.Errorf("expected missing site error: %s", stderr.String())
	}
}

func TestBackupList_Success(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/backups" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"backups": []map[string]any{
					{"id": 1, "site_id": 2, "status": "completed", "created_at": "2024-01-01T00:00:00Z"},
					{"id": 2, "site_id": 2, "status": "pending", "created_at": "2024-01-02T00:00:00Z"},
				},
			},
		})
	})

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"backup", "list"})

	if code != 0 {
		t.Fatalf("expected 0, got %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "completed") {
		t.Errorf("expected completed in output: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "pending") {
		t.Errorf("expected pending in output: %s", stdout.String())
	}
}

func TestBackupList_FilterBySite(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/backups" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		q := r.URL.Query().Get("site_id")
		if q != "3" {
			t.Errorf("expected site_id=3, got %s", q)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"backups": []map[string]any{}},
		})
	})

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"backup", "list", "--site", "3"})
	if code != 0 {
		t.Fatalf("expected 0, got %d, stderr=%s", code, stderr.String())
	}
}

func TestBackupRestore_Success(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/backups/8/restore" || r.Method != "POST" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(202)
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"backup_id": 8, "job_id": 22},
		})
	})

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"backup", "restore", "8"})

	if code != 0 {
		t.Fatalf("expected 0, got %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "backup_id=8") {
		t.Errorf("expected backup_id in output: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "job_id=22") {
		t.Errorf("expected job_id in output: %s", stdout.String())
	}
}

func TestBackupRestore_MissingID(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"backup", "restore"})
	if code != 2 {
		t.Errorf("expected 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), "usage") {
		t.Errorf("expected usage hint: %s", stderr.String())
	}
}

func TestBackupHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"backup", "help"})
	if code != 0 {
		t.Errorf("expected 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), "create") {
		t.Errorf("expected create in help: %s", stdout.String())
	}
}

func TestBackupUnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"backup", "unknown"})
	if code != 2 {
		t.Errorf("expected 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), "unknown backup subcommand") {
		t.Errorf("expected unknown subcommand error: %s", stderr.String())
	}
}
