package agenthttp

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sigardapanel/internal/agenttls"
)

func TestAgentTLSEnableCreatesPinnedCertificate(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, "agent.env")
	os.WriteFile(env, []byte("SIGARDAPANEL_AGENT_ADDR=:7710\nSIGARDAPANEL_AGENT_TOKEN=x\nSIGARDAPANEL_AGENT_TLS_CERT=/old\n"), 0o600)
	tlsDir := filepath.Join(dir, "tls")
	restarted := make(chan struct{}, 1)
	e := NewRouterWithOptions(RouterOptions{
		AgentTLSDir: tlsDir, AgentTLSEnvFile: env,
		AgentTLSActive:  func() string { return "" },
		AgentTLSRestart: func() { restarted <- struct{}{} },
	})

	req := httptest.NewRequest(http.MethodPost, "/tasks/agent/tls/enable", strings.NewReader(`{"ip":"203.0.113.5"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("enable = %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Fingerprint string `json:"fingerprint"`
		Restarting  bool   `json:"restarting"`
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	pair, err := tls.LoadX509KeyPair(filepath.Join(tlsDir, "agent.crt"), filepath.Join(tlsDir, "agent.key"))
	if err != nil {
		t.Fatalf("key pair: %v", err)
	}
	if out.Fingerprint != agenttls.Fingerprint(pair.Certificate[0]) || !out.Restarting {
		t.Fatalf("answer = %+v", out)
	}
	<-restarted
	if fi, _ := os.Stat(filepath.Join(tlsDir, "agent.key")); fi.Mode().Perm() != 0o600 {
		t.Fatalf("key mode = %v", fi.Mode().Perm())
	}
	raw, _ := os.ReadFile(env)
	s := string(raw)
	if !strings.Contains(s, "SIGARDAPANEL_AGENT_TLS_CERT="+filepath.Join(tlsDir, "agent.crt")) || strings.Contains(s, "=/old") ||
		!strings.Contains(s, "SIGARDAPANEL_AGENT_TOKEN=x") || strings.Count(s, "SIGARDAPANEL_AGENT_TLS_KEY=") != 1 {
		t.Fatalf("agent.env = %q", s)
	}

	// A second call keeps the same certificate (the panel's pin stays valid).
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/tasks/agent/tls/enable", strings.NewReader(`{}`)))
	if !strings.Contains(rec.Body.String(), out.Fingerprint) {
		t.Fatalf("second enable = %s", rec.Body.String())
	}
	// Status reports the fingerprint.
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tasks/agent/tls", nil))
	if !strings.Contains(rec.Body.String(), out.Fingerprint) {
		t.Fatalf("status = %s", rec.Body.String())
	}
}

func TestAgentTLSEnableRefusedWithoutAgentEnv(t *testing.T) {
	e := NewRouterWithOptions(RouterOptions{AgentTLSDir: t.TempDir(), AgentTLSEnvFile: filepath.Join(t.TempDir(), "missing.env")})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/tasks/agent/tls/enable", strings.NewReader(`{}`)))
	if rec.Code != http.StatusConflict {
		t.Fatalf("without agent.env = %d %s", rec.Code, rec.Body.String())
	}
}
