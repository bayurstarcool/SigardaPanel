package apihttp

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sigardapanel/internal/store"
)

func TestTenantIsolation_GitAndDocker(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	adminToken := newAdminToken(t, s, ctx)
	userToken := newUserToken(t, s, ctx) // "regular"

	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/docker/containers"):
			w.Write([]byte(`{"containers":[{"id":"aaa111","name":"site-mine"},{"id":"bbb222","name":"site-theirs"},{"id":"ccc333","name":"postgres"}]}`))
		case strings.Contains(r.URL.Path, "/docker/container/logs"):
			w.Write([]byte(`{"logs":"secret log line"}`))
		default:
			w.Write([]byte(`{"branches":["main"]}`))
		}
	}))
	defer agent.Close()

	srv := &store.Server{Name: "s", Hostname: "h", Status: "active", AgentToken: "tok",
		AgentURL: sql.NullString{String: agent.URL, Valid: true}}
	s.Servers.Create(ctx, srv)
	regular, _ := s.Users.GetByUsername(ctx, "regular")
	admin, _ := s.Users.GetByUsername(ctx, "admin")
	mine := &store.Site{ServerID: srv.ID, Name: "mine", Domain: "mine.example.com", Runtime: "docker", RootPath: "/home/regular/htdocs/mine", OwnerID: sql.NullInt64{Int64: regular.ID, Valid: true}}
	theirs := &store.Site{ServerID: srv.ID, Name: "theirs", Domain: "theirs.example.com", Runtime: "docker", RootPath: "/home/admin/htdocs/theirs", OwnerID: sql.NullInt64{Int64: admin.ID, Valid: true}}
	s.Sites.Create(ctx, mine)
	s.Sites.Create(ctx, theirs)

	get := func(path, token string) *httptest.ResponseRecorder { return doRequest(t, e, "GET", path, nil, token) }
	idStr := func(id int64) string { return itoa(id) }

	// git: another tenant's repository is invisible, own repository works.
	if rec := get("/api/v1/sites/"+idStr(theirs.ID)+"/git/branches", userToken); rec.Code != http.StatusNotFound {
		t.Errorf("foreign git branches: got %d, want 404", rec.Code)
	}
	if rec := get("/api/v1/sites/"+idStr(theirs.ID)+"/git/log", userToken); rec.Code != http.StatusNotFound {
		t.Errorf("foreign git log: got %d, want 404", rec.Code)
	}
	if rec := get("/api/v1/sites/"+idStr(mine.ID)+"/git/branches", userToken); rec.Code != http.StatusOK {
		t.Errorf("own git branches: got %d, want 200 (%s)", rec.Code, rec.Body.String())
	}

	// docker: a tenant only sees their own site's container.
	base := "/api/v1/servers/" + idStr(srv.ID)
	rec := get(base+"/docker/containers", userToken)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "site-mine") ||
		strings.Contains(rec.Body.String(), "site-theirs") || strings.Contains(rec.Body.String(), "postgres") {
		t.Errorf("tenant container list not filtered: %d %s", rec.Code, rec.Body.String())
	}
	if rec := get(base+"/docker/container/logs?container_id=aaa111", userToken); rec.Code != http.StatusOK {
		t.Errorf("own container logs: got %d", rec.Code)
	}
	for _, id := range []string{"bbb222", "ccc333", "site-theirs", ""} {
		if rec := get(base+"/docker/container/logs?container_id="+id, userToken); rec.Code != http.StatusNotFound {
			t.Errorf("logs of container %q: got %d, want 404", id, rec.Code)
		}
	}
	// admins still see everything; infra reads are admin-only.
	if rec := get(base+"/docker/containers", adminToken); !strings.Contains(rec.Body.String(), "postgres") {
		t.Errorf("admin should see all containers: %s", rec.Body.String())
	}
	for _, p := range []string{"/firewall/rules", "/firewall/status", "/docker/images", "/redis/info", "/fail2ban/status", "/security/overview"} {
		if rec := get(base+p, userToken); rec.Code != http.StatusForbidden {
			t.Errorf("%s as regular user: got %d, want 403", p, rec.Code)
		}
	}
}
