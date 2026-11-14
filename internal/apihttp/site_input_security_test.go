package apihttp

import (
	"database/sql"
	"net/http"
	"strings"
	"testing"

	"sigardapanel/internal/auth"
	"sigardapanel/internal/store"
)

func TestCreateSite_RejectsRootEscalationInputs(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	userToken := newUserToken(t, s, ctx) // role=user, username "regular"
	srvID := createServer(t, s, ctx)

	base := func() map[string]any {
		return map[string]any{"server_id": srvID, "name": "x", "domain": "x.example.com", "runtime": "proxy",
			"proxy_pass_url": "http://127.0.0.1:3000"}
	}
	cases := map[string]struct {
		mutate func(map[string]any)
		want   int
	}{
		"nginx injection via proxy_pass_url": {func(b map[string]any) {
			b["proxy_pass_url"] = "http://x;\n}\nlocation /p/ {alias /home/;autoindex on;\n"
		}, http.StatusBadRequest},
		"root_path with newline": {func(b map[string]any) { b["root_path"] = "/home/regular/x\n}\nserver {" }, http.StatusBadRequest},
		"other tenant's home":    {func(b map[string]any) { b["root_path"] = "/home/someone/htdocs/x" }, http.StatusForbidden},
		"system tree":            {func(b map[string]any) { b["root_path"] = "/etc/nginx" }, http.StatusBadRequest},
		"docker host bind mount": {func(b map[string]any) {
			b["runtime"] = "docker"
			b["docker_image"] = "nginx"
			b["docker_volume"] = "/:/host"
		}, http.StatusForbidden}, // docker sites are super_admin only
		"docker option-like image": {func(b map[string]any) { b["runtime"] = "docker"; b["docker_image"] = "--privileged" }, http.StatusForbidden},
		"bad php version":          {func(b map[string]any) { b["runtime"] = "php"; b["php_version"] = "8.3;x" }, http.StatusBadRequest},
	}
	for name, tc := range cases {
		b := base()
		tc.mutate(b)
		rec := doRequest(t, e, "POST", "/api/v1/sites", b, userToken)
		if rec.Code != tc.want {
			t.Errorf("%s: got %d want %d (%s)", name, rec.Code, tc.want, strings.TrimSpace(rec.Body.String()))
		}
	}

	// A non-admin cannot choose the unix account: system_user is pinned to their own.
	b := base()
	b["system_user"] = "root"
	rec := doRequest(t, e, "POST", "/api/v1/sites", b, userToken)
	if rec.Code != http.StatusCreated {
		t.Fatalf("valid site rejected: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"system_user":"regular"`) {
		t.Errorf("system_user not pinned to owner: %s", rec.Body.String())
	}
}

// The site.create job tells the agent which Linux account owns the site, so
// remote servers create it (and chown the new root) like the panel host.
func TestCreateSite_JobPayloadCarriesSystemUser(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)
	admin, _ := s.Users.GetByUsername(ctx, "admin")
	// A tenant admin runs sites as one of the users it created.
	if err := s.Users.Create(ctx, &store.User{Username: "alice", Email: "alice@example.com", PasswordHash: "x", Role: auth.RoleUser, CreatedBy: sql.NullInt64{Int64: admin.ID, Valid: true}}); err != nil {
		t.Fatal(err)
	}
	srvID := createServer(t, s, ctx)
	rec := doRequest(t, e, "POST", "/api/v1/sites", map[string]any{
		"server_id": srvID, "name": "x", "domain": "owner.example.com", "runtime": "static", "system_user": "alice",
	}, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	jobs, err := s.Jobs.List(ctx, store.JobListFilter{Type: "site.create"})
	if err != nil || len(jobs) == 0 {
		t.Fatalf("no job queued: %v", err)
	}
	if !strings.Contains(jobs[0].Payload, `"system_user":"alice"`) {
		t.Fatalf("site.create payload lacks system_user: %s", jobs[0].Payload)
	}
}

func TestCreateSite_AdminSystemUserMustNotBeRoot(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)
	srvID := createServer(t, s, ctx)
	rec := doRequest(t, e, "POST", "/api/v1/sites", map[string]any{
		"server_id": srvID, "name": "x", "domain": "y.example.com", "runtime": "static", "system_user": "root",
	}, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("admin system_user=root: got %d", rec.Code)
	}
}
