package apihttp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
)

func registrationToken(t *testing.T, e *echo.Echo, admin string) string {
	t.Helper()
	rec := doRequest(t, e, "POST", "/api/v1/agents/tokens", nil, admin)
	var tr struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&tr)
	if tr.Data.Token == "" {
		t.Fatalf("no registration token: %d %s", rec.Code, rec.Body.String())
	}
	return tr.Data.Token
}

// A TLS-capable installer registers the agent at https with its pin; the
// pin follows a re-install (even from the earlier http registration) and an
// invalid fingerprint is refused.
func TestRegisterAgentStoresTLSPin(t *testing.T) {
	e, s, ctx := newTestApp(t)
	admin := newSuperAdminToken(t, s, ctx)

	// Registered over http first (an installer from before TLS).
	rec := doRequest(t, e, "POST", "/api/v1/agents/register", map[string]any{
		"registration_token": registrationToken(t, e, admin), "hostname": "web1", "ip_address": "203.0.113.7", "agent_token": "tok-1",
	}, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("http register = %d %s", rec.Code, rec.Body.String())
	}

	if rec := doRequest(t, e, "POST", "/api/v1/agents/register", map[string]any{
		"registration_token": registrationToken(t, e, admin), "hostname": "web1", "ip_address": "203.0.113.7", "agent_token": "tok-2",
		"previous_agent_token": "tok-1", "tls_fingerprint": "not-a-fingerprint",
	}, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad fingerprint = %d %s", rec.Code, rec.Body.String())
	}

	fp := strings.Repeat("AB", 32)
	rec = doRequest(t, e, "POST", "/api/v1/agents/register", map[string]any{
		"registration_token": registrationToken(t, e, admin), "hostname": "web1", "ip_address": "203.0.113.7", "agent_token": "tok-2",
		"previous_agent_token": "tok-1", "tls_fingerprint": fp,
	}, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"rebound":true`) {
		t.Fatalf("tls re-register = %d %s", rec.Code, rec.Body.String())
	}
	servers, _ := s.Servers.List(ctx)
	var n int
	for _, srv := range servers {
		if srv.IPAddress.String != "203.0.113.7" {
			continue
		}
		n++
		if srv.AgentURL.String != "https://203.0.113.7:7710" || srv.AgentTLSFingerprint != strings.ToLower(fp) || srv.AgentToken != "tok-2" {
			t.Fatalf("server = %s %s %s", srv.AgentURL.String, srv.AgentTLSFingerprint, srv.AgentToken)
		}
	}
	if n != 1 {
		t.Fatalf("%d server rows for one agent", n)
	}
	pin, err := s.Servers.TLSPinForAddr(ctx, "203.0.113.7:7710")
	if err != nil || pin != strings.ToLower(fp) {
		t.Fatalf("pin lookup = %q %v", pin, err)
	}
	if pin, _ := s.Servers.TLSPinForAddr(ctx, "203.0.113.8:7710"); pin != "" {
		t.Fatalf("pin for another address = %q", pin)
	}

	// The servers list flags plain-http remote agents, not pinned ones.
	rec = doRequest(t, e, "GET", "/api/v1/servers", nil, admin)
	body := rec.Body.String()
	if !strings.Contains(body, `"tls_pinned":true`) {
		t.Fatalf("servers list = %s", body)
	}
}

func TestInstallScriptCarriesTLSAndFirewall(t *testing.T) {
	script := fmt.Sprintf(installScriptTemplate, shellQuote("https://panel.example.com"), shellQuote("tok"),
		shellQuote("v1.9.1"), shellQuote(releasePublicKey), shellQuote("0"), shellQuote("203.0.113.9"))
	for _, want := range []string{
		"openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1",
		"SIGARDAPANEL_AGENT_TLS_CERT=",
		"SIGARDAPANEL_AGENT_TLS_KEY=",
		"tls_fingerprint:$tls_fingerprint",
		"https://127.0.0.1:7710/health",
		`ufw allow from "$PANEL_IP" to any port 7710 proto tcp`,
		"PANEL_IP_DEFAULT='203.0.113.9'",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("install script lacks %q", want)
		}
	}
	// ufw is never enabled by the installer (that could lock out SSH).
	if strings.Contains(script, "ufw enable") || strings.Contains(script, "ufw --force enable") {
		t.Fatal("installer enables ufw")
	}
}
