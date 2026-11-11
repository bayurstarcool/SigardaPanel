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
