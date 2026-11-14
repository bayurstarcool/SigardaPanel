package store

import (
	"context"
	"testing"

	"sigardapanel/internal/db"
	"sigardapanel/internal/migrations"
	"sigardapanel/internal/rbac"
)

func TestRolesMigrationAndCustomRoles(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := migrations.Up(database, migrations.AllMigrations); err != nil {
		t.Fatal(err)
	}
	s := New(database)
	ctx := context.Background()

	roles, err := s.Roles.List(ctx)
	if err != nil || len(roles) != 3 {
		t.Fatalf("seeded roles = %v %v", roles, err)
	}
	for _, r := range roles {
		if !r.BuiltIn || len(r.Permissions) != len(rbac.BuiltinPermissions(r.Name)) {
			t.Errorf("built-in %s = %+v", r.Name, r)
		}
	}

	// A custom role, then a user with it.
	r := &Role{Name: "support", Description: "Helpdesk", Permissions: map[string]bool{rbac.SiteViewAll: true, rbac.UserImpersonate: true}}
	if err := s.Roles.Create(ctx, r); err != nil {
		t.Fatal(err)
	}
	if err := s.Roles.Create(ctx, &Role{Name: "admin"}); err == nil {
		t.Fatal("a custom role named like a built-in was stored")
	}
	if err := s.Roles.Create(ctx, &Role{Name: "bad", Permissions: map[string]bool{"no.such": true}}); err == nil {
		t.Fatal("unknown permission stored")
	}
	u := &User{Username: "helpdesk", Email: "h@example.test", PasswordHash: "Temp-pass-12345", Role: "support"}
	if err := s.Users.Create(ctx, u); err != nil {
		t.Fatalf("user with custom role: %v", err)
	}
	if err := s.Users.Create(ctx, &User{Username: "ghost", Email: "g@example.test", PasswordHash: "Temp-pass-12345", Role: "nosuchrole"}); err == nil {
		t.Fatal("user with an unknown role stored")
	}
	perms, _ := s.Roles.PermissionsOf(ctx, "support")
	if !perms[rbac.SiteViewAll] || !perms[rbac.UserImpersonate] || len(perms) != 2 {
		t.Fatalf("support permissions = %v", perms)
	}
	if err := s.Roles.Update(ctx, r.ID, "Helpdesk team", map[string]bool{rbac.SiteViewAll: true}); err != nil {
		t.Fatal(err)
	}
	if perms, _ = s.Roles.PermissionsOf(ctx, "support"); len(perms) != 1 {
		t.Fatalf("updated permissions = %v", perms)
	}
	if err := s.Roles.Delete(ctx, r.ID); err != ErrRoleInUse {
		t.Fatalf("delete role in use = %v", err)
	}
	admin, _ := s.Roles.GetByName(ctx, "admin")
	if err := s.Roles.Update(ctx, admin.ID, "x", nil); err == nil {
		t.Fatal("built-in role updated")
	}
}
