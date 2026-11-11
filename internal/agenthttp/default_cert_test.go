package agenthttp

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureDefaultSSLCert_CreatesUsablePair(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ssl")
	if err := ensureDefaultSSLCert(dir); err != nil {
		t.Fatalf("ensureDefaultSSLCert: %v", err)
	}
	crt, key := filepath.Join(dir, "sigardapanel.crt"), filepath.Join(dir, "sigardapanel.key")
	if _, err := tls.LoadX509KeyPair(crt, key); err != nil {
		t.Fatalf("generated pair does not load: %v", err)
	}
	if fi, _ := os.Stat(key); fi.Mode().Perm() != 0o600 {
		t.Fatalf("key mode = %v, want 0600", fi.Mode().Perm())
	}
	before, _ := os.ReadFile(crt)
	if err := ensureDefaultSSLCert(dir); err != nil {
		t.Fatalf("second call: %v", err)
	}
	after, _ := os.ReadFile(crt)
	if !bytes.Equal(before, after) {
		t.Fatal("existing certificate was overwritten")
	}
}

func TestEnsureDefaultSSLCert_KeepsOperatorFiles(t *testing.T) {
	dir := t.TempDir()
	crt := filepath.Join(dir, "sigardapanel.crt")
	if err := os.WriteFile(crt, []byte("operator cert"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureDefaultSSLCert(dir); err == nil {
		t.Fatal("expected an error when only the certificate exists")
	}
	if b, _ := os.ReadFile(crt); string(b) != "operator cert" {
		t.Fatal("operator certificate was modified")
	}
}

// A vhost that fails "nginx -t" must not stay in sites-enabled: it would keep
// nginx from starting on the next restart.
func TestSiteCreateTask_RemovesVhostWhenNginxTestFails(t *testing.T) {
	binDir := t.TempDir()
	failingNginx := "#!/bin/sh\necho 'nginx: [emerg] broken' >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(binDir, "nginx"), []byte(failingNginx), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	nginxDir := t.TempDir()
	certDir := filepath.Join(t.TempDir(), "ssl")
	e := NewRouterWithOptions(RouterOptions{NginxConfDir: nginxDir, DefaultCertDir: certDir})
	body, _ := json.Marshal(map[string]any{
		"site_id": 1, "domain": "broken.example.com", "runtime": "static",
		"root_path": filepath.Join(t.TempDir(), "broken.example.com"),
	})
	req := httptest.NewRequest(http.MethodPost, "/tasks/site/create", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(nginxDir, "broken.example.com.conf")); !os.IsNotExist(err) {
		t.Fatalf("vhost left behind after failed nginx -t (stat err=%v)", err)
	}
	if _, err := os.Stat(filepath.Join(certDir, "sigardapanel.crt")); err != nil {
		t.Fatalf("placeholder certificate not created: %v", err)
	}
}

func TestAcmeContactUsable(t *testing.T) {
	for email, want := range map[string]bool{
		"ops@company.co.id":  true,
		"admin@localhost":    false,
		"alice@panel.local":  false,
		"bob@srv.test":       false,
		"x@example.com":      false,
		"":                   false,
		"no-at-sign":         false,
		"a@b":                false,
		"me@sub.example.org": true,
	} {
		if got := acmeContactUsable(email); got != want {
			t.Errorf("acmeContactUsable(%q) = %v, want %v", email, got, want)
		}
	}
}
