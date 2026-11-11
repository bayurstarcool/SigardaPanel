package commands

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSSLStatus_Success(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/sites/2/ssl" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"active": true, "issuer": "Let's Encrypt"},
		})
	})

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"ssl", "status", "--site", "2"})
	if code != 0 {
		t.Fatalf("expected 0, got %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Let's Encrypt") {
		t.Errorf("expected issuer in output: %s", stdout.String())
	}
}

func TestSSLStatus_MissingSite(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"ssl", "status"})
	if code != 2 {
		t.Errorf("expected 2, got %d", code)
	}
}

func TestSSLIssue_Success(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/sites/3/ssl/issue" || r.Method != "POST" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["email"] != "admin@test.com" {
			t.Errorf("expected email=admin@test.com, got %s", body["email"])
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(202)
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"job_id": 55}})
	})

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"ssl", "issue", "--site", "3", "--email", "admin@test.com"})
	if code != 0 {
		t.Fatalf("expected 0, got %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "job_id=55") {
		t.Errorf("expected job_id in output: %s", stdout.String())
	}
}

func TestSSLIssue_EmailOptional(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/sites/3/ssl/issue" || r.Method != "POST" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(202)
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"job_id": 56, "status": "queued", "email": "owner@test.com"}})
	})
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"ssl", "issue", "3"})
	if code != 0 {
		t.Fatalf("expected 0, got %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "email=owner@test.com") {
		t.Errorf("expected resolved email in output: %s", stdout.String())
	}
}

func TestSSLIssue_MissingSite(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	if code := app.Run([]string{"ssl", "issue"}); code != 2 {
		t.Errorf("expected 2, got %d", code)
	}
}

func TestSSLRenewAll_UsesServerEndpoint(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/ssl/renew-all" || r.Method != "POST" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"queued": 2, "skipped": 1, "total": 3}})
	})
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	if code := app.Run([]string{"ssl", "renew-all"}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "2 renewal job(s) queued") {
		t.Errorf("output: %s", stdout.String())
	}
}

func TestSSLRenewAll_DryRun(t *testing.T) {
	soon := time.Now().Add(10 * 24 * time.Hour).UTC().Format(time.RFC3339Nano)
	later := time.Now().Add(80 * 24 * time.Hour).UTC().Format(time.RFC3339)
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/v1/sites" {
			t.Errorf("dry run must not POST: %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"sites": []map[string]any{
			{"id": 1, "domain": "soon.test", "status": "active", "ssl_status": "active", "ssl_expires_at": soon},
			{"id": 2, "domain": "later.test", "status": "active", "ssl_status": "active", "ssl_expires_at": later},
		}}})
	})
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	if code := app.Run([]string{"ssl", "renew-all", "--dry-run"}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "soon.test") || strings.Contains(stdout.String(), "later.test") {
		t.Errorf("output: %s", stdout.String())
	}
}

func TestSSLRenew_Success(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/sites/4/ssl/renew" || r.Method != "POST" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(202)
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"job_id": 66}})
	})

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"ssl", "renew", "--site", "4"})
	if code != 0 {
		t.Fatalf("expected 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), "job_id=66") {
		t.Errorf("expected job_id in output: %s", stdout.String())
	}
}
