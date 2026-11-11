package agenthttp

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestSiteSuspendResume_MovesVhostAndRenewal(t *testing.T) {
	setupTestBinaries(t)
	base := t.TempDir()
	enabled := filepath.Join(base, "nginx", "sites-enabled")
	le := filepath.Join(base, "letsencrypt")
	for _, d := range []string{enabled, filepath.Join(le, "renewal")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	vhost := filepath.Join(enabled, "sus.example.com.conf")
	renewal := filepath.Join(le, "renewal", "sus.example.com.conf")
	os.WriteFile(vhost, []byte("server {}"), 0o644)
	os.WriteFile(renewal, []byte("[renewalparams]"), 0o644)

	e := NewRouterWithOptions(RouterOptions{NginxConfDir: enabled, LetsEncryptDir: le})
	call := func(path string) int {
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader([]byte(`{"site_id":7,"domain":"sus.example.com"}`)))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := call("/tasks/site/suspend"); code != http.StatusOK {
		t.Fatalf("suspend: %d", code)
	}
	// The original vhost is replaced by the 503 placeholder.
	if data, err := os.ReadFile(vhost); err != nil || !isSuspendedPlaceholder(data) {
		t.Fatalf("expected the suspended placeholder in sites-enabled: %v %q", err, data)
	}
	if _, err := os.Stat(filepath.Join(base, "nginx", "sites-disabled", "sus.example.com.conf")); err != nil {
		t.Fatalf("vhost not in sites-disabled: %v", err)
	}
	if _, err := os.Stat(filepath.Join(le, "renewal-disabled", "sus.example.com.conf")); err != nil {
		t.Fatalf("renewal not disabled: %v", err)
	}

	// Suspending twice is a no-op, not an error.
	if code := call("/tasks/site/suspend"); code != http.StatusOK {
		t.Fatalf("second suspend: %d", code)
	}

	if code := call("/tasks/site/resume"); code != http.StatusOK {
		t.Fatalf("resume: %d", code)
	}
	for _, p := range []string{vhost, renewal} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("%s not restored: %v", p, err)
		}
	}
	if data, _ := os.ReadFile(vhost); string(data) != "server {}" {
		t.Fatalf("original vhost not restored: %q", data)
	}
}

func TestSiteSuspend_RejectsBadDomain(t *testing.T) {
	e := NewRouterWithOptions(RouterOptions{NginxConfDir: t.TempDir(), LetsEncryptDir: t.TempDir()})
	req := httptest.NewRequest(http.MethodPost, "/tasks/site/suspend", bytes.NewReader([]byte(`{"site_id":1,"domain":"../../etc/passwd"}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}
