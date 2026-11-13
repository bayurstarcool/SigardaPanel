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

func setupMailFiles(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig := []string{emailPasswdFile, emailMailboxFile, emailDomainsFile, emailMailboxRoot, emailPostfixCheck}
	emailPasswdFile = filepath.Join(dir, "passwd")
	emailMailboxFile = filepath.Join(dir, "vmailbox")
	emailDomainsFile = filepath.Join(dir, "vdomains")
	emailMailboxRoot = filepath.Join(dir, "vhosts")
	emailPostfixCheck = filepath.Join(dir, "main.cf")
	t.Cleanup(func() {
		emailPasswdFile, emailMailboxFile, emailDomainsFile, emailMailboxRoot, emailPostfixCheck = orig[0], orig[1], orig[2], orig[3], orig[4]
	})
	os.WriteFile(emailPasswdFile, nil, 0o600)
	os.WriteFile(emailPostfixCheck, nil, 0o644)
	bin := setupTestBinaries(t)
	os.WriteFile(filepath.Join(bin, "doveadm"), []byte("#!/bin/sh\necho '{SHA512-CRYPT}$6$salt$hash'\n"), 0o755)
	for _, b := range []string{"postmap", "chown"} {
		os.WriteFile(filepath.Join(bin, b), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	}
	return dir
}

func mail(t *testing.T, h http.Handler, body map[string]any) (int, string) {
	t.Helper()
	rec := postJSON(t, h, "/tasks/email/manage", body)
	return rec.Code, rec.Body.String()
}

func TestEmailAccountLifecycle(t *testing.T) {
	setupMailFiles(t)
	h := NewRouterWithOptions(RouterOptions{})

	if code, body := mail(t, h, map[string]any{"action": "create", "domain": "example.com", "email": "Info@example.com", "password": "s3cret-pass!"}); code != http.StatusOK {
		t.Fatalf("create = %d %s", code, body)
	}
	mail(t, h, map[string]any{"action": "create", "domain": "example.com", "email": "sales@example.com", "password": "s3cret-pass!"})
	if code, _ := mail(t, h, map[string]any{"action": "create", "domain": "example.com", "email": "info@example.com", "password": "another-pass"}); code != http.StatusConflict {
		t.Fatalf("duplicate create = %d", code)
	}
	if code, _ := mail(t, h, map[string]any{"action": "create", "domain": "example.com", "email": "x@other.org", "password": "s3cret-pass!"}); code != http.StatusBadRequest {
		t.Fatalf("address outside the domain = %d", code)
	}
	_, list := mail(t, h, map[string]any{"action": "list", "domain": ""})
	if !strings.Contains(list, "info@example.com") || !strings.Contains(list, "sales@example.com") {
		t.Fatalf("list = %s", list)
	}
	domains, _ := os.ReadFile(emailDomainsFile)
	if strings.Count(string(domains), "example.com OK") != 1 {
		t.Fatalf("vdomains = %q", domains)
	}
	if code, _ := mail(t, h, map[string]any{"action": "update-password", "email": "info@example.com", "password": "new-pass-123"}); code != http.StatusOK {
		t.Fatalf("update-password = %d", code)
	}
	// Delete is exact: "example.com" as an address must not wipe the domain.
	if code, _ := mail(t, h, map[string]any{"action": "delete", "email": "example.com"}); code != http.StatusBadRequest {
		t.Fatalf("delete with a bare domain = %d", code)
	}
	if code, _ := mail(t, h, map[string]any{"action": "delete", "email": "info@example.com"}); code != http.StatusOK {
		t.Fatalf("delete = %d", code)
	}
	passwd, _ := os.ReadFile(emailPasswdFile)
	if strings.Contains(string(passwd), "info@") || !strings.Contains(string(passwd), "sales@example.com:") {
		t.Fatalf("passwd after delete = %q", passwd)
	}
	if fi, _ := os.Stat(emailPasswdFile); fi.Mode().Perm() != 0o600 {
		t.Fatalf("passwd mode = %v (holds password hashes)", fi.Mode().Perm())
	}
	_, status := mail(t, h, map[string]any{"action": "status"})
	if !strings.Contains(status, `"installed":true`) {
		t.Fatalf("status = %s", status)
	}
}

func TestBotBlockerKeepsRulesAndRollsBack(t *testing.T) {
	bin := setupTestBinaries(t)
	dir := t.TempDir()
	origA, origI := botAgentsConf, botIPsConf
	botAgentsConf, botIPsConf = filepath.Join(dir, "bot-blocker.conf"), filepath.Join(dir, "bot-ips.conf")
	t.Cleanup(func() { botAgentsConf, botIPsConf = origA, origI })
	os.WriteFile(botAgentsConf, []byte("map $http_user_agent $bad_bot {\n    default 0;\n    ~*(curl|wget) 0;\n    ~*(nikto|sqlmap) 1;\n}\n"), 0o644)
	os.WriteFile(botIPsConf, []byte("deny 1.2.3.4;\ndeny 1.2.3.40;\n"), 0o644)
	h := NewRouterWithOptions(RouterOptions{})

	if rec := postJSON(t, h, "/tasks/bot-blocker/manage", map[string]any{"action": "add-agent", "agents": []string{"Bad Bot"}}); rec.Code != http.StatusOK {
		t.Fatalf("add agent = %d %s", rec.Code, rec.Body.String())
	}
	conf, _ := os.ReadFile(botAgentsConf)
	for _, want := range []string{`"~*(curl|wget)" 0;`, `"~*(nikto|sqlmap)" 1;`, `"~*(Bad Bot)" 1;`} {
		if !strings.Contains(string(conf), want) {
			t.Errorf("config lacks %s:\n%s", want, conf)
		}
	}
	if rec := postJSON(t, h, "/tasks/bot-blocker/manage", map[string]any{"action": "remove-ip", "ips": []string{"1.2.3.4"}}); rec.Code != http.StatusOK {
		t.Fatalf("remove ip = %d %s", rec.Code, rec.Body.String())
	}
	ips, _ := os.ReadFile(botIPsConf)
	if strings.Contains(string(ips), "deny 1.2.3.4;") || !strings.Contains(string(ips), "deny 1.2.3.40;") {
		t.Fatalf("ips = %q", ips)
	}
	// nginx -t failing: the change is reverted.
	os.WriteFile(filepath.Join(bin, "nginx"), []byte("#!/bin/sh\necho 'emerg: broken'\nexit 1\n"), 0o755)
	before, _ := os.ReadFile(botIPsConf)
	if rec := postJSON(t, h, "/tasks/bot-blocker/manage", map[string]any{"action": "add-ip", "ips": []string{"5.6.7.8"}}); rec.Code == http.StatusOK {
		t.Fatal("a failed nginx -t reported success")
	}
	after, _ := os.ReadFile(botIPsConf)
	if string(after) != string(before) {
		t.Fatalf("config not restored after nginx -t failed:\n%s", after)
	}
}

func postJSON(t *testing.T, h http.Handler, path string, payload map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}
