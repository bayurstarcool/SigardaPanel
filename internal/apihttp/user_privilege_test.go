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

func sessionFor(t *testing.T, f *authzFixture, u *store.User) string {
	t.Helper()
	tok, _ := auth.NewToken()
	if err := f.s.Sessions.Create(f.ctx, &store.Session{UserID: u.ID, TokenHash: auth.HashToken(tok), ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	return tok
}

func TestCreatorAdminCannotTakeOverPromotedUser(t *testing.T) {
	f := newAuthzFixture(t)
	admin, adminTok := f.user("adm", auth.RoleAdmin)
	bob, _ := f.user("bob", auth.RoleUser)
	f.s.DB().Exec(`UPDATE users SET created_by = ? WHERE id = ?`, admin.ID, bob.ID)

	// While bob is a plain user his creator may manage him.
	if rec := doRequest(t, f.e, http.MethodPatch, "/api/v1/users/"+strconv.FormatInt(bob.ID, 10), map[string]any{"max_sites": 3}, adminTok); rec.Code != http.StatusOK {
		t.Fatalf("manage own user = %d %s", rec.Code, rec.Body.String())
	}
	f.s.Users.UpdateRole(f.ctx, bob.ID, auth.RoleSuperAdmin)
	if rec := doRequest(t, f.e, http.MethodPatch, "/api/v1/users/"+strconv.FormatInt(bob.ID, 10), map[string]any{"password": "takeover123"}, adminTok); rec.Code != http.StatusForbidden {
		t.Fatalf("password reset of a promoted user = %d %s", rec.Code, rec.Body.String())
	}
	if rec := doRequest(t, f.e, http.MethodDelete, "/api/v1/users/"+strconv.FormatInt(bob.ID, 10), nil, adminTok); rec.Code != http.StatusForbidden {
		t.Fatalf("delete of a promoted user = %d", rec.Code)
	}
}

func TestPasswordChangesRevokeSessions(t *testing.T) {
	f := newAuthzFixture(t)
	root, rootTok := f.user("root", auth.RoleSuperAdmin)
	alice, aliceTok := f.user("alice", auth.RoleUser)
	aliceOther := sessionFor(t, f, alice)
	hash, _ := auth.HashPassword("oldpassword1")
	f.s.Users.UpdatePassword(f.ctx, alice.ID, hash)

	// Alice changes her own password: her other session ends, this one stays.
	rec := doRequest(t, f.e, http.MethodPut, "/api/v1/auth/profile", map[string]any{"old_password": "oldpassword1", "new_password": "newpassword2"}, aliceTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("own change = %d %s", rec.Code, rec.Body.String())
	}
	if rec := doRequest(t, f.e, http.MethodGet, "/api/v1/auth/me", nil, aliceOther); rec.Code != http.StatusUnauthorized {
		t.Fatalf("other session after own password change = %d", rec.Code)
	}
	if rec := doRequest(t, f.e, http.MethodGet, "/api/v1/auth/me", nil, aliceTok); rec.Code != http.StatusOK {
		t.Fatalf("current session after own password change = %d", rec.Code)
	}

	// An admin reset ends all of her sessions and forces a new password.
	rec = doRequest(t, f.e, http.MethodPatch, "/api/v1/users/"+strconv.FormatInt(alice.ID, 10), map[string]any{"password": "chosenbyadmin1"}, rootTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin reset = %d %s", rec.Code, rec.Body.String())
	}
	if rec := doRequest(t, f.e, http.MethodGet, "/api/v1/auth/me", nil, aliceTok); rec.Code != http.StatusUnauthorized {
		t.Fatalf("session after admin reset = %d", rec.Code)
	}
	u, _ := f.s.Users.GetByID(f.ctx, alice.ID)
	var changed sql.NullTime
	f.s.DB().QueryRow(`SELECT password_changed_at FROM users WHERE id = ?`, u.ID).Scan(&changed)
	if changed.Valid {
		t.Fatal("an admin-chosen password must require a change at the next login")
	}
	_ = root
}

func TestAdminScopedReads(t *testing.T) {
	f := newAuthzFixture(t)
	root, rootTok := f.user("root", auth.RoleSuperAdmin)
	admin, adminTok := f.user("adm", auth.RoleAdmin)
	f.s.DB().Exec(`INSERT INTO audit_logs (user_id, action, resource) VALUES (?, 'command.exec', 'server'), (?, 'site.update', 'site')`, root.ID, admin.ID)

	rec := doRequest(t, f.e, http.MethodGet, "/api/v1/audit-logs?user_id="+strconv.FormatInt(root.ID, 10), nil, adminTok)
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "command.exec") || !strings.Contains(rec.Body.String(), "site.update") {
		t.Fatalf("admin audit view = %d %s", rec.Code, rec.Body.String())
	}
	if rec := doRequest(t, f.e, http.MethodGet, "/api/v1/audit-logs", nil, rootTok); !strings.Contains(rec.Body.String(), "command.exec") {
		t.Fatalf("super_admin audit view lacks entries: %s", rec.Body.String())
	}
	if rec := doRequest(t, f.e, http.MethodGet, "/api/v1/users/"+strconv.FormatInt(root.ID, 10)+"/sites", nil, adminTok); rec.Code != http.StatusNotFound {
		t.Fatalf("admin listing another tenant's sites = %d", rec.Code)
	}
	if rec := doRequest(t, f.e, http.MethodGet, "/api/v1/service-tokens", nil, adminTok); rec.Code != http.StatusForbidden {
		t.Fatalf("admin service tokens = %d", rec.Code)
	}
}
