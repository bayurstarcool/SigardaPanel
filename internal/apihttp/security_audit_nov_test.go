package apihttp

import (
	"database/sql"
	"net/http"
	"strconv"
	"testing"
	"time"

	"sigardapanel/internal/auth"
	"sigardapanel/internal/store"
)

// A tenant must not register the server's own schemas (GRANT ALL on mysql.*
// is every account on the server).
func TestDatabaseCreateRefusesSystemSchemas(t *testing.T) {
	f := newAuthzFixture(t)
	u, tok := f.user("tenant", auth.RoleUser)
	st := f.site(u, "shop", "php")
	for _, name := range []string{"mysql", "MySQL", "sys", "information_schema", "performance_schema", "phpmyadmin"} {
		rec := f.do("POST", "/api/v1/sites/"+strconv.FormatInt(st.ID, 10)+"/databases", map[string]any{"name": name, "type": "mysql"}, tok)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("mysql %q = %d %s", name, rec.Code, rec.Body.String())
		}
	}
	for _, name := range []string{"postgres", "template1", "template0"} {
		rec := f.do("POST", "/api/v1/sites/"+strconv.FormatInt(st.ID, 10)+"/databases", map[string]any{"name": name, "type": "postgres"}, tok)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("postgres %q = %d %s", name, rec.Code, rec.Body.String())
		}
	}
}

// An impersonation ends as soon as the impersonator could no longer start it.
func TestImpersonationEndsWhenImpersonatorLosesAccess(t *testing.T) {
	f := newAuthzFixture(t)
	admin, _ := f.user("tadmin", auth.RoleAdmin)
	target, _ := f.user("tuser", auth.RoleUser)
	if _, err := f.s.DB().ExecContext(f.ctx, `UPDATE users SET created_by = ? WHERE id = ?`, admin.ID, target.ID); err != nil {
		t.Fatal(err)
	}
	imp, _ := auth.NewToken()
	if err := f.s.Sessions.Create(f.ctx, &store.Session{UserID: target.ID, TokenHash: auth.HashToken(imp), ExpiresAt: time.Now().Add(time.Hour), ImpersonatedBy: sql.NullInt64{Int64: admin.ID, Valid: true}}); err != nil {
		t.Fatal(err)
	}
	if rec := f.do("GET", "/api/v1/auth/me", nil, imp); rec.Code != http.StatusOK {
		t.Fatalf("live impersonation = %d %s", rec.Code, rec.Body.String())
	}
	// Demoted: the session dies and stays revoked.
	if _, err := f.s.DB().ExecContext(f.ctx, `UPDATE users SET role = 'user' WHERE id = ?`, admin.ID); err != nil {
		t.Fatal(err)
	}
	if rec := f.do("GET", "/api/v1/auth/me", nil, imp); rec.Code != http.StatusUnauthorized {
		t.Fatalf("after demotion = %d", rec.Code)
	}
	f.s.DB().ExecContext(f.ctx, `UPDATE users SET role = 'admin' WHERE id = ?`, admin.ID)
	if rec := f.do("GET", "/api/v1/auth/me", nil, imp); rec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked session revived = %d", rec.Code)
	}

	// Revoking all of the admin's sessions also ends impersonations it started.
	imp2, _ := auth.NewToken()
	f.s.Sessions.Create(f.ctx, &store.Session{UserID: target.ID, TokenHash: auth.HashToken(imp2), ExpiresAt: time.Now().Add(time.Hour), ImpersonatedBy: sql.NullInt64{Int64: admin.ID, Valid: true}})
	if err := f.s.Sessions.RevokeAllForUser(f.ctx, admin.ID, 0); err != nil {
		t.Fatal(err)
	}
	if rec := f.do("GET", "/api/v1/auth/me", nil, imp2); rec.Code != http.StatusUnauthorized {
		t.Fatalf("after admin reset = %d", rec.Code)
	}
}

// Deleting an admin who impersonated someone works (the FK used to block it
// after the owner/audit references were already cleared).
func TestDeleteAdminWhoImpersonated(t *testing.T) {
	f := newAuthzFixture(t)
	_, rootTok := f.user("root", auth.RoleSuperAdmin)
	admin, _ := f.user("gone", auth.RoleAdmin)
	target, _ := f.user("victim", auth.RoleUser)
	imp, _ := auth.NewToken()
	f.s.Sessions.Create(f.ctx, &store.Session{UserID: target.ID, TokenHash: auth.HashToken(imp), ExpiresAt: time.Now().Add(time.Hour), ImpersonatedBy: sql.NullInt64{Int64: admin.ID, Valid: true}})
	if rec := f.do("DELETE", "/api/v1/users/"+strconv.FormatInt(admin.ID, 10), nil, rootTok); rec.Code != http.StatusOK {
		t.Fatalf("delete admin = %d %s", rec.Code, rec.Body.String())
	}
	if u, _ := f.s.Users.GetByID(f.ctx, admin.ID); u != nil {
		t.Fatal("admin still exists")
	}
}

// Your own password changes only through Profile (current password needed).
func TestPatchOwnPasswordRefused(t *testing.T) {
	f := newAuthzFixture(t)
	root, rootTok := f.user("root", auth.RoleSuperAdmin)
	if rec := f.do("PATCH", "/api/v1/users/"+strconv.FormatInt(root.ID, 10), map[string]any{"password": "New-pass-12345"}, rootTok); rec.Code != http.StatusBadRequest {
		t.Fatalf("own password via PATCH = %d %s", rec.Code, rec.Body.String())
	}
}
