package apihttp

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"sigardapanel/internal/auth"
	"sigardapanel/internal/store"
)

// newTempPasswordAdmin creates an admin whose password someone else chose
// (installer-generated / set by an administrator) and returns the user, a
// login session token and a personal API key of that user.
func newTempPasswordAdmin(t *testing.T, s *store.Store, ctx context.Context) (*store.User, string, string) {
	t.Helper()
	u := &store.User{Username: "admin", Email: "admin@example.com", PasswordHash: "password123",
		Role: auth.RoleAdmin, MustChangePassword: true}
	if err := s.Users.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	token, _ := auth.NewToken()
	if err := s.Sessions.Create(ctx, &store.Session{UserID: u.ID, TokenHash: auth.HashToken(token), ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	key, _ := auth.NewToken()
	if err := s.APIKeys.Create(ctx, &store.UserAPIKey{UserID: u.ID, Name: "k", KeyHash: auth.HashToken(key), Prefix: key[:8]}); err != nil {
		t.Fatal(err)
	}
	return u, token, key
}

func assertPasswordChangeRequired(t *testing.T, code int, body, what string) {
	t.Helper()
	if code != http.StatusForbidden || !strings.Contains(body, `"code":"password_change_required"`) {
		t.Fatalf("%s: expected 403 password_change_required, got %d %s", what, code, body)
	}
}

func TestPasswordChangeRequired_EnforcedOnAPI(t *testing.T) {
	e, s, ctx := newTestApp(t)
	u, token, key := newTempPasswordAdmin(t, s, ctx)

	// Everything except the password-change flow is refused.
	for _, r := range []struct{ method, path string }{
		{"GET", "/api/v1/servers"},
		{"GET", "/api/v1/sites"},
		{"GET", "/api/v1/api-keys"},
		{"POST", "/api/v1/auth/2fa/setup"},
		{"GET", "/api/v1/users/1/usage"},
	} {
		rec := doRequest(t, e, r.method, r.path, nil, token)
		assertPasswordChangeRequired(t, rec.Code, rec.Body.String(), r.method+" "+r.path)
	}

	// Allowed: read the account, 2FA status, and the password change itself.
	if rec := doRequest(t, e, "GET", "/api/v1/auth/me", nil, token); rec.Code != http.StatusOK {
		t.Fatalf("me: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doRequest(t, e, "GET", "/api/v1/auth/2fa/status", nil, token); rec.Code != http.StatusOK {
		t.Fatalf("2fa status: %d %s", rec.Code, rec.Body.String())
	}

	// API keys of the account are refused, even for /auth/me.
	for _, p := range []string{"/api/v1/servers", "/api/v1/auth/me"} {
		rec := doRequest(t, e, "GET", p, nil, key)
		assertPasswordChangeRequired(t, rec.Code, rec.Body.String(), "api key "+p)
	}

	rec := doRequest(t, e, "PUT", "/api/v1/auth/profile", map[string]string{
		"old_password": "password123", "new_password": "a-new-password",
	}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("change password: %d %s", rec.Code, rec.Body.String())
	}
	if got, _ := s.Users.GetByID(ctx, u.ID); got.PasswordChangeRequired() {
		t.Fatal("flag still set after the change")
	}

	// Afterwards the same session and key work normally.
	if rec := doRequest(t, e, "GET", "/api/v1/servers", nil, token); rec.Code != http.StatusOK {
		t.Fatalf("servers after change: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doRequest(t, e, "GET", "/api/v1/servers", nil, key); rec.Code != http.StatusOK {
		t.Fatalf("api key after change: %d %s", rec.Code, rec.Body.String())
	}
}

func TestPasswordChangeRequired_LogoutAllowed(t *testing.T) {
	e, s, ctx := newTestApp(t)
	_, token, _ := newTempPasswordAdmin(t, s, ctx)
	if rec := doRequest(t, e, "POST", "/api/v1/auth/logout", nil, token); rec.Code != http.StatusOK {
		t.Fatalf("logout: %d %s", rec.Code, rec.Body.String())
	}
}

func TestPasswordChangeRequired_ImpersonationExempt(t *testing.T) {
	e, s, ctx := newTestApp(t)
	u, _, _ := newTempPasswordAdmin(t, s, ctx)
	boss := &store.User{Username: "boss", Email: "boss@example.com", PasswordHash: "password123", Role: auth.RoleSuperAdmin}
	if err := s.Users.Create(ctx, boss); err != nil {
		t.Fatal(err)
	}
	token, _ := auth.NewToken()
	if err := s.Sessions.Create(ctx, &store.Session{UserID: u.ID, TokenHash: auth.HashToken(token),
		ExpiresAt: time.Now().Add(time.Hour), ImpersonatedBy: sql.NullInt64{Int64: boss.ID, Valid: true}}); err != nil {
		t.Fatal(err)
	}
	if rec := doRequest(t, e, "GET", "/api/v1/servers", nil, token); rec.Code != http.StatusOK {
		t.Fatalf("impersonation session: %d %s", rec.Code, rec.Body.String())
	}
}

func TestUsersCreatedByAdminMustChangePassword(t *testing.T) {
	e, s, ctx := newTestApp(t)
	admin := &store.User{Username: "boss", Email: "boss@example.com", PasswordHash: "password123", Role: auth.RoleSuperAdmin}
	if err := s.Users.Create(ctx, admin); err != nil {
		t.Fatal(err)
	}
	token, _ := auth.NewToken()
	_ = s.Sessions.Create(ctx, &store.Session{UserID: admin.ID, TokenHash: auth.HashToken(token), ExpiresAt: time.Now().Add(time.Hour)})
	rec := doRequest(t, e, "POST", "/api/v1/users", map[string]any{
		"username": "newbie", "email": "newbie@example.com", "password": "set-by-admin", "role": "user",
	}, token)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("create user: %d %s", rec.Code, rec.Body.String())
	}
	u, _ := s.Users.GetByUsername(ctx, "newbie")
	if u == nil || !u.PasswordChangeRequired() {
		t.Fatalf("an admin-created account must change its password: %+v", u)
	}
}

func TestMeReportsPasswordChangeRequired(t *testing.T) {
	e, s, ctx := newTestApp(t)
	_, token, _ := newTempPasswordAdmin(t, s, ctx) // like an installer-generated password

	get := func() bool {
		rec := doRequest(t, e, "GET", "/api/v1/auth/me", nil, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("me: %d %s", rec.Code, rec.Body.String())
		}
		var res struct {
			Data map[string]any `json:"data"`
		}
		_ = json.NewDecoder(rec.Body).Decode(&res)
		v, ok := res.Data["password_change_required"].(bool)
		if !ok {
			t.Fatalf("password_change_required missing: %v", res.Data)
		}
		return v
	}
	if !get() {
		t.Fatal("new account: expected password_change_required=true")
	}
	u, _ := s.Users.GetByUsername(ctx, "admin")
	if err := s.Users.UpdatePassword(ctx, u.ID, "irrelevant-hash"); err != nil {
		t.Fatal(err)
	}
	if get() {
		t.Fatal("after a password change: expected password_change_required=false")
	}
}

func TestSignupStatusIsPublicAndClosedByDefault(t *testing.T) {
	e, _, _ := newTestApp(t)
	rec := doRequest(t, e, "GET", "/api/v1/auth/signup", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var res struct {
		Data map[string]bool `json:"data"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&res)
	if res.Data["member_signup"] || res.Data["panel_signup"] {
		t.Fatalf("sign-up should be closed without member DB / opt-in: %v", res.Data)
	}
}

func getPublicConfig(t *testing.T, path string) map[string]bool {
	t.Helper()
	e, _, _ := newTestApp(t)
	rec := doRequest(t, e, "GET", path, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: expected 200, got %d", path, rec.Code)
	}
	var res struct {
		Data map[string]bool `json:"data"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&res)
	for _, k := range []string{"member_signup", "panel_signup", "landing_page"} {
		if _, ok := res.Data[k]; !ok {
			t.Fatalf("%s: %q missing: %v", path, k, res.Data)
		}
	}
	return res.Data
}

func TestPublicConfig_SelfHostedDefaults(t *testing.T) {
	t.Setenv("SIGARDAPANEL_LANDING_PAGE", "")
	t.Setenv("SIGARDAPANEL_ALLOW_MEMBER_SIGNUP", "")
	t.Setenv("SIGARDAPANEL_ALLOW_REGISTRATION", "")
	for _, p := range []string{"/api/v1/public/config", "/api/v1/auth/signup"} {
		d := getPublicConfig(t, p)
		if d["landing_page"] || d["member_signup"] || d["panel_signup"] {
			t.Fatalf("%s: self-hosted defaults must all be false: %v", p, d)
		}
	}
}

func TestPublicConfig_SaaSSwitches(t *testing.T) {
	t.Setenv("SIGARDAPANEL_LANDING_PAGE", "true")
	t.Setenv("SIGARDAPANEL_ALLOW_MEMBER_SIGNUP", "true")
	d := getPublicConfig(t, "/api/v1/public/config")
	if !d["landing_page"] {
		t.Fatalf("landing_page should follow SIGARDAPANEL_LANDING_PAGE: %v", d)
	}
	// The member portal needs its database; without one sign-up stays closed
	// even when allowed.
	if d["member_signup"] {
		t.Fatalf("member_signup without a member DB: %v", d)
	}
	if !publicConfigData(true)["member_signup"].(bool) {
		t.Fatal("member_signup should be open with a member DB and the flag set")
	}
	t.Setenv("SIGARDAPANEL_ALLOW_MEMBER_SIGNUP", "false")
	if publicConfigData(true)["member_signup"].(bool) {
		t.Fatal("member_signup must be closed when the flag is false")
	}
}
