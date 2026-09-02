package auth

import "testing"

func TestIsValidRole(t *testing.T) {
	tests := []struct {
		role  string
		valid bool
	}{
		{RoleAdmin, true},
		{RoleUser, true},
		{"superuser", false},
		{"", false},
		{"ADMIN", false},
		{"User", false},
	}

	for _, tt := range tests {
		got := IsValidRole(tt.role)
		if got != tt.valid {
			t.Errorf("IsValidRole(%q) = %v, want %v", tt.role, got, tt.valid)
		}
	}
}

func TestHasRole(t *testing.T) {
	tests := []struct {
		userRole   string
		required   string
		hasAccess  bool
	}{
		{RoleAdmin, RoleAdmin, true},
		{RoleUser, RoleAdmin, false},
		{RoleAdmin, RoleUser, true},
		{RoleUser, RoleUser, true},
		{"", RoleUser, false},
		{RoleAdmin, "unknown", false},
		{RoleUser, "unknown", false},
		{"", RoleAdmin, false},
		{"", "", false},
	}

	for _, tt := range tests {
		got := HasRole(tt.userRole, tt.required)
		if got != tt.hasAccess {
			t.Errorf("HasRole(%q, %q) = %v, want %v", tt.userRole, tt.required, got, tt.hasAccess)
		}
	}
}