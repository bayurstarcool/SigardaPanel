package commands

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestDBCreate_Success(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/sites/5/databases" || r.Method != "POST" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if body["name"] != "appdb" {
			t.Errorf("expected name=appdb, got %s", body["name"])
		}
		if body["type"] != "mysql" {
			t.Errorf("expected type=mysql, got %s", body["type"])
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(202)
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"database_id": 11,
				"job_id":      42,
			},
		})
	})

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"db", "create", "--site", "5", "--name", "appdb", "--type", "mysql"})

	if code != 0 {
		t.Fatalf("expected 0, got %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "id=11") {
		t.Errorf("expected database id in output, got: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "job_id=42") {
		t.Errorf("expected job_id in output, got: %s", stdout.String())
	}
}

func TestDBCreate_MissingSite(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"db", "create", "--name", "appdb"})

	if code != 2 {
		t.Fatalf("expected 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), "--site") {
		t.Errorf("expected --site error hint, got: %s", stderr.String())
	}
}

func TestDBList_Success(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/sites/7/databases" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"databases": []map[string]any{
					{"id": 1, "name": "alpha", "type": "mysql", "charset": "utf8mb4", "size_bytes": 0},
					{"id": 2, "name": "beta", "type": "postgres", "charset": "utf8", "size_bytes": 1024},
				},
				"count": 2,
			},
		})
	})

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"db", "list", "--site", "7"})

	if code != 0 {
		t.Fatalf("expected 0, got %d, stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "alpha") {
		t.Errorf("expected alpha in output, got: %s", out)
	}
	if !strings.Contains(out, "beta") {
		t.Errorf("expected beta in output, got: %s", out)
	}
	if !strings.Contains(out, "NAME") {
		t.Errorf("expected table header NAME, got: %s", out)
	}
}

func TestDBList_JSON(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"databases": []map[string]any{
					{"id": 1, "name": "alpha"},
				},
			},
		})
	})

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"db", "list", "--site", "1", "--output", "json"})

	if code != 0 {
		t.Fatalf("expected 0, got %d, stderr=%s", code, stderr.String())
	}
	var data []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &data); err != nil {
		t.Fatalf("output not valid JSON: %v, output=%s", err, stdout.String())
	}
	if len(data) != 1 || data[0]["name"] != "alpha" {
		t.Fatalf("unexpected JSON output: %+v", data)
	}
}

func TestDBDelete_Success(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/databases/9" || r.Method != "DELETE" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(202)
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"database_id": 9, "job_id": 100},
		})
	})

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader("y\n"))
	code := app.Run([]string{"db", "delete", "9"})

	if code != 0 {
		t.Fatalf("expected 0, got %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "9") {
		t.Errorf("expected database id in output, got: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "deleted") {
		t.Errorf("expected 'deleted' in output, got: %s", stdout.String())
	}
}

func TestDBDelete_MissingArg(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"db", "delete"})

	if code != 2 {
		t.Fatalf("expected 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), "usage") {
		t.Errorf("expected usage hint, got: %s", stderr.String())
	}
}
