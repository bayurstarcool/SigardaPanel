package apihttp

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/auth"
	"sigardapanel/internal/jobmodel"
	"sigardapanel/internal/store"
)

// authzFixture is a router + store with helpers to create users, sites,
// assignments and tokens for authorization tests.
type authzFixture struct {
	t        *testing.T
	e        *echo.Echo
	s        *store.Store
	ctx      context.Context
	serverID int64
}

func newAuthzFixture(t *testing.T) *authzFixture {
	t.Helper()
	e, s, ctx := newTestAppWithAllMigrations(t)
	return &authzFixture{t: t, e: e, s: s, ctx: ctx, serverID: createServer(t, s, ctx)}
}

// user creates a user with the given role and returns it with a session token.
func (f *authzFixture) user(name, role string) (*store.User, string) {
	f.t.Helper()
	u := &store.User{Username: name, Email: name + "@example.com", PasswordHash: "password123", Role: role}
	if err := f.s.Users.Create(f.ctx, u); err != nil {
		f.t.Fatalf("create user %s: %v", name, err)
	}
	token, err := auth.NewToken()
	if err != nil {
		f.t.Fatalf("token: %v", err)
	}
	if err := f.s.Sessions.Create(f.ctx, &store.Session{UserID: u.ID, TokenHash: auth.HashToken(token), ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		f.t.Fatalf("session: %v", err)
	}
	return u, token
}

func (f *authzFixture) site(owner *store.User, name, runtime string) *store.Site {
	f.t.Helper()
	st := &store.Site{
		ServerID: f.serverID, Name: name, Domain: name + ".example.com", Runtime: runtime,
		RootPath: "/home/" + owner.Username + "/htdocs/" + name + ".example.com",
		OwnerID:  sql.NullInt64{Int64: owner.ID, Valid: true},
	}
	if runtime == "node" {
		st.AppPort = sql.NullInt64{Int64: 3100 + int64(len(name)), Valid: true}
	}
	if err := f.s.Sites.Create(f.ctx, st); err != nil {
		f.t.Fatalf("create site %s: %v", name, err)
	}
	return st
}

func (f *authzFixture) assign(u *store.User, st *store.Site, perm string) {
	f.t.Helper()
	if err := f.s.UserSites.Assign(f.ctx, u.ID, st.ID, perm); err != nil {
		f.t.Fatalf("assign: %v", err)
	}
}

func (f *authzFixture) serviceToken(scopes string) string {
	f.t.Helper()
	token, _ := auth.NewToken()
	if err := f.s.ServiceTokens.Create(f.ctx, &store.ServiceToken{Name: "tok-" + scopes, KeyHash: auth.HashToken(token), Scopes: scopes}); err != nil {
		f.t.Fatalf("service token: %v", err)
	}
	return token
}

func (f *authzFixture) do(method, path string, body any, token string) *httptest.ResponseRecorder {
	f.t.Helper()
	return doRequest(f.t, f.e, method, path, body, token)
}

type authzCase struct {
	name   string
	method string
	path   string
	body   any
	token  string
	want   int
}

func (f *authzFixture) run(cases []authzCase) {
	f.t.Helper()
	for _, tc := range cases {
		rec := f.do(tc.method, tc.path, tc.body, tc.token)
		if rec.Code != tc.want {
			f.t.Errorf("%s: %s %s got %d, want %d (%s)", tc.name, tc.method, tc.path, rec.Code, tc.want, strings.TrimSpace(rec.Body.String()))
		}
	}
}

func sitePath(st *store.Site, suffix string) string {
	return "/api/v1/sites/" + itoa(st.ID) + suffix
}

// ─── Bug 1: service tokens must not bypass user-management restrictions ──

func TestUserManagement_ServiceTokenCannotManageUsers(t *testing.T) {
	f := newAuthzFixture(t)
	root, rootTok := f.user("root", auth.RoleSuperAdmin)
	_, adminTok := f.user("adm", auth.RoleAdmin)
	svc := f.serviceToken("*")

	// The admin creates a user of their own (legitimate path).
	rec := f.do("POST", "/api/v1/users", map[string]any{"username": "child", "email": "child@example.com", "password": "password123"}, adminTok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("admin create user: %d %s", rec.Code, rec.Body.String())
	}
	child, _ := f.s.Users.GetByUsername(f.ctx, "child")

	f.run([]authzCase{
		{"svc token patches super admin password", "PATCH", "/api/v1/users/" + itoa(root.ID), map[string]any{"password": "pwned-password"}, svc, http.StatusForbidden},
		{"svc token lists users", "GET", "/api/v1/users", nil, svc, http.StatusForbidden},
		{"svc token creates admin", "POST", "/api/v1/users", map[string]any{"username": "x", "email": "x@example.com", "password": "password123", "role": "admin"}, svc, http.StatusForbidden},
		{"svc token deletes user", "DELETE", "/api/v1/users/" + itoa(child.ID), nil, svc, http.StatusForbidden},
		{"svc token assigns site", "POST", "/api/v1/users/" + itoa(child.ID) + "/sites", map[string]any{"site_id": 1}, svc, http.StatusForbidden},
		{"admin patches super admin", "PATCH", "/api/v1/users/" + itoa(root.ID), map[string]any{"password": "pwned-password"}, adminTok, http.StatusNotFound},
		{"admin patches own child", "PATCH", "/api/v1/users/" + itoa(child.ID), map[string]any{"password": "newpassword123"}, adminTok, http.StatusOK},
		{"super admin patches anyone", "PATCH", "/api/v1/users/" + itoa(child.ID), map[string]any{"max_sites": 3}, rootTok, http.StatusOK},
	})

	// The super admin's password must be unchanged.
	u, _ := f.s.Users.GetByID(f.ctx, root.ID)
	if ok, _ := auth.VerifyPassword(u.PasswordHash, "password123"); !ok {
		t.Fatal("super admin password was changed")
	}
}

// ─── Bug 2: only site owner / super_admin may manage assignments ──

func TestUserSiteAssignment_RequiresOwnerOrSuperAdmin(t *testing.T) {
	f := newAuthzFixture(t)
	root, rootTok := f.user("root", auth.RoleSuperAdmin)
	adm, admTok := f.user("adm", auth.RoleAdmin)
	other, _ := f.user("other", auth.RoleUser)

	rootSite := f.site(root, "rootsite", "static")
	admSite := f.site(adm, "admsite", "static")
	visible := f.site(root, "visible", "static")
	f.assign(adm, visible, "write")

	assign := func(u *store.User) string { return "/api/v1/users/" + itoa(u.ID) + "/sites" }
	one := func(u *store.User, st *store.Site) string { return assign(u) + "/" + itoa(st.ID) }

	f.run([]authzCase{
		{"admin grants self admin on foreign site", "POST", assign(adm), map[string]any{"site_id": rootSite.ID, "permission": "admin"}, admTok, http.StatusNotFound},
		{"write assignee grants self admin", "POST", assign(adm), map[string]any{"site_id": visible.ID, "permission": "admin"}, admTok, http.StatusForbidden},
		{"write assignee changes own permission", "PATCH", one(adm, visible), map[string]any{"permission": "admin"}, admTok, http.StatusForbidden},
		{"write assignee unassigns", "DELETE", one(adm, visible), nil, admTok, http.StatusForbidden},
		{"owner assigns user", "POST", assign(other), map[string]any{"site_id": admSite.ID, "permission": "read"}, admTok, http.StatusOK},
		{"owner updates permission", "PATCH", one(other, admSite), map[string]any{"permission": "write"}, admTok, http.StatusOK},
		{"owner unassigns", "DELETE", one(other, admSite), nil, admTok, http.StatusOK},
		{"super admin assigns anywhere", "POST", assign(other), map[string]any{"site_id": rootSite.ID, "permission": "read"}, rootTok, http.StatusOK},
	})

	if perm, _ := f.s.UserSites.GetPermission(f.ctx, adm.ID, visible.ID); perm != "write" {
		t.Fatalf("assignee permission changed to %q", perm)
	}
	if ok, _ := f.s.UserSites.HasAccess(f.ctx, adm.ID, rootSite.ID); ok {
		t.Fatal("admin gained access to a foreign site")
	}
}

// ─── Bug 3: mutations need write, backup restore/delete need admin ──

func TestSitePermission_WriteRequiredForMutations(t *testing.T) {
	f := newAuthzFixture(t)
	owner, ownerTok := f.user("owner", auth.RoleUser)
	reader, readTok := f.user("reader", auth.RoleUser)
	writer, writeTok := f.user("writer", auth.RoleUser)
	_, strangerTok := f.user("stranger", auth.RoleAdmin)

	php := f.site(owner, "phpsite", "php")
	app := f.site(owner, "appsite", "node")
	for _, st := range []*store.Site{php, app} {
		f.assign(reader, st, "read")
		f.assign(writer, st, "write")
	}

	phpBody := map[string]any{"php_ini": "memory_limit=256M"}
	ipBody := map[string]any{"ip": "10.0.0.1", "action": "deny"}
	f.run([]authzCase{
		{"reader views php config", "GET", sitePath(php, "/php/config"), nil, readTok, http.StatusOK},
		{"reader changes php config", "PUT", sitePath(php, "/php/config"), phpBody, readTok, http.StatusForbidden},
		{"stranger changes php config", "PUT", sitePath(php, "/php/config"), phpBody, strangerTok, http.StatusNotFound},
		{"writer changes php config", "PUT", sitePath(php, "/php/config"), phpBody, writeTok, http.StatusOK},
		{"reader views app status", "GET", sitePath(app, "/app/status"), nil, readTok, http.StatusOK},
		{"reader stops app", "POST", sitePath(app, "/app/stop"), nil, readTok, http.StatusForbidden},
		{"reader updates app config", "PUT", sitePath(app, "/app/config"), map[string]any{}, readTok, http.StatusForbidden},
		{"writer stops app", "POST", sitePath(app, "/app/stop"), nil, writeTok, http.StatusAccepted},
		{"reader views ssl", "GET", sitePath(php, "/ssl"), nil, readTok, http.StatusOK},
		{"reader issues ssl", "POST", sitePath(php, "/ssl/issue"), nil, readTok, http.StatusForbidden},
		{"reader renews ssl", "POST", sitePath(php, "/ssl/renew"), nil, readTok, http.StatusForbidden},
		{"writer issues ssl", "POST", sitePath(php, "/ssl/issue"), nil, writeTok, http.StatusAccepted},
		{"owner renews ssl", "POST", sitePath(php, "/ssl/renew"), nil, ownerTok, http.StatusAccepted},
		{"reader lists ip rules", "GET", sitePath(php, "/ip-restrictions"), nil, readTok, http.StatusOK},
		{"reader adds ip rule", "POST", sitePath(php, "/ip-restrictions"), ipBody, readTok, http.StatusForbidden},
		{"writer adds ip rule", "POST", sitePath(php, "/ip-restrictions"), ipBody, writeTok, http.StatusCreated},
		{"reader creates backup", "POST", sitePath(php, "/backups"), nil, readTok, http.StatusForbidden},
	})

	rules, _ := f.s.IPRestrictions.List(f.ctx, php.ID)
	if len(rules) != 1 {
		t.Fatalf("expected 1 ip rule, got %d", len(rules))
	}
	f.run([]authzCase{
		{"reader deletes ip rule", "DELETE", sitePath(php, "/ip-restrictions/"+itoa(rules[0].ID)), nil, readTok, http.StatusForbidden},
		{"writer deletes ip rule", "DELETE", sitePath(php, "/ip-restrictions/"+itoa(rules[0].ID)), nil, writeTok, http.StatusOK},
	})
	_ = writer
}

func TestBackupRestoreDelete_RequireOwnerOrAdminAssignment(t *testing.T) {
	f := newAuthzFixture(t)
	owner, ownerTok := f.user("owner", auth.RoleUser)
	writer, writeTok := f.user("writer", auth.RoleUser)
	siteAdmin, siteAdminTok := f.user("siteadmin", auth.RoleUser)
	_, strangerTok := f.user("stranger", auth.RoleAdmin)
	st := f.site(owner, "bk", "static")
	f.assign(writer, st, "write")
	f.assign(siteAdmin, st, "admin")

	completed := func() int64 {
		b := &store.Backup{SiteID: st.ID, BackupType: "files", Status: "pending", StorageType: "local"}
		if err := f.s.Backups.Create(f.ctx, b); err != nil {
			t.Fatalf("create backup: %v", err)
		}
		path := filepath.Join(t.TempDir(), "b.tar.gz")
		os.WriteFile(path, []byte("x"), 0600)
		if err := f.s.Backups.MarkCompleted(f.ctx, b.ID, 1, 1, path); err != nil {
			t.Fatalf("complete backup: %v", err)
		}
		return b.ID
	}
	b1 := completed()
	b2 := completed()
	bp := func(id int64, suffix string) string { return "/api/v1/backups/" + itoa(id) + suffix }

	f.run([]authzCase{
		{"stranger restores", "POST", bp(b1, "/restore"), nil, strangerTok, http.StatusNotFound},
		{"write assignee restores", "POST", bp(b1, "/restore"), nil, writeTok, http.StatusForbidden},
		{"write assignee deletes", "DELETE", bp(b1, ""), nil, writeTok, http.StatusForbidden},
		{"stranger deletes", "DELETE", bp(b1, ""), nil, strangerTok, http.StatusNotFound},
		{"owner restores", "POST", bp(b1, "/restore"), nil, ownerTok, http.StatusAccepted},
		{"admin assignee restores", "POST", bp(b2, "/restore"), nil, siteAdminTok, http.StatusAccepted},
		{"admin assignee deletes", "DELETE", bp(b2, ""), nil, siteAdminTok, http.StatusOK},
		{"owner deletes", "DELETE", bp(b1, ""), nil, ownerTok, http.StatusOK},
	})
}

// ─── Bug 4: job scoping, redaction and log append restrictions ──

func TestJobs_ScopedToAccessibleSites(t *testing.T) {
	f := newAuthzFixture(t)
	alice, aliceTok := f.user("alice", auth.RoleUser)
	bob, _ := f.user("bob", auth.RoleUser)
	reader, readTok := f.user("reader", auth.RoleUser)
	_, adminTok := f.user("adm", auth.RoleAdmin)
	_, rootTok := f.user("root", auth.RoleSuperAdmin)
	svc := f.serviceToken("*")

	aliceSite := f.site(alice, "alice", "static")
	bobSite := f.site(bob, "bob", "static")
	f.assign(reader, aliceSite, "read")

	mkJob := func(siteID int64, payload string) *jobmodel.Job {
		j := &jobmodel.Job{Type: jobmodel.TypeSiteDeploy, Status: jobmodel.StatusQueued, Payload: payload,
			ServerID: sql.NullInt64{Int64: f.serverID, Valid: true}}
		if siteID > 0 {
			j.SiteID = sql.NullInt64{Int64: siteID, Valid: true}
		}
		if err := f.s.Jobs.Create(f.ctx, j); err != nil {
			t.Fatalf("create job: %v", err)
		}
		return j
	}
	aliceJob := mkJob(aliceSite.ID, `{"git_repo":"https://alice:ghp_alicesecret@github.com/a/b.git","git_token":"tok-a"}`)
	bobJob := mkJob(bobSite.ID, `{"git_repo":"https://bob:ghp_bobsecret@github.com/b/c.git"}`)
	serverJob := mkJob(0, `{"action":"install"}`)

	listIDs := func(token string) (map[int64]bool, string) {
		rec := f.do("GET", "/api/v1/jobs", nil, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("list jobs: %d %s", rec.Code, rec.Body.String())
		}
		var resp struct {
			Data struct {
				Jobs []struct {
					ID int64 `json:"id"`
				} `json:"jobs"`
			} `json:"data"`
		}
		json.Unmarshal(rec.Body.Bytes(), &resp)
		ids := map[int64]bool{}
		for _, j := range resp.Data.Jobs {
			ids[j.ID] = true
		}
		return ids, rec.Body.String()
	}

	ids, body := listIDs(aliceTok)
	if !ids[aliceJob.ID] || ids[bobJob.ID] || ids[serverJob.ID] {
		t.Errorf("alice job list not scoped: %v", ids)
	}
	if strings.Contains(body, "ghp_alicesecret") || strings.Contains(body, "tok-a") {
		t.Errorf("credentials leaked in job list: %s", body)
	}
	if ids, _ := listIDs(adminTok); ids[aliceJob.ID] || ids[bobJob.ID] || !ids[serverJob.ID] {
		t.Errorf("non-super admin should only see server-level jobs here: %v", ids)
	}
	if ids, _ := listIDs(rootTok); !ids[aliceJob.ID] || !ids[bobJob.ID] || !ids[serverJob.ID] {
		t.Errorf("super admin should see all jobs: %v", ids)
	}

	jp := func(j *jobmodel.Job, suffix string) string { return "/api/v1/jobs/" + itoa(j.ID) + suffix }
	logLine := map[string]any{"line_number": 1, "message": "hi"}
	f.run([]authzCase{
		{"alice gets own job", "GET", jp(aliceJob, ""), nil, aliceTok, http.StatusOK},
		{"alice gets bob job", "GET", jp(bobJob, ""), nil, aliceTok, http.StatusNotFound},
		{"alice gets server job", "GET", jp(serverJob, ""), nil, aliceTok, http.StatusNotFound},
		{"alice reads bob logs", "GET", jp(bobJob, "/logs"), nil, aliceTok, http.StatusNotFound},
		{"alice reads own logs", "GET", jp(aliceJob, "/logs"), nil, aliceTok, http.StatusOK},
		{"alice appends log", "POST", jp(aliceJob, "/logs"), logLine, aliceTok, http.StatusForbidden},
		{"admin appends log to foreign site job", "POST", jp(aliceJob, "/logs"), logLine, adminTok, http.StatusNotFound},
		{"admin appends log to server job", "POST", jp(serverJob, "/logs"), logLine, adminTok, http.StatusCreated},
		{"service token appends log", "POST", jp(aliceJob, "/logs"), logLine, svc, http.StatusCreated},
		{"alice cancels bob job", "POST", jp(bobJob, "/cancel"), nil, aliceTok, http.StatusNotFound},
		{"read assignee cancels", "POST", jp(aliceJob, "/cancel"), nil, readTok, http.StatusForbidden},
		{"alice cancels own job", "POST", jp(aliceJob, "/cancel"), nil, aliceTok, http.StatusOK},
	})
}

func TestRedactSensitivePayload_URLCredentialsAndTokenKeys(t *testing.T) {
	in := `{"git_repo":"https://user:ghp_x@github.com/o/r.git","api_token":"abc","nested":{"smtp_password":"p"},"list":["http://u:p@h/x"],"basic_auth_enabled":true,"domain":"a.com"}`
	got := redactSensitivePayload(in)
	for _, leak := range []string{"ghp_x", `"abc"`, `"p"`, "u:p@"} {
		if strings.Contains(got, leak) {
			t.Errorf("leaked %q in %s", leak, got)
		}
	}
	for _, keep := range []string{"github.com/o/r.git", `"basic_auth_enabled":true`, `"domain":"a.com"`} {
		if !strings.Contains(got, keep) {
			t.Errorf("lost %q in %s", keep, got)
		}
	}
}

// ─── Bug 5: backup listing scope + legacy schedule authz/cron validation ──

func TestBackupList_ScopedForNonSuperAdmin(t *testing.T) {
	f := newAuthzFixture(t)
	alice, aliceTok := f.user("alice", auth.RoleUser)
	bob, _ := f.user("bob", auth.RoleAdmin)
	_, rootTok := f.user("root", auth.RoleSuperAdmin)
	a := f.site(alice, "alice", "static")
	b := f.site(bob, "bob", "static")
	for _, st := range []*store.Site{a, b} {
		if err := f.s.Backups.Create(f.ctx, &store.Backup{SiteID: st.ID, BackupType: "files", Status: "completed", StorageType: "local"}); err != nil {
			t.Fatal(err)
		}
	}
	count := func(token string) int {
		rec := f.do("GET", "/api/v1/backups", nil, token)
		var resp struct {
			Data struct {
				Count int `json:"count"`
			} `json:"data"`
		}
		json.Unmarshal(rec.Body.Bytes(), &resp)
		return resp.Data.Count
	}
	if n := count(aliceTok); n != 1 {
		t.Errorf("alice sees %d backups, want 1", n)
	}
	if n := count(rootTok); n != 2 {
		t.Errorf("super admin sees %d backups, want 2", n)
	}
}

func TestBackupSchedules_AuthzAndCronValidation(t *testing.T) {
	f := newAuthzFixture(t)
	owner, ownerTok := f.user("owner", auth.RoleUser)
	reader, readTok := f.user("reader", auth.RoleUser)
	writer, writeTok := f.user("writer", auth.RoleUser)
	_, strangerTok := f.user("stranger", auth.RoleAdmin)
	st := f.site(owner, "sched", "static")
	f.assign(reader, st, "read")
	f.assign(writer, st, "write")
	_ = writer

	create := sitePath(st, "/schedules")
	f.run([]authzCase{
		{"stranger lists", "GET", create, nil, strangerTok, http.StatusNotFound},
		{"reader lists", "GET", create, nil, readTok, http.StatusOK},
		{"stranger creates", "POST", create, map[string]any{"cron_expr": "0 3 * * *"}, strangerTok, http.StatusNotFound},
		{"reader creates", "POST", create, map[string]any{"cron_expr": "0 3 * * *"}, readTok, http.StatusForbidden},
		{"writer every minute", "POST", create, map[string]any{"cron_expr": "* * * * *"}, writeTok, http.StatusBadRequest},
		{"writer every 5 minutes", "POST", create, map[string]any{"cron_expr": "*/5 * * * *"}, writeTok, http.StatusBadRequest},
		{"writer garbage", "POST", create, map[string]any{"cron_expr": "nope"}, writeTok, http.StatusBadRequest},
		{"writer daily", "POST", create, map[string]any{"cron_expr": "0 3 * * *"}, writeTok, http.StatusCreated},
	})
	scheds, _ := f.s.BackupSchedules.ListBySite(f.ctx, st.ID)
	if len(scheds) != 1 {
		t.Fatalf("expected 1 schedule, got %d", len(scheds))
	}
	sp := "/api/v1/schedules/" + itoa(scheds[0].ID)
	f.run([]authzCase{
		{"stranger patches", "PATCH", sp, map[string]any{"enabled": false}, strangerTok, http.StatusNotFound},
		{"reader patches", "PATCH", sp, map[string]any{"enabled": false}, readTok, http.StatusForbidden},
		{"owner sets every minute", "PATCH", sp, map[string]any{"cron_expr": "* * * * *"}, ownerTok, http.StatusBadRequest},
		{"owner patches", "PATCH", sp, map[string]any{"cron_expr": "*/15 * * * *"}, ownerTok, http.StatusOK},
		{"stranger deletes", "DELETE", sp, nil, strangerTok, http.StatusNotFound},
		{"writer deletes", "DELETE", sp, nil, writeTok, http.StatusOK},
	})
}

func TestValidateBackupCron(t *testing.T) {
	for expr, ok := range map[string]bool{
		"0 3 * * *":        true,
		"*/15 * * * *":     true,
		"0,30 * * * *":     true,
		"15 2 * * 0":       true,
		"0 */6 1-15 * 1-5": true,
		"* * * * *":        false,
		"*/14 * * * *":     false,
		"0,10 * * * *":     false,
		"0-59/20 * * * *":  true,
		"0 3 * *":          false,
		"60 * * * *":       false,
		"a b c d e":        false,
		"0 3 * * */0":      false,
	} {
		if err := validateBackupCron(expr); (err == nil) != ok {
			t.Errorf("validateBackupCron(%q) err=%v, want ok=%v", expr, err, ok)
		}
	}
}

// ─── Bug 6: databases and logs use the site permission model ──

func TestDatabasesAndLogs_SitePermissionModel(t *testing.T) {
	f := newAuthzFixture(t)
	root, _ := f.user("root", auth.RoleSuperAdmin)
	_, admTok := f.user("adm", auth.RoleAdmin)
	reader, readTok := f.user("reader", auth.RoleUser)
	st := f.site(root, "rootsite", "php")
	f.assign(reader, st, "read")

	db := &store.Database{SiteID: st.ID, Name: "rootdb", Type: "mysql"}
	if err := f.s.Databases.Create(f.ctx, db); err != nil {
		t.Fatal(err)
	}
	dp := "/api/v1/databases/" + itoa(db.ID)
	f.run([]authzCase{
		{"non-super admin reads foreign pma creds", "GET", dp + "/pma-credentials", nil, admTok, http.StatusNotFound},
		{"non-super admin gets foreign db", "GET", dp, nil, admTok, http.StatusNotFound},
		{"non-super admin deletes foreign db", "DELETE", dp, nil, admTok, http.StatusNotFound},
		{"read assignee gets db", "GET", dp, nil, readTok, http.StatusOK},
		{"read assignee reads pma creds", "GET", dp + "/pma-credentials", nil, readTok, http.StatusForbidden},
		{"read assignee deletes db", "DELETE", dp, nil, readTok, http.StatusForbidden},
		{"non-super admin reads foreign logs", "GET", sitePath(st, "/logs"), nil, admTok, http.StatusNotFound},
		{"read assignee reads logs", "GET", sitePath(st, "/logs"), nil, readTok, http.StatusOK},
	})

	rec := f.do("GET", "/api/v1/databases", nil, admTok)
	if strings.Contains(rec.Body.String(), "rootdb") {
		t.Errorf("non-super admin sees foreign database in list: %s", rec.Body.String())
	}
	rec = f.do("GET", "/api/v1/databases", nil, readTok)
	if !strings.Contains(rec.Body.String(), "rootdb") {
		t.Errorf("read assignee should see the site's database: %s", rec.Body.String())
	}
}

// ─── Bug 7: admin role resolved from the session user on RequireAuth routes ──

func TestPlansAndOrders_AdminRoleFromSession(t *testing.T) {
	f := newAuthzFixture(t)
	_, admTok := f.user("adm", auth.RoleAdmin)
	u1, u1Tok := f.user("u1", auth.RoleUser)
	u2, _ := f.user("u2", auth.RoleUser)

	inactive := &store.Plan{Name: "Hidden", Slug: "hidden", BillingCycle: "monthly", Features: "{}", IsActive: false}
	if err := f.s.Plans.Create(f.ctx, inactive); err != nil {
		t.Fatal(err)
	}
	for _, u := range []*store.User{u1, u2} {
		if err := f.s.Orders.Create(f.ctx, &store.LicenseOrder{UserID: u.ID, PlanID: inactive.ID, Status: "pending"}); err != nil {
			t.Fatal(err)
		}
	}
	if !strings.Contains(f.do("GET", "/api/v1/plans", nil, admTok).Body.String(), "hidden") {
		t.Error("admin should see inactive plans")
	}
	if strings.Contains(f.do("GET", "/api/v1/plans", nil, u1Tok).Body.String(), "hidden") {
		t.Error("user should not see inactive plans")
	}
	orderCount := func(token string) int {
		var resp struct {
			Data struct {
				Count int `json:"count"`
			} `json:"data"`
		}
		json.Unmarshal(f.do("GET", "/api/v1/orders", nil, token).Body.Bytes(), &resp)
		return resp.Data.Count
	}
	if n := orderCount(admTok); n != 2 {
		t.Errorf("admin sees %d orders, want 2", n)
	}
	if n := orderCount(u1Tok); n != 1 {
		t.Errorf("user sees %d orders, want 1", n)
	}
}

// ─── Bug 8: metrics ingest restricted; range covers the whole window ──

func TestMetrics_IngestRestrictedAndRangeSpread(t *testing.T) {
	f := newAuthzFixture(t)
	_, userTok := f.user("u", auth.RoleUser)
	_, admTok := f.user("adm", auth.RoleAdmin)
	svc := f.serviceToken("*")
	mp := "/api/v1/servers/" + itoa(f.serverID) + "/metrics"
	f.run([]authzCase{
		{"user ingests", "POST", mp, map[string]any{"cpu_percent": 1.0}, userTok, http.StatusForbidden},
		{"admin ingests", "POST", mp, map[string]any{"cpu_percent": 1.0}, admTok, http.StatusCreated},
		{"service token ingests", "POST", mp, map[string]any{"cpu_percent": 1.0}, svc, http.StatusCreated},
	})
	if _, err := f.s.DB().ExecContext(f.ctx, `DELETE FROM server_metrics`); err != nil {
		t.Fatal(err)
	}

	// One sample per minute over the last 24h (1440 rows).
	now := time.Now().UTC()
	for i := 0; i < 1440; i++ {
		ts := now.Add(-time.Duration(1440-i) * time.Minute).Add(30 * time.Second)
		if _, err := f.s.DB().ExecContext(f.ctx,
			`INSERT INTO server_metrics (server_id, collected_at, cpu_percent) VALUES (?, ?, ?)`,
			f.serverID, ts.Format("2006-01-02 15:04:05"), float64(i%100)); err != nil {
			t.Fatal(err)
		}
	}
	rec := f.do("GET", mp+"?range=24h", nil, admTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("range: %d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			Metrics []struct {
				CollectedAt time.Time `json:"collected_at"`
			} `json:"metrics"`
		} `json:"data"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	m := resp.Data.Metrics
	if len(m) == 0 || len(m) > metricRangeMaxPoints {
		t.Fatalf("got %d points, want 1..%d", len(m), metricRangeMaxPoints)
	}
	if first := m[0].CollectedAt; now.Sub(first) < 23*time.Hour {
		t.Errorf("oldest point %v is not near the start of the 24h range", first)
	}
	if last := m[len(m)-1].CollectedAt; now.Sub(last) > 5*time.Minute {
		t.Errorf("newest point %v is not near now", last)
	}
	for i := 1; i < len(m); i++ {
		if m[i].CollectedAt.Before(m[i-1].CollectedAt) {
			t.Fatalf("points not ordered oldest→newest at %d", i)
		}
	}

	// Retention: pruning older than 12h removes roughly half.
	deleted, err := f.s.PruneMetrics(f.ctx, 12*time.Hour)
	if err != nil || deleted < 700 || deleted > 740 {
		t.Errorf("PruneMetrics deleted %d (err %v), want ~720", deleted, err)
	}
}

// ─── Bug 9: child ids must belong to the site in the URL ──

func TestSiteChildResources_ScopedToSite(t *testing.T) {
	f := newAuthzFixture(t)
	a, aTok := f.user("adma", auth.RoleAdmin)
	b, bTok := f.user("admb", auth.RoleAdmin)
	siteA := f.site(a, "sitea", "static")
	siteB := f.site(b, "siteb", "static")

	rule := &store.IPRestriction{SiteID: siteB.ID, IP: "1.2.3.4", Action: "deny"}
	if err := f.s.IPRestrictions.Create(f.ctx, rule); err != nil {
		t.Fatal(err)
	}
	cron := &store.SiteCronJob{SiteID: siteB.ID, Schedule: "0 * * * *", Command: "echo b"}
	if err := f.s.Cron.Create(f.ctx, cron); err != nil {
		t.Fatal(err)
	}
	ssh := &store.SiteSSHUser{SiteID: siteB.ID, Username: "bssh", SSHPrivateKey: "PRIVATE-KEY-B", SSHPublicKey: "ssh-ed25519 AAA", HomeDir: "/nonexistent/bssh"}
	if err := f.s.SSH.Create(f.ctx, ssh); err != nil {
		t.Fatal(err)
	}

	f.run([]authzCase{
		{"delete B's ip rule via A", "DELETE", sitePath(siteA, "/ip-restrictions/"+itoa(rule.ID)), nil, aTok, http.StatusNotFound},
		{"delete B's cron via A", "DELETE", sitePath(siteA, "/cron/"+itoa(cron.ID)), nil, aTok, http.StatusNotFound},
		{"B's ssh key via A", "GET", sitePath(siteA, "/ssh-ftp/"+itoa(ssh.ID)+"/key"), nil, aTok, http.StatusNotFound},
		{"B's ssh key via B as A", "GET", sitePath(siteB, "/ssh-ftp/"+itoa(ssh.ID)+"/key"), nil, aTok, http.StatusNotFound},
		{"delete B's ssh via A", "DELETE", sitePath(siteA, "/ssh-ftp/"+itoa(ssh.ID)), nil, aTok, http.StatusNotFound},
		{"list B's ssh users as A", "GET", sitePath(siteB, "/ssh-ftp"), nil, aTok, http.StatusNotFound},
		{"create ssh user on B as A", "POST", sitePath(siteB, "/ssh-ftp"), map[string]any{"username": "evil"}, aTok, http.StatusNotFound},
		{"owner downloads own key", "GET", sitePath(siteB, "/ssh-ftp/"+itoa(ssh.ID)+"/key"), nil, bTok, http.StatusOK},
	})

	if rules, _ := f.s.IPRestrictions.List(f.ctx, siteB.ID); len(rules) != 1 {
		t.Error("B's ip rule was deleted through A")
	}
	if crons, _ := f.s.Cron.ListBySiteID(f.ctx, siteB.ID); len(crons) != 1 {
		t.Error("B's cron was deleted through A")
	}
	if u, _ := f.s.SSH.GetByID(f.ctx, ssh.ID); u == nil {
		t.Error("B's ssh user was deleted through A")
	}
	rec := f.do("GET", sitePath(siteB, "/ssh-ftp"), nil, bTok)
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "PRIVATE-KEY-B") {
		t.Errorf("ssh listing must not include private keys: %d %s", rec.Code, rec.Body.String())
	}
	f.run([]authzCase{
		{"owner deletes own cron", "DELETE", sitePath(siteB, "/cron/"+itoa(cron.ID)), nil, bTok, http.StatusOK},
		{"owner deletes own ip rule", "DELETE", sitePath(siteB, "/ip-restrictions/"+itoa(rule.ID)), nil, bTok, http.StatusOK},
	})
}

// ─── Bug 10: database server endpoints are super_admin only ──

func TestDatabaseServer_SuperAdminOnly(t *testing.T) {
	f := newAuthzFixture(t)
	_, admTok := f.user("adm", auth.RoleAdmin)
	_, rootTok := f.user("root", auth.RoleSuperAdmin)
	svc := f.serviceToken("*")
	f.run([]authzCase{
		{"admin views db server", "GET", "/api/v1/database-server", nil, admTok, http.StatusForbidden},
		{"admin imports", "POST", "/api/v1/database-server/import", nil, admTok, http.StatusForbidden},
		{"service token imports", "POST", "/api/v1/database-server/import", nil, svc, http.StatusForbidden},
		{"super admin views db server", "GET", "/api/v1/database-server", nil, rootTok, http.StatusOK},
		// No server configured in tests: reaches the handler and reports it.
		{"super admin imports", "POST", "/api/v1/database-server/import", nil, rootTok, http.StatusNotFound},
	})
}

// ─── Bug 11: file create errors and notification ownership ──

func TestFileManagerCreate_ReportsErrors(t *testing.T) {
	f := newAuthzFixture(t)
	owner, tok := f.user("owner", auth.RoleUser)
	st := f.site(owner, "files", "static")
	root := t.TempDir()
	if _, err := f.s.DB().ExecContext(f.ctx, `UPDATE sites SET root_path = ? WHERE id = ?`, root, st.ID); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, "afile"), []byte("keep"), 0644)

	create := sitePath(st, "/files/create")
	f.run([]authzCase{
		{"dir over an existing file", "POST", create, map[string]any{"path": "afile", "is_dir": true}, tok, http.StatusInternalServerError},
		{"existing file", "POST", create, map[string]any{"path": "afile"}, tok, http.StatusConflict},
		{"new file", "POST", create, map[string]any{"path": "new/ok.txt"}, tok, http.StatusCreated},
		{"new dir", "POST", create, map[string]any{"path": "newdir", "is_dir": true}, tok, http.StatusCreated},
	})
	if b, _ := os.ReadFile(filepath.Join(root, "afile")); string(b) != "keep" {
		t.Errorf("existing file was truncated: %q", b)
	}
	if _, err := os.Stat(filepath.Join(root, "new", "ok.txt")); err != nil {
		t.Errorf("new file not created: %v", err)
	}
}

func TestNotifications_MarkReadDeleteOwnOnly(t *testing.T) {
	f := newAuthzFixture(t)
	alice, aliceTok := f.user("alice", auth.RoleUser)
	_, bobTok := f.user("bob", auth.RoleUser)
	n := &store.Notification{UserID: alice.ID, Type: "t", Title: "t", Message: "m", Severity: "info"}
	if err := f.s.Notifications.Create(f.ctx, n); err != nil {
		t.Fatal(err)
	}
	np := "/api/v1/notifications/" + itoa(n.ID)

	f.do("POST", np+"/read", nil, bobTok)
	f.do("DELETE", np, nil, bobTok)
	got, _ := f.s.Notifications.GetByID(f.ctx, n.ID)
	if got == nil || got.Read {
		t.Fatalf("bob modified alice's notification: %+v", got)
	}

	if rec := f.do("POST", np+"/read", nil, aliceTok); rec.Code != http.StatusOK {
		t.Fatalf("alice mark read: %d", rec.Code)
	}
	if got, _ := f.s.Notifications.GetByID(f.ctx, n.ID); got == nil || !got.Read {
		t.Fatal("alice could not mark her notification read")
	}
	f.do("DELETE", np, nil, aliceTok)
	if got, _ := f.s.Notifications.GetByID(f.ctx, n.ID); got != nil {
		t.Fatal("alice could not delete her notification")
	}
}
