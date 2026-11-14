package agenttls

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func pinFor(srv *httptest.Server) string {
	return Fingerprint(srv.Certificate().Raw)
}

func withLoader(t *testing.T, pins map[string]string) {
	t.Helper()
	SetLoader(func(_ context.Context, addr string) (string, error) { return pins[addr], nil })
	t.Cleanup(func() { SetLoader(nil) })
}

func TestPinnedTLS(t *testing.T) {
	agent := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok") }))
	defer agent.Close()

	addr := strings.TrimPrefix(agent.URL, "https://")
	client := &http.Client{Timeout: 5 * time.Second, Transport: Transport(nil)}

	// No pin stored: refused (an unpinned self-signed cert is never accepted).
	withLoader(t, map[string]string{})
	if _, err := client.Get(agent.URL); err == nil || !strings.Contains(err.Error(), "not pinned") {
		t.Fatalf("unpinned = %v", err)
	}
	// Pin of another certificate: refused.
	withLoader(t, map[string]string{addr: strings.Repeat("ef", 32)})
	client.CloseIdleConnections()
	if _, err := client.Get(agent.URL); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("wrong pin = %v", err)
	}
	// Matching pin (any spelling): accepted.
	upper := strings.ToUpper(pinFor(agent))
	withLoader(t, map[string]string{addr: upper[:2] + ":" + upper[2:]})
	client.CloseIdleConnections()
	resp, err := client.Get(agent.URL)
	if err != nil {
		t.Fatalf("pinned = %v", err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(b) != "ok" {
		t.Fatalf("body = %q", b)
	}
	// Plain http agents keep working.
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "plain") }))
	defer plain.Close()
	if resp, err := client.Get(plain.URL); err != nil {
		t.Fatalf("http = %v", err)
	} else {
		resp.Body.Close()
	}
}

func TestNormalizeAndHelpers(t *testing.T) {
	fp := strings.Repeat("ab", 32)
	if Normalize("AB:"+strings.Repeat("AB", 31)) != fp || Normalize("sha256="+fp) != fp {
		t.Fatal("normalize")
	}
	if Normalize("abc") != "" || Normalize(strings.Repeat("zz", 32)) != "" {
		t.Fatal("bad fingerprints accepted")
	}
	if AddrOf("https://203.0.113.5:7710") != "203.0.113.5:7710" || AddrOf("https://Agent.Example") != "agent.example:443" {
		t.Fatal("AddrOf")
	}
	for u, want := range map[string]bool{
		"http://203.0.113.5:7710": true, "http://agent.example:7710": true,
		"http://127.0.0.1:7710": false, "http://localhost:7710": false, "https://203.0.113.5:7710": false,
	} {
		if InsecureTransport(u) != want {
			t.Fatalf("InsecureTransport(%s) != %v", u, want)
		}
	}
}

func TestUpgrade(t *testing.T) {
	fp := strings.Repeat("cd", 32)
	var gotAuth, gotIP string
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tasks/agent/tls/enable" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		gotAuth = r.Header.Get("Authorization")
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		gotIP = body["ip"]
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "fingerprint": strings.ToUpper(fp)})
	}))
	defer agent.Close()
	// The agent "lives" at a public name; dial the test listener instead.
	ln := strings.TrimPrefix(agent.URL, "http://")
	client := &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, ln)
	}}}
	newURL, got, err := Upgrade(context.Background(), client, "http://agent.example:7710", "tok", "203.0.113.5")
	if err != nil {
		t.Fatal(err)
	}
	if newURL != "https://agent.example:7710" || got != fp || gotAuth != "Bearer tok" || gotIP != "203.0.113.5" {
		t.Fatalf("upgrade = %s %s auth=%q ip=%q", newURL, got, gotAuth, gotIP)
	}
	if _, _, err := Upgrade(context.Background(), client, "http://127.0.0.1:7710", "tok", ""); err == nil {
		t.Fatal("loopback agent upgraded")
	}
	if _, _, err := Upgrade(context.Background(), client, "https://agent.example:7710", "tok", ""); err == nil {
		t.Fatal("https agent upgraded again")
	}
	old := httptest.NewServer(http.NotFoundHandler())
	defer old.Close()
	ln2 := strings.TrimPrefix(old.URL, "http://")
	client2 := &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, ln2)
	}}}
	if _, _, err := Upgrade(context.Background(), client2, "http://agent.example:7710", "tok", ""); err != ErrAgentTooOld {
		t.Fatalf("old agent = %v", err)
	}
}
