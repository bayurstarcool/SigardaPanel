package apihttp

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"sigardapanel/internal/agenthttp"
	"sigardapanel/internal/auth"
	"sigardapanel/internal/store"
)

func TestVhostEditingRunsOnTheSiteAgent(t *testing.T) {
	bin := t.TempDir()
	// Fake nginx: -t fails when any vhost contains BROKEN.
	confDir := t.TempDir()
	os.WriteFile(filepath.Join(bin, "nginx"), []byte("#!/bin/sh\nif [ \"$1\" = \"-t\" ]; then if grep -rq BROKEN "+confDir+"; then echo 'nginx: [emerg] unknown directive \"BROKEN\"'; exit 1; fi; echo 'syntax is ok'; fi\nexit 0\n"), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	agent := httptest.NewServer(agenthttp.NewRouterWithOptions(agenthttp.RouterOptions{NginxConfDir: confDir}))
	defer agent.Close()

	e, s, ctx := newTestApp(t)
	root := roleToken(t, s, ctx, "root", auth.RoleSuperAdmin)
	admin := roleToken(t, s, ctx, "adm", auth.RoleAdmin)
	srv := &store.Server{Name: "panel", Hostname: "p", Status: "active", AgentToken: "tok", AgentURL: sql.NullString{String: agent.URL, Valid: true}}
	s.Servers.Create(ctx, srv)
	admUser, _ := s.Users.GetByUsername(ctx, "adm")
	site := &store.Site{ServerID: srv.ID, Name: "v", Domain: "v.example.com", Runtime: "static", RootPath: "/home/u/htdocs/v.example.com", Status: "active",
		OwnerID: sql.NullInt64{Int64: admUser.ID, Valid: true}} // the admin owns it: only the panel-host rule stops them
	s.Sites.Create(ctx, site)
	base := "/api/v1/sites/" + strconv.FormatInt(site.ID, 10) + "/vhost"

	// On the panel host (loopback agent) raw vhosts are super_admin only.
	if rec := doRequest(t, e, http.MethodPut, base, map[string]any{"vhost_config": "server {}"}, admin); rec.Code != http.StatusForbidden {
		t.Fatalf("admin vhost save on the panel host = %d", rec.Code)
	}
	rec := doRequest(t, e, http.MethodPut, base, map[string]any{"vhost_config": "server { listen 80; }"}, root)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"success"`) {
		t.Fatalf("save = %d %s", rec.Code, rec.Body.String())
	}
	if got, _ := os.ReadFile(filepath.Join(confDir, "v.example.com.conf")); string(got) != "server { listen 80; }" {
		t.Fatalf("vhost on the agent = %q", got)
	}
	rec = doRequest(t, e, http.MethodPut, base, map[string]any{"vhost_config": "BROKEN"}, root)
	if !strings.Contains(rec.Body.String(), "saved_nginx_invalid") {
		t.Fatalf("invalid save = %s", rec.Body.String())
	}
	if got, _ := os.ReadFile(filepath.Join(confDir, "v.example.com.conf")); string(got) != "server { listen 80; }" {
		t.Fatalf("previous vhost not kept: %q", got)
	}
	rec = doRequest(t, e, http.MethodPost, base+"/validate", map[string]any{"vhost_config": "BROKEN"}, root)
	if !strings.Contains(rec.Body.String(), `"valid":false`) || !strings.Contains(rec.Body.String(), "unknown directive") {
		t.Fatalf("validate = %s", rec.Body.String())
	}
	if got, _ := os.ReadFile(filepath.Join(confDir, "v.example.com.conf")); string(got) != "server { listen 80; }" {
		t.Fatalf("validation left the candidate in place: %q", got)
	}
	rec = doRequest(t, e, http.MethodGet, base, nil, root)
	if !strings.Contains(rec.Body.String(), `"source":"nginx"`) || !strings.Contains(rec.Body.String(), "listen 80") {
		t.Fatalf("get = %s", rec.Body.String())
	}
}
