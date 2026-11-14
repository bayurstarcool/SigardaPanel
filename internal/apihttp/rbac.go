package apihttp

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v5"
	"sigardapanel/internal/auth"
	"sigardapanel/internal/rbac"
	"sigardapanel/internal/store"
)

// rolePermissions returns the permission set of a role name: built-in roles
// from internal/rbac, custom roles from the roles table (nil when unknown).
func rolePermissions(ctx context.Context, s *store.Store, role string) map[string]bool {
	if p := rbac.BuiltinPermissions(role); p != nil {
		return p
	}
	if s == nil || s.Roles == nil {
		return nil
	}
	p, err := s.Roles.PermissionsOf(ctx, role)
	if err != nil {
		return nil
	}
	return p
}

// userCan reports whether the user's role holds perm.
func userCan(ctx context.Context, s *store.Store, u *store.User, perm string) bool {
	if u == nil {
		return false
	}
	return rolePermissions(ctx, s, u.Role)[perm]
}

// sessionCan reports whether the session's user holds perm (service tokens
// and missing users never do here; routes check tokens in RequirePermission).
func sessionCan(ctx context.Context, s *store.Store, sess *store.Session, perm string) bool {
	return userCan(ctx, s, sessionUser(ctx, s, sess), perm)
}

// RequirePermission guards a route with one permission from internal/rbac.
// Signed-in users need it in their role's permission set. Service tokens keep
// their scope model: a token passes when its scope covers the lowest built-in
// role holding the permission, and never for super_admin-only permissions.
// The caller's role is put in the context (roleKey) for handlers.
func RequirePermission(s *store.Store, perm string) echo.MiddlewareFunc {
	p, ok := rbac.Get(perm)
	if !ok {
		panic("RequirePermission: unknown permission " + perm)
	}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			sess, ok := c.Get(sessionKey).(*store.Session)
			if !ok {
				return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
			}
			if sess.UserID == 0 {
				scopes, _ := c.Get(serviceTokenScopesKey).(string)
				if scopes == "" {
					return echo.NewHTTPError(http.StatusForbidden, "service token has no scopes")
				}
				if p.MinRole == auth.RoleSuperAdmin || !serviceTokenHasScope(scopes, p.MinRole) {
					return echo.NewHTTPError(http.StatusForbidden, "service token lacks required scope")
				}
				c.Set(roleKey, "service_token")
				return next(c)
			}
			user, err := s.Users.GetByID(c.Request().Context(), sess.UserID)
			if err != nil || user == nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "user not found")
			}
			if !userCan(c.Request().Context(), s, user, perm) {
				return echo.NewHTTPError(http.StatusForbidden, "insufficient permissions")
			}
			c.Set(roleKey, user.Role)
			return next(c)
		}
	}
}

// assignableRole reports whether the caller may give role to a user: the
// role exists, its permissions are a subset of the caller's, and without
// user.manage_all (super_admin) it holds neither user management (an admin
// cannot create admins) nor dangerous permissions.
func assignableRole(ctx context.Context, s *store.Store, caller *store.User, role string) bool {
	if caller == nil {
		return false
	}
	target := rolePermissions(ctx, s, role)
	if target == nil {
		return false
	}
	mine := rolePermissions(ctx, s, caller.Role)
	if mine[rbac.UserManageAll] {
		return rbac.Subset(target, mine)
	}
	if target[rbac.UserManage] || target[rbac.UserManageAll] || rbac.HasDangerous(target) {
		return false
	}
	return rbac.Subset(target, mine)
}

// canImpersonate applies the impersonation rules: user.impersonate is needed;
// user.manage_all (super_admin) may impersonate anyone; user.impersonate_any
// any user whose permissions the caller holds; otherwise only users the caller
// created whose role it could assign (an admin: its "user" accounts).
func canImpersonate(ctx context.Context, s *store.Store, caller, target *store.User) bool {
	if caller == nil || target == nil {
		return false
	}
	mine := rolePermissions(ctx, s, caller.Role)
	if !mine[rbac.UserImpersonate] {
		return false
	}
	theirs := rolePermissions(ctx, s, target.Role)
	if theirs == nil {
		return false
	}
	// Even user.manage_all only reaches accounts holding no more than the
	// caller (super_admin holds everything, so it reaches anyone).
	if mine[rbac.UserManageAll] {
		return rbac.Subset(theirs, mine)
	}
	if mine[rbac.UserImpersonateAny] && rbac.Subset(theirs, mine) && !theirs[rbac.UserManageAll] {
		return true
	}
	return target.CreatedBy.Valid && target.CreatedBy.Int64 == caller.ID && assignableRole(ctx, s, caller, target.Role)
}
