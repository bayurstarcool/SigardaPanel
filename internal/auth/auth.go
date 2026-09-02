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
		return userRole == RoleSuperAdmin || userRole == RoleAdmin || userRole == RoleUser
	default:
		return false
	}
}
