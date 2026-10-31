package apihttp

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/auth"
	"sigardapanel/internal/store"
)

// Site permission levels (see AGENTS.md "User-Site Isolation Model").
// The owner of a site (sites.owner_id) and super_admin always hold every
// level; other users only hold what their user_sites assignment grants.
const (
	sitePermRead  = "read"
	sitePermWrite = "write"
	sitePermAdmin = "admin"
)

// sitePermissionRank maps a user_sites permission to a comparable rank.
func sitePermissionRank(perm string) int {
	switch perm {
	case sitePermRead:
		return 1
	case sitePermWrite:
		return 2
	case sitePermAdmin:
		return 3
	default:
		return 0
	}
}

// sessionUser returns the panel user behind the session, or nil for service
// tokens (UserID 0), missing sessions and deleted users.
func sessionUser(ctx context.Context, s *store.Store, sess *store.Session) *store.User {
	if sess == nil || sess.UserID == 0 {
		return nil
	}
	user, err := s.Users.GetByID(ctx, sess.UserID)
	if err != nil {
		return nil
	}
	return user
}

// isSessionSuperAdmin reports whether the session belongs to a super_admin.
func isSessionSuperAdmin(ctx context.Context, s *store.Store, sess *store.Session) bool {
	u := sessionUser(ctx, s, sess)
	return u != nil && u.Role == auth.RoleSuperAdmin
}

// isSessionAdmin reports whether the session belongs to an admin or super_admin.
func isSessionAdmin(ctx context.Context, s *store.Store, sess *store.Session) bool {
	u := sessionUser(ctx, s, sess)
	return u != nil && auth.HasRole(u.Role, auth.RoleAdmin)
}

// hasSitePermission reports whether the session holds at least the required
// permission level on site. It mirrors siteHandler.checkSitePermission so that
// handlers outside siteHandler share one model: super_admin and the owner have
// full access; everyone else needs a user_sites assignment of the given level.
// Service tokens have no user and therefore no site permission.
func hasSitePermission(ctx context.Context, s *store.Store, sess *store.Session, site *store.Site, required string) bool {
	if site == nil {
		return false
	}
	user := sessionUser(ctx, s, sess)
	if user == nil {
		return false
	}
	if user.Role == auth.RoleSuperAdmin {
		return true
	}
	if site.OwnerID.Valid && site.OwnerID.Int64 == user.ID {
		return true
	}
	need := sitePermissionRank(required)
	if need == 0 {
		return false
	}
	perm, err := s.UserSites.GetPermission(ctx, user.ID, site.ID)
	if err != nil {
		return false
	}
	return sitePermissionRank(perm) >= need
}

// isSiteOwnerOrSuperAdmin reports whether the session is the site's owner or a
// super_admin (the only principals allowed to manage a site's user assignments).
func isSiteOwnerOrSuperAdmin(ctx context.Context, s *store.Store, sess *store.Session, site *store.Site) bool {
	if site == nil {
		return false
	}
	user := sessionUser(ctx, s, sess)
	if user == nil {
		return false
	}
	if user.Role == auth.RoleSuperAdmin {
		return true
	}
	return site.OwnerID.Valid && site.OwnerID.Int64 == user.ID
}

// requireSitePermission returns nil when the session holds the required level
// on site. Callers that cannot even read the site get 404 (so the site's
// existence is not revealed, matching the rest of the API); callers that can
// read it but lack the level get 403.
func requireSitePermission(ctx context.Context, s *store.Store, sess *store.Session, site *store.Site, required string) error {
	if hasSitePermission(ctx, s, sess, site, required) {
		return nil
	}
	if required != sitePermRead && hasSitePermission(ctx, s, sess, site, sitePermRead) {
		return echo.NewHTTPError(http.StatusForbidden, "insufficient site permission")
	}
	return echo.NewHTTPError(http.StatusNotFound, "site not found")
}

// accessibleSiteIDs returns the IDs of every site the (non super_admin) user
// owns or is assigned to.
func accessibleSiteIDs(ctx context.Context, s *store.Store, userID int64) (map[int64]bool, error) {
	ids := make(map[int64]bool)
	owned, err := s.Sites.List(ctx, store.SiteListFilter{OwnerID: userID, Limit: 100000})
	if err != nil {
		return nil, err
	}
	for _, site := range owned {
		ids[site.ID] = true
	}
	assigned, err := s.UserSites.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, us := range assigned {
		ids[us.SiteID] = true
	}
	return ids, nil
}
