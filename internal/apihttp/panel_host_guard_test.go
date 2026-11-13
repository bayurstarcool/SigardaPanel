package apihttp

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"sigardapanel/internal/auth"
)

func TestPanelHostRootActionsNeedSuperAdmin(t *testing.T) {
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true}`))
	}))
	defer agent.Close()
	e, s, ctx := newTestApp(t)
	admin := roleToken(t, s, ctx, "adm", auth.RoleAdmin)
	root := roleToken(t, s, ctx, "boss", auth.RoleSuperAdmin)
	local := strconv.FormatInt(addServer(t, s, ctx, "local", agent.URL), 10) // loopback = panel host

	for _, c := range []struct {
		path string
		body map[string]any
	}{
		{"/docker/container/create", map[string]any{"image": "alpine", "volumes": []string{"/:/host"}}},
		{"/docker/container/exec", map[string]any{"container_id": "x", "command": "id"}},
		{"/firewall/reset", map[string]any{}},
		{"/fail2ban/ban", map[string]any{"jail": "sshd", "ip": "1.2.3.4"}},
	} {
		if rec := doRequest(t, e, http.MethodPost, "/api/v1/servers/"+local+c.path, c.body, admin); rec.Code != http.StatusForbidden {
			t.Errorf("admin %s on the panel host = %d", c.path, rec.Code)
		}
	}
	if rec := doRequest(t, e, http.MethodPost, "/api/v1/servers/"+local+"/firewall/reset", map[string]any{}, root); rec.Code != http.StatusOK {
		t.Errorf("super_admin firewall reset on the panel host = %d %s", rec.Code, rec.Body.String())
	}
	if rec := doRequest(t, e, http.MethodPost, "/api/v1/terminal/tickets", map[string]any{"server_id": mustAtoi(local)}, admin); rec.Code != http.StatusForbidden {
		t.Errorf("admin root terminal on the panel host = %d %s", rec.Code, rec.Body.String())
	}

	// A remote server keeps the admin role check, and the action is audited.
	ln, err := net.Listen("tcp", net.JoinHostPort(remoteIP(t), "0"))
	if err != nil {
		t.Skip(err)
	}
	remoteAgent := httptest.NewUnstartedServer(agent.Config.Handler)
	remoteAgent.Listener.Close()
	remoteAgent.Listener = ln
	remoteAgent.Start()
	defer remoteAgent.Close()
	remote := strconv.FormatInt(addServer(t, s, ctx, "remote", remoteAgent.URL), 10)
	if rec := doRequest(t, e, http.MethodPost, "/api/v1/servers/"+remote+"/firewall/deny", map[string]any{"from": "1.2.3.4"}, admin); rec.Code != http.StatusOK {
		t.Fatalf("admin firewall deny on a remote server = %d %s", rec.Code, rec.Body.String())
	}
	var n int
	s.DB().QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action = 'server.firewall.deny'`).Scan(&n)
	if n != 1 {
		t.Fatalf("firewall action audit rows = %d", n)
	}
}

func mustAtoi(s string) int64 { n, _ := strconv.ParseInt(s, 10, 64); return n }
