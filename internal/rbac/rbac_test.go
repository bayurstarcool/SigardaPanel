package rbac

import (
	"regexp"
	"testing"
)

func TestRegistryIsWellFormed(t *testing.T) {
	name := regexp.MustCompile(`^[a-z_]+(\.[a-z_]+)+$`)
	seen := map[string]bool{}
	for _, p := range All() {
		if !name.MatchString(p.Name) {
			t.Errorf("bad permission name %q", p.Name)
		}
		if seen[p.Name] {
			t.Errorf("duplicate permission %q", p.Name)
		}
		seen[p.Name] = true
		if p.Category == "" || p.Description == "" {
			t.Errorf("%s: category and description required", p.Name)
		}
		if !IsBuiltinRole(p.MinRole) {
			t.Errorf("%s: MinRole %q is not a built-in role", p.Name, p.MinRole)
		}
		// Dangerous permissions never reach tenants by default.
		if p.Dangerous && p.MinRole != RoleSuperAdmin {
			t.Errorf("%s: dangerous permissions must be super_admin-only by default", p.Name)
		}
	}
}

func TestBuiltinRolesNest(t *testing.T) {
	super, admin, user := BuiltinPermissions(RoleSuperAdmin), BuiltinPermissions(RoleAdmin), BuiltinPermissions(RoleUser)
	if len(super) != len(All()) {
		t.Fatalf("super_admin has %d of %d permissions", len(super), len(All()))
	}
	if !Subset(user, admin) || !Subset(admin, super) {
		t.Fatal("built-in roles must nest: user ⊆ admin ⊆ super_admin")
	}
	if HasDangerous(admin) || HasDangerous(user) {
		t.Fatal("admin and user must not hold dangerous permissions")
	}
	if BuiltinPermissions("support") != nil {
		t.Fatal("custom names have no built-in permissions")
	}
	// Exactly the admin-level capabilities of today.
	want := []string{AuditView, BackupBulk, BackupProviderView, DockerControl, JobLogsWrite, JobViewServer,
		MetricsIngest, QuotaUnlimited, SettingsView, SiteChooseOwner, SiteCron, SiteSuspend, UserImpersonate, UserManage}
	if got := Sorted(admin); len(got) != len(want) {
		t.Fatalf("admin permissions = %v, want %v", got, want)
	}
	for _, p := range want {
		if !admin[p] {
			t.Errorf("admin lacks %s", p)
		}
	}
	if len(user) != 0 {
		t.Fatalf("user has extra permissions %v (its capabilities are ownership-scoped)", Sorted(user))
	}
}
