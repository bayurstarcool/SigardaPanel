package apihttp

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/auth"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/store"
)

// listUserSites returns all sites a given user has access to (owned + assigned).
func (h *userHandler) listUserSites(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	_ = rid

	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || userID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid user id")
	}

	// Only admin/super_admin can see other users' site mappings
	sess := sessionFromEcho(c)
	role := c.Get(roleKey).(string)
	if !auth.HasRole(role, auth.RoleAdmin) {
		if sess == nil || sess.UserID != userID {
			return echo.NewHTTPError(http.StatusForbidden, "access denied")
		}
	}

	// Get assigned sites from user_sites table
	assignedSites, _ := h.store.UserSites.ListByUser(c.Request().Context(), userID)

	// Get owned sites (sites where user is owner_id)
	ownedSites, _ := h.store.Sites.List(c.Request().Context(), store.SiteListFilter{OwnerID: userID, Limit: 500})

	// Merge: owned sites (permission=owner) + assigned sites
	type siteEntry struct {
		SiteID     int64  `json:"site_id"`
		SiteName   string `json:"site_name"`
		SiteDomain string `json:"site_domain"`
		Permission string `json:"permission"`
		AssignedAt string `json:"assigned_at,omitempty"`
		IsOwner    bool   `json:"is_owner"`
	}
	var merged []siteEntry

	// Add owned sites first
	for _, s := range ownedSites {
		merged = append(merged, siteEntry{
			SiteID:     s.ID,
			SiteName:   s.Name,
			SiteDomain: s.Domain,
			Permission: "owner",
			IsOwner:    true,
		})
	}

	// Add assigned sites (skip if already in owned list)
	for _, s := range assignedSites {
		alreadyOwned := false
		for _, m := range merged {
			if m.SiteID == s.SiteID {
				alreadyOwned = true
				break
			}
		}
		if !alreadyOwned {
			siteName := s.Domain
			if assignedSite, err := h.store.Sites.GetByID(c.Request().Context(), s.SiteID); err == nil && assignedSite != nil {
				siteName = assignedSite.Name
			}
			merged = append(merged, siteEntry{
				SiteID:     s.SiteID,
				SiteName:   siteName,
				SiteDomain: s.Domain,
				Permission: s.Permission,
				AssignedAt: s.CreatedAt,
				IsOwner:    false,
			})
		}
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"sites": merged, "count": len(merged)},
		RequestID: rid,
	})
}

// listSiteUsers returns all users who have access to a given site.
func (h *userHandler) listSiteUsers(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)

	siteID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || siteID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid site id")
	}

	// Verify site exists and caller has access
	site, err := h.store.Sites.GetByID(c.Request().Context(), siteID)
	if err != nil || site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}
	if !hasSitePermission(c.Request().Context(), h.store, sess, site, sitePermRead) {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	users, err := h.store.UserSites.ListBySite(c.Request().Context(), siteID)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"users": users, "count": len(users)},
		RequestID: rid,
	})
}

// requireSiteAssignmentManager loads the site and verifies the caller may
// manage its user assignments: only the site's owner or a super_admin may
// (AGENTS.md permission matrix, "Manage users"). Callers who cannot see the
// site at all get 404; callers who can see it but are not owner get 403.
func (h *userHandler) requireSiteAssignmentManager(c echo.Context, siteID int64) error {
	ctx := c.Request().Context()
	sess := sessionFromEcho(c)
	site, err := h.store.Sites.GetByID(ctx, siteID)
	if err != nil || site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}
	if isSiteOwnerOrSuperAdmin(ctx, h.store, sess, site) {
		return nil
	}
	if hasSitePermission(ctx, h.store, sess, site, sitePermRead) {
		return echo.NewHTTPError(http.StatusForbidden, "only the site owner or a super admin can manage site users")
	}
	return echo.NewHTTPError(http.StatusNotFound, "site not found")
}

// assignUserSite grants a user access to a site.
func (h *userHandler) assignUserSite(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)

	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || userID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid user id")
	}

	var body struct {
		SiteID     int64  `json:"site_id"`
		Permission string `json:"permission"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if body.SiteID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "site_id required")
	}
	if body.Permission == "" {
		body.Permission = "read"
	}
	if body.Permission != "read" && body.Permission != "write" && body.Permission != "admin" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid permission (read|write|admin)")
	}

	if err := h.requireSiteAssignmentManager(c, body.SiteID); err != nil {
		return err
	}
	if target, err := h.store.Users.GetByID(c.Request().Context(), userID); err != nil || target == nil {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}

	if err := h.store.UserSites.Assign(c.Request().Context(), userID, body.SiteID, body.Permission); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), sess.UserID, "update", "user",
		userID, map[string]any{
			"action":     "assign_site",
			"site_id":    body.SiteID,
			"permission": body.Permission,
		})

	h.store.Notify(c.Request().Context(), userID, "site_assigned", "Site Assigned", fmt.Sprintf("You were assigned to site %d", body.SiteID), "info", "site", body.SiteID)
	h.store.NotifyAdmins(c.Request().Context(), "user_site_assigned", "User Assigned", fmt.Sprintf("User %d assigned to site %d", userID, body.SiteID), "info", "site", body.SiteID)

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"ok": true, "user_id": userID, "site_id": body.SiteID, "permission": body.Permission},
		RequestID: rid,
	})
}

// unassignUserSite revokes a user's access to a site.
func (h *userHandler) unassignUserSite(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)

	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || userID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid user id")
	}

	siteID, err := strconv.ParseInt(c.Param("siteId"), 10, 64)
	if err != nil || siteID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid site id")
	}

	if err := h.requireSiteAssignmentManager(c, siteID); err != nil {
		return err
	}

	if err := h.store.UserSites.Unassign(c.Request().Context(), userID, siteID); err != nil {
		if err.Error() == "sql: no rows in result set" || err.Error() == "no rows found" {
			return echo.NewHTTPError(http.StatusNotFound, "assignment not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), sess.UserID, "update", "user",
		userID, map[string]any{
			"action":  "unassign_site",
			"site_id": siteID,
		})

	h.store.Notify(c.Request().Context(), userID, "site_removed", "Site Removed", fmt.Sprintf("You were removed from site %d", siteID), "warning", "site", siteID)

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"ok": true, "user_id": userID, "site_id": siteID},
		RequestID: rid,
	})
}

// updateSitePermission changes the permission level for an existing assignment.
func (h *userHandler) updateSitePermission(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)

	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || userID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid user id")
	}

	siteID, err := strconv.ParseInt(c.Param("siteId"), 10, 64)
	if err != nil || siteID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid site id")
	}

	var body struct {
		Permission string `json:"permission"`
	}
	if err := c.Bind(&body); err != nil || body.Permission == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "permission required (read|write|admin)")
	}
	if body.Permission != "read" && body.Permission != "write" && body.Permission != "admin" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid permission (read|write|admin)")
	}

	if err := h.requireSiteAssignmentManager(c, siteID); err != nil {
		return err
	}

	if err := h.store.UserSites.UpdatePermission(c.Request().Context(), userID, siteID, body.Permission); err != nil {
		if err.Error() == "sql: no rows in result set" || err.Error() == "no rows found" {
			return echo.NewHTTPError(http.StatusNotFound, "assignment not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), sess.UserID, "update", "user",
		userID, map[string]any{
			"action":     "update_site_permission",
			"site_id":    siteID,
			"permission": body.Permission,
		})

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"ok": true, "user_id": userID, "site_id": siteID, "permission": body.Permission},
		RequestID: rid,
	})
}
