package apihttp

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestMeReportsPasswordChangeRequired(t *testing.T) {
	e, s, ctx := newTestApp(t)
	token := newAdminToken(t, s, ctx) // created like install.sh/init: password never changed

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
