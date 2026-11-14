package apihttp

import (
	"net"
	"net/http"
	"strconv"
	"testing"
)

func TestNormalizeAgentURL(t *testing.T) {
	orig := lookupAgentHost
	lookupAgentHost = func(host string) ([]net.IP, error) {
		if host == "metadata.internal" {
			return []net.IP{net.ParseIP("169.254.169.254")}, nil
		}
		return []net.IP{net.ParseIP("203.0.113.9")}, nil
	}
	t.Cleanup(func() { lookupAgentHost = orig })
	t.Setenv("SIGARDAPANEL_ALLOW_PRIVATE_AGENTS", "")

	good := map[string]string{
		"http://203.0.113.5:7710":    "http://203.0.113.5:7710",
		"https://agent.example.com/": "https://agent.example.com",
	}
	for in, want := range good {
		if got, err := normalizeAgentURL(in, false); err != nil || got != want {
			t.Errorf("normalizeAgentURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{
		"http://169.254.169.254/latest/meta-data/#",
		"http://203.0.113.5:7710/latest/meta-data",
		"http://203.0.113.5:7710?x=1",
		"http://user:pw@203.0.113.5:7710",
		"http://127.0.0.1:7700",
		"http://localhost:7710",
		"http://metadata.internal:80",
		"http://10.0.0.5:7710",
		"ftp://203.0.113.5",
	} {
		if got, err := normalizeAgentURL(bad, false); err == nil {
			t.Errorf("accepted %q → %q", bad, got)
		}
	}
	t.Setenv("SIGARDAPANEL_ALLOW_PRIVATE_AGENTS", "true")
	if _, err := normalizeAgentURL("http://10.0.0.5:7710", false); err != nil {
		t.Errorf("private agent with the opt-in: %v", err)
	}
	if _, err := normalizeAgentURL("http://127.0.0.1:7710", true); err != nil {
		t.Errorf("loopback for the panel's own server: %v", err)
	}
}

func TestLocalServerTokenRegenRefused(t *testing.T) {
	e, s, ctx := newTestApp(t)
	tok := newSuperAdminToken(t, s, ctx)
	id := addServer(t, s, ctx, "local", "http://127.0.0.1:7710")
	if rec := doRequest(t, e, http.MethodPost, "/api/v1/servers/"+strconv.FormatInt(id, 10)+"/token", nil, tok); rec.Code != http.StatusConflict {
		t.Fatalf("local token regen = %d %s", rec.Code, rec.Body.String())
	}
}
