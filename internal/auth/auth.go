package auth

const (
	RoleSuperAdmin = "super_admin"
	RoleAdmin      = "admin"
	RoleUser       = "user"
)

// IsValidRole returns true if the role is one of the known roles.
func IsValidRole(role string) bool {
	return role == RoleSuperAdmin || role == RoleAdmin || role == RoleUser
}

// HasRole checks whether userRole meets the requiredRole.
// Hierarchy: super_admin > admin > user
// HasRole("admin", "user") == true  (admin inherits user)
// HasRole("user", "admin") == false
func HasRole(userRole, requiredRole string) bool {
	switch requiredRole {
	case RoleSuperAdmin:
		return userRole == RoleSuperAdmin
	case RoleAdmin:
		return userRole == RoleSuperAdmin || userRole == RoleAdmin
	case RoleUser:
		// Custom roles (internal/rbac) are at least the user tier: their
		// extra capabilities are checked by permission, never by rank.
		return userRole != "" && userRole != "service_token"
	default:
		return false
	}
}
