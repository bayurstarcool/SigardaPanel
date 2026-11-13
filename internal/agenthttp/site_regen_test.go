package agenthttp

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sigardapanel/internal/executor"
)

func TestSiteCreateRegenWithSecurityAndLetsEncrypt(t *testing.T) {
	setupTestBinaries(t)
	root := filepath.Join(t.TempDir(), "site")
	os.MkdirAll(root, 0o755)
	nginxDir, leDir := t.TempDir(), t.TempDir()
	live := filepath.Join(leDir, "live", "regen.example.com")
	os.MkdirAll(live, 0o755)
	os.WriteFile(filepath.Join(live, "fullchain.pem"), []byte("c"), 0o644)
	os.WriteFile(filepath.Join(live, "privkey.pem"), []byte("k"), 0o600)
	opts := RouterOptions{NginxConfDir: nginxDir, LetsEncryptDir: leDir}

	rec := postSiteCreate(t, opts, map[string]any{
		"site_id": 1, "domain": "regen.example.com", "runtime": "static", "root_path": root, "vhost_regen": true,
		"security": map[string]any{"blocked_ips": []string{"1.2.3.4", "10.0.0.0/8"}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("regen = %d %s", rec.Code, rec.Body.String())
	}
	vhost, _ := os.ReadFile(filepath.Join(nginxDir, "regen.example.com.conf"))
	for _, want := range []string{"deny 1.2.3.4;", "deny 10.0.0.0/8;", "ssl_certificate " + filepath.Join(live, "fullchain.pem") + ";"} {
		if !strings.Contains(string(vhost), want) {
			t.Errorf("vhost lacks %q", want)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "index.html")); err == nil {
		t.Error("a vhost regeneration must not write starter files")
	}

	// The agent re-validates: an injected value is refused and nothing is written.
	os.Remove(filepath.Join(nginxDir, "regen.example.com.conf"))
	rec = postSiteCreate(t, opts, map[string]any{
		"site_id": 1, "domain": "regen.example.com", "runtime": "static", "root_path": root, "vhost_regen": true,
		"security": map[string]any{"blocked_ips": []string{"1.2.3.4; } location /x/ { alias /etc/; }"}},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("injected security = %d %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(nginxDir, "regen.example.com.conf")); err == nil {
		t.Fatal("a vhost was written for an injected value")
	}
}

func TestServeLetsEncryptCertSwitchesPlaceholder(t *testing.T) {
	bin := setupTestBinaries(t)
	os.WriteFile(filepath.Join(bin, "nginx"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	nginxDir, leDir := t.TempDir(), t.TempDir()
	live := filepath.Join(leDir, "live", "le.example.com")
	os.MkdirAll(live, 0o755)
	os.WriteFile(filepath.Join(live, "fullchain.pem"), []byte("c"), 0o644)
	os.WriteFile(filepath.Join(live, "privkey.pem"), []byte("k"), 0o600)
	conf := filepath.Join(nginxDir, "le.example.com.conf")
	os.WriteFile(conf, []byte("server {\n    ssl_certificate /etc/nginx/ssl/sigardapanel.crt;\n    ssl_certificate_key /etc/nginx/ssl/sigardapanel.key;\n}\n"), 0o644)
	r := &Router{nginxConfDir: nginxDir, leDir: leDir, executor: &executor.Executor{}}
	if err := r.serveLetsEncryptCert(t.Context(), "le.example.com"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(conf)
	if !strings.Contains(string(got), "ssl_certificate "+filepath.Join(live, "fullchain.pem")+";") || strings.Contains(string(got), "sigardapanel.crt") {
		t.Fatalf("vhost not switched:\n%s", got)
	}
}
