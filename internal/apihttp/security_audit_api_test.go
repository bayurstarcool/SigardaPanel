package apihttp

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"sigardapanel/internal/auth"
	"sigardapanel/internal/store"
)

// A tenant admin creates sites only as itself or the users it created, inside
// that account's home (not /var/lib/sigardapanel, not another tenant's home).
func TestAdminSiteCreateConfinedToManagedAccounts(t *testing.T) {
	f := newAuthzFixture(t)
	admin, adminTok := f.user("tadmin", auth.RoleAdmin)
	other, _ := f.user("otheradmin", auth.RoleAdmin)
	mine := &store.User{Username: "mycust", Email: "mycust@example.com", PasswordHash: "x", Role: auth.RoleUser, CreatedBy: sql.NullInt64{Int64: admin.ID, Valid: true}}
	theirs := &store.User{Username: "theircust", Email: "theircust@example.com", PasswordHash: "x", Role: auth.RoleUser, CreatedBy: sql.NullInt64{Int64: other.ID, Valid: true}}
	for _, u := range []*store.User{mine, theirs} {
		if err := f.s.Users.Create(f.ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	body := func(extra map[string]any) map[string]any {
		b := map[string]any{"server_id": f.serverID, "name": "x", "domain": "x" + strconv.Itoa(len(extra)) + ".example.com", "runtime": "static"}
		for k, v := range extra {
			b[k] = v
		}
		return b
	}
	cases := []struct {
		name  string
		extra map[string]any
		want  int
	}{
		{"panel database dir", map[string]any{"root_path": "/var/lib/sigardapanel", "managed_by": "external"}, http.StatusForbidden},
		{"another admin's customer", map[string]any{"system_user": "theircust"}, http.StatusForbidden},
		{"another admin", map[string]any{"system_user": "otheradmin"}, http.StatusForbidden},
		{"service account", map[string]any{"system_user": "postgres"}, http.StatusBadRequest},
		{"own customer, other home", map[string]any{"system_user": "mycust", "root_path": "/home/tadmin/htdocs/y.example.com"}, http.StatusForbidden},
	}
	for _, tc := range cases {
		if rec := f.do("POST", "/api/v1/sites", body(tc.extra), adminTok); rec.Code != tc.want {
			t.Errorf("%s: %d %s", tc.name, rec.Code, rec.Body.String())
		}
	}
	rec := f.do("POST", "/api/v1/sites", map[string]any{"server_id": f.serverID, "name": "ok", "domain": "ok.example.com", "runtime": "static", "system_user": "mycust"}, adminTok)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), "/home/mycust/htdocs/ok.example.com") {
		t.Fatalf("own customer = %d %s", rec.Code, rec.Body.String())
	}

	// A user whose panel name is a service account owns no sites.
	_, ubuntuTok := f.user("ubuntu", auth.RoleUser)
	if rec := f.do("POST", "/api/v1/sites", map[string]any{"server_id": f.serverID, "name": "u", "domain": "u.example.com", "runtime": "static"}, ubuntuTok); rec.Code != http.StatusForbidden {
		t.Fatalf("service-account name = %d %s", rec.Code, rec.Body.String())
	}
}

// Sign-up derives a safe Linux user name instead of using the raw email.
func TestRegisterDerivesSafeUsername(t *testing.T) {
	t.Setenv("SIGARDAPANEL_ALLOW_REGISTRATION", "true")
	e, s, ctx := newTestAppWithAllMigrations(t)
	if rec := doRequest(t, e, "POST", "/api/v1/auth/register", map[string]any{"name": "x", "email": "ubuntu", "password": "Long-pass-123"}, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("non-email = %d %s", rec.Code, rec.Body.String())
	}
	rec := doRequest(t, e, "POST", "/api/v1/auth/register", map[string]any{"name": "x", "email": "Ubuntu@example.com", "password": "Long-pass-123"}, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("register = %d %s", rec.Code, rec.Body.String())
	}
	u, _ := s.Users.GetByEmail(ctx, "Ubuntu@example.com")
	if u == nil || u.Username != "ubuntu1" {
		t.Fatalf("username = %+v", u)
	}
}

// A transfer drops what the sender set up to run code (git deploy, webhook,
// app command) and every previous assignment; unknown emails get the same
// answer as any refusal.
func TestTransferClearsDeployConfigAndAssignments(t *testing.T) {
	f := newAuthzFixture(t)
	sender, senderTok := f.user("sender", auth.RoleUser)
	target, _ := f.user("receiver", auth.RoleUser)
	helper, _ := f.user("helper", auth.RoleUser)
	st := f.site(sender, "app", "node")
	f.assign(helper, st, "write")
	if _, err := f.s.DB().ExecContext(f.ctx, `UPDATE sites SET git_repo = 'https://x/r.git', webhook_secret = 'sek', deploy_key = 'k', app_command = 'npm start' WHERE id = ?`, st.ID); err != nil {
		t.Fatal(err)
	}
	rec := f.do("POST", "/api/v1/sites/"+strconv.FormatInt(st.ID, 10)+"/transfer", map[string]any{"new_owner_email": "nobody@example.com"}, senderTok)
	if rec.Code != http.StatusBadRequest || strings.Contains(rec.Body.String(), "not found") {
		t.Fatalf("unknown email = %d %s", rec.Code, rec.Body.String())
	}
	if err := f.s.Sites.Transfer(f.ctx, st.ID, target.ID); err != nil {
		t.Fatal(err)
	}
	var repo, secret, key sql.NullString
	var cmd string
	if err := f.s.DB().QueryRowContext(f.ctx, `SELECT git_repo, webhook_secret, deploy_key, app_command FROM sites WHERE id = ?`, st.ID).Scan(&repo, &secret, &key, &cmd); err != nil {
		t.Fatal(err)
	}
	if repo.Valid || secret.Valid || key.Valid || cmd != "" {
		t.Fatalf("deploy config kept: %v %v %v %q", repo, secret, key, cmd)
	}
	var n int
	f.s.DB().QueryRowContext(f.ctx, `SELECT COUNT(*) FROM user_sites WHERE site_id = ? AND user_id = ?`, st.ID, helper.ID).Scan(&n)
	if n != 0 {
		t.Fatal("previous assignment kept")
	}
}

// PHP_VALUE accepts only the allowlisted settings.
func TestPHPConfigAllowlist(t *testing.T) {
	f := newAuthzFixture(t)
	u, tok := f.user("phpu", auth.RoleUser)
	st := f.site(u, "php1", "php")
	path := "/api/v1/sites/" + strconv.FormatInt(st.ID, 10) + "/php/config"
	for _, bad := range []string{"auto_prepend_file = /home/x/htdocs/a.php", "open_basedir = /", "memory_limit = $uri", "extension = evil.so"} {
		if rec := f.do("PUT", path, map[string]any{"php_ini": bad}, tok); rec.Code != http.StatusBadRequest {
			t.Errorf("%q = %d %s", bad, rec.Code, rec.Body.String())
		}
	}
	rec := f.do("PUT", path, map[string]any{"php_ini": "memory_limit = 256M\nmax_execution_time = 60\ndate.timezone = Asia/Jakarta"}, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("allowed settings = %d %s", rec.Code, rec.Body.String())
	}
}

// Registration tokens and agent token rotation are the host operator's;
// re-binding a known agent address needs the current agent token.
func TestAgentRegistrationGuards(t *testing.T) {
	f := newAuthzFixture(t)
	_, adminTok := f.user("tadmin", auth.RoleAdmin)
	if rec := f.do("POST", "/api/v1/agents/tokens", map[string]any{}, adminTok); rec.Code != http.StatusForbidden {
		t.Fatalf("admin registration token = %d", rec.Code)
	}
	if rec := f.do("POST", "/api/v1/servers/"+strconv.FormatInt(f.serverID, 10)+"/token", map[string]any{}, adminTok); rec.Code != http.StatusForbidden {
		t.Fatalf("admin token rotation = %d", rec.Code)
	}

	mk := func() int64 {
		rt := &store.RegistrationToken{TokenHash: auth.HashToken("rt-" + strconv.FormatInt(time.Now().UnixNano(), 10)), ExpiresAt: time.Now().Add(time.Hour)}
		if err := f.s.RegistrationTokens.Create(f.ctx, rt); err != nil {
			t.Fatal(err)
		}
		return rt.ID
	}
	url := sql.NullString{String: "http://203.0.113.9:7710", Valid: true}
	first := &store.Server{Name: "a", Hostname: "a", AgentToken: "old-token", AgentURL: url, SSHPort: 22}
	if err := f.s.ConsumeRegistrationTokenAndCreateServer(f.ctx, mk(), first); err != nil {
		t.Fatal(err)
	}
	hijack := &store.Server{Name: "b", Hostname: "b", AgentToken: "attacker", AgentURL: url, SSHPort: 22}
	tok := mk()
	if err := f.s.ConsumeRegistrationTokenAndCreateServer(f.ctx, tok, hijack); err != store.ErrServerAlreadyRegistered {
		t.Fatalf("rebind without proof = %v", err)
	}
	reinstall := &store.Server{Name: "a", Hostname: "a", AgentToken: "new-token", AgentURL: url, SSHPort: 22, PreviousAgentToken: "old-token"}
	if err := f.s.ConsumeRegistrationTokenAndCreateServer(f.ctx, tok, reinstall); err != nil || !reinstall.Rebound || reinstall.ID != first.ID {
		t.Fatalf("re-install = %v rebound=%v id=%d", err, reinstall.Rebound, reinstall.ID)
	}
}

// Global alert channels and plans are not tenant admins' to change.
func TestGlobalAlertsAndPlansSuperAdminOnly(t *testing.T) {
	f := newAuthzFixture(t)
	_, adminTok := f.user("tadmin", auth.RoleAdmin)
	_, rootTok := f.user("root", auth.RoleSuperAdmin)
	for _, r := range []struct{ m, p string }{
		{"GET", "/api/v1/alert-channels"}, {"POST", "/api/v1/alert-channels"}, {"DELETE", "/api/v1/alert-channels/1"},
		{"GET", "/api/v1/alerts"}, {"POST", "/api/v1/plans"}, {"PATCH", "/api/v1/plans/1"}, {"DELETE", "/api/v1/plans/1"},
	} {
		if rec := f.do(r.m, r.p, map[string]any{}, adminTok); rec.Code != http.StatusForbidden {
			t.Errorf("admin %s %s = %d", r.m, r.p, rec.Code)
		}
	}
	if rec := f.do("GET", "/api/v1/alert-channels", nil, rootTok); rec.Code != http.StatusOK {
		t.Fatalf("super_admin alert channels = %d", rec.Code)
	}
}

// 2FA verify is an interactive, own-session action.
func TestTwoFAVerifyNeedsOwnSession(t *testing.T) {
	f := newAuthzFixture(t)
	root, _ := f.user("root", auth.RoleSuperAdmin)
	target, _ := f.user("t2fa", auth.RoleUser)
	imp, _ := auth.NewToken()
	f.s.Sessions.Create(f.ctx, &store.Session{UserID: target.ID, TokenHash: auth.HashToken(imp), ExpiresAt: time.Now().Add(time.Hour), ImpersonatedBy: sql.NullInt64{Int64: root.ID, Valid: true}})
	if rec := f.do("POST", "/api/v1/auth/2fa/verify", map[string]any{"code": "000000"}, imp); rec.Code != http.StatusForbidden {
		t.Fatalf("impersonated verify = %d %s", rec.Code, rec.Body.String())
	}
}
