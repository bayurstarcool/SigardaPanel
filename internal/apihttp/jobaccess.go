package apihttp

import (
	"github.com/labstack/echo/v4"
	"sigardapanel/internal/auth"
	"sigardapanel/internal/jobmodel"
	"sigardapanel/internal/store"
)

// jobVisibility describes which jobs a caller may see.
type jobVisibility struct {
	all   bool           // super_admin: every job
	scope store.JobScope // otherwise: jobs of accessible sites (+ server jobs for admins)
}

// isAdminServiceToken reports whether the request is authenticated with a
// service token carrying admin-level scope.
func isAdminServiceToken(c echo.Context) bool {
	sess := sessionFromEcho(c)
	if sess == nil || sess.UserID != 0 {
		return false
	}
	scopes, _ := c.Get(serviceTokenScopesKey).(string)
	return scopes != "" && serviceTokenHasScope(scopes, auth.RoleAdmin)
}

// jobVisibilityFor computes the job visibility of the current caller.
// Jobs not tied to a site (server-level jobs) are admin-only; service tokens
// have no site assignments, so admin-scoped tokens only see server-level jobs.
func jobVisibilityFor(c echo.Context, s *store.Store) jobVisibility {
	ctx := c.Request().Context()
	sess := sessionFromEcho(c)
	if sess != nil && sess.UserID == 0 {
		return jobVisibility{scope: store.JobScope{IncludeServerJobs: isAdminServiceToken(c)}}
	}
	user := sessionUser(ctx, s, sess)
	if user == nil {
		return jobVisibility{}
	}
	if user.Role == auth.RoleSuperAdmin {
		return jobVisibility{all: true}
	}
	return jobVisibility{scope: store.JobScope{
		UserID:            user.ID,
		IncludeServerJobs: auth.HasRole(user.Role, auth.RoleAdmin),
	}}
}

// canSeeJob reports whether the caller holds the required site permission on
// the job's site (super_admin always; server-level jobs: admins and
// admin-scoped service tokens only).
func canSeeJob(c echo.Context, s *store.Store, job *jobmodel.Job, required string) bool {
	ctx := c.Request().Context()
	sess := sessionFromEcho(c)
	if job == nil {
		return false
	}
	if !job.SiteID.Valid || job.SiteID.Int64 == 0 {
		return isAdminServiceToken(c) || isSessionAdmin(ctx, s, sess)
	}
	if sess == nil || sess.UserID == 0 {
		return false
	}
	site, err := s.Sites.GetByID(ctx, job.SiteID.Int64)
	if err != nil || site == nil {
		// Orphaned job (site deleted): only super_admin.
		return isSessionSuperAdmin(ctx, s, sess)
	}
	return hasSitePermission(ctx, s, sess, site, required)
}
