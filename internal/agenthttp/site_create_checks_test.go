package agenthttp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func postSiteCreate(t *testing.T, opts RouterOptions, payload map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/tasks/site/create", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	NewRouterWithOptions(opts).ServeHTTP(rec, req)
	return rec
}

func phpSocketDir(t *testing.T, versions ...string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "php")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, v := range versions {
		if err := os.WriteFile(filepath.Join(dir, "php"+v+"-fpm.sock"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestSiteCreate_PHPWithoutFPMFailsFast(t *testing.T) {
	setupTestBinaries(t)
	root := filepath.Join(t.TempDir(), "site")
	nginxDir := t.TempDir()
	opts := RouterOptions{NginxConfDir: nginxDir, PHPRunDir: phpSocketDir(t, "8.2")}

	// 7.0 has no socket here and no CloudPanel TCP port.
	rec := postSiteCreate(t, opts, map[string]any{
		"site_id": 1, "domain": "php.example.com", "runtime": "php", "root_path": root, "php_version": "7.0",
	})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d %s", rec.Code, rec.Body.String())
	}
	msg := rec.Body.String()
	for _, want := range []string{"PHP-FPM 7.0 is not installed on this server", "php7.0-fpm.sock", "Installed PHP-FPM versions: 8.2"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error should mention %q: %s", want, msg)
		}
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("nothing may be created when the precondition fails")
	}
	if entries, _ := os.ReadDir(nginxDir); len(entries) != 0 {
		t.Fatalf("no vhost may be written: %v", entries)
	}
}

func TestSiteCreate_PHPDefaultVersionNeedsADefaultSocket(t *testing.T) {
	setupTestBinaries(t)
	// Only 8.5 installed: a vhost without php_version would point at a
	// default socket that does not exist.
	rec := postSiteCreate(t, RouterOptions{NginxConfDir: t.TempDir(), PHPRunDir: phpSocketDir(t, "8.5")}, map[string]any{
		"site_id": 1, "domain": "php.example.com", "runtime": "php", "root_path": filepath.Join(t.TempDir(), "s"),
	})
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "Installed PHP-FPM versions: 8.5") {
		t.Fatalf("expected 422 listing 8.5, got %d %s", rec.Code, rec.Body.String())
	}

	rec = postSiteCreate(t, RouterOptions{NginxConfDir: t.TempDir(), PHPRunDir: phpSocketDir(t)}, map[string]any{
		"site_id": 1, "domain": "php.example.com", "runtime": "php", "root_path": filepath.Join(t.TempDir(), "s"),
	})
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "No PHP-FPM is installed on this server") {
		t.Fatalf("expected 422 without any PHP, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestSiteCreate_PHPWithInstalledFPMSucceeds(t *testing.T) {
	setupTestBinaries(t)
	for _, version := range []string{"8.3", ""} {
		rec := postSiteCreate(t, RouterOptions{NginxConfDir: t.TempDir(), PHPRunDir: phpSocketDir(t, "8.3")}, map[string]any{
			"site_id": 1, "domain": "php.example.com", "runtime": "php", "root_path": filepath.Join(t.TempDir(), "s"), "php_version": version,
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("php_version %q: expected 200, got %d %s", version, rec.Code, rec.Body.String())
		}
	}
}

func TestSiteCreate_RejectsBadPHPVersion(t *testing.T) {
	setupTestBinaries(t)
	rec := postSiteCreate(t, RouterOptions{NginxConfDir: t.TempDir()}, map[string]any{
		"site_id": 1, "domain": "x.example.com", "runtime": "php", "root_path": filepath.Join(t.TempDir(), "s"), "php_version": "8.3;rm",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d %s", rec.Code, rec.Body.String())
	}
}
