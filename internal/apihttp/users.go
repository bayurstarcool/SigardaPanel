package apihttp

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v5"
	"sigardapanel/internal/audit"
	"sigardapanel/internal/auth"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/store"
)

type userHandler struct {
	store *store.Store
	audit *audit.Recorder
}

func newUserHandler(s *store.Store, a *audit.Recorder) *userHandler {
	return &userHandler{store: s, audit: a}
}

func (h *userHandler) list(c *echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	role := c.Get(roleKey).(string)

	var filter store.UserListFilter
	// Admin sees only users they created; super_admin sees all. Anything that
	// is not a super_admin (including service tokens) gets the restricted view.
	if role != auth.RoleSuperAdmin {
		filter.CreatedBy = sess.UserID
	}

	users, err := h.store.Users.List(c.Request().Context(), filter)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	type userResp struct {
		ID        int64  `json:"id"`
		Username  string `json:"username"`
		Email     string `json:"email"`
		Role      string `json:"role"`
		CreatedAt string `json:"created_at"`
		DiskLimit *int64 `json:"disk_limit"` // nil = unlimited
		MaxSites  *int64 `json:"max_sites"`  // nil = unlimited
		PlanID    *int64 `json:"plan_id"`    // nil = no plan
	}
	var out []userResp
	for _, u := range users {
		var dl *int64
		if u.DiskLimit.Valid {
			dl = &u.DiskLimit.Int64
		}
		var ms *int64
		if u.MaxSites.Valid {
			ms = &u.MaxSites.Int64
		}
		var pid *int64
		if u.PlanID.Valid {
			pid = &u.PlanID.Int64
		}
		out = append(out, userResp{
			ID: u.ID, Username: u.Username, Email: u.Email,
			Role: u.Role, CreatedAt: u.CreatedAt.Format(time.RFC3339),
			DiskLimit: dl, MaxSites: ms, PlanID: pid,
		})
	}
	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"users": out, "count": len(out)},
		RequestID: rid,
	})
}

func (h *userHandler) create(c *echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	callerRole := c.Get(roleKey).(string)

	var body struct {
		Username  string `json:"username"`
		Email     string `json:"email"`
		Password  string `json:"password"`
		Role      string `json:"role"`
		DiskLimit *int64 `json:"disk_limit"` // nil = unlimited, 0 = unlimited too
		MaxSites  *int64 `json:"max_sites"`  // nil = unlimited, 0 = unlimited too
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if body.Username == "" || body.Password == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "username and password required")
	}
	if body.Email == "" || !emailRegex.MatchString(body.Email) {
		return echo.NewHTTPError(http.StatusBadRequest, "valid email required")
	}
	if body.Role == "" {
		body.Role = "user"
	}

	// Permission: admin can only create "user" role; super_admin can create any
	if body.Role == auth.RoleSuperAdmin && callerRole != auth.RoleSuperAdmin {
		return echo.NewHTTPError(http.StatusForbidden, "cannot create super admin")
	}
	if callerRole != auth.RoleSuperAdmin && body.Role != auth.RoleUser {
		return echo.NewHTTPError(http.StatusForbidden, "admins can only create users")
	}

	user := &store.User{
		Username:     body.Username,
		Email:        body.Email,
		PasswordHash: body.Password,
		Role:         body.Role,
		CreatedBy:    int64ToNullInt64(sess.UserID),
		// 0 (or less) means unlimited, as on update and in the CLI.
		DiskLimit: limitToNull(body.DiskLimit),
		MaxSites:  limitToNull(body.MaxSites),
		// The administrator knows this password: the user must choose their
		// own before using the panel.
		MustChangePassword: true,
	}
	if err := h.store.Users.Create(c.Request().Context(), user); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	h.audit.RecordAction(c.Request().Context(), sess.UserID, audit.ActionUserCreate, audit.ResourceUser, user.ID, map[string]any{
		"username": body.Username,
		"role":     body.Role,
	})

	h.store.NotifyAdmins(c.Request().Context(), "user_created", "User Created", fmt.Sprintf("User %s created", body.Username), "info", "user", user.ID)

	var dl *int64
	if user.DiskLimit.Valid {
		dl = &user.DiskLimit.Int64
	}
	var ms *int64
	if user.MaxSites.Valid {
		ms = &user.MaxSites.Int64
	}
	var pid *int64
	if user.PlanID.Valid {
		pid = &user.PlanID.Int64
	}
	return c.JSON(http.StatusCreated, httpjson.Success{
		Data: map[string]any{
			"id": user.ID, "username": user.Username,
			"email": user.Email, "role": user.Role,
			"disk_limit": dl, "max_sites": ms, "plan_id": pid,
		},
		RequestID: rid,
	})
}

func (h *userHandler) delete(c *echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	callerRole := c.Get(roleKey).(string)

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid user id")
	}
	if sess.UserID == id {
		return echo.NewHTTPError(http.StatusBadRequest, "cannot delete yourself")
	}

	// Only super_admin may delete arbitrary users; everyone else (admins and,
	// defensively, service tokens) may only delete users they created.
	if callerRole != auth.RoleSuperAdmin {
		u, err := h.store.Users.GetByID(c.Request().Context(), id)
		if err != nil || u == nil || !u.CreatedBy.Valid || u.CreatedBy.Int64 != sess.UserID {
			return echo.NewHTTPError(http.StatusNotFound, "user not found")
		}
		// A user the admin created may have been promoted since.
		if u.Role != auth.RoleUser {
			return echo.NewHTTPError(http.StatusForbidden, "only a super admin can delete admin accounts")
		}
	}

	// Record audit log before nullifying FKs (actor is the admin, not the deleted user)
	h.audit.RecordAction(c.Request().Context(), sess.UserID, audit.ActionUserDelete, audit.ResourceUser, id, nil)

	// Nullify FK references before deleting user
	if err := h.store.Users.NullifyCreatedBy(c.Request().Context(), id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if err := h.store.Sites.NullifyOwner(c.Request().Context(), id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if err := h.store.AuditLogs.NullifyUser(c.Request().Context(), id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	if err := h.store.Sessions.DeleteImpersonationsBy(c.Request().Context(), id); err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if err := h.store.Users.Delete(c.Request().Context(), id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"deleted": true, "id": id},
		RequestID: rid,
	})
}

func (h *userHandler) update(c *echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	callerRole := c.Get(roleKey).(string)

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid user id")
	}
	var body struct {
		Role      string `json:"role"`
		Password  string `json:"password"`
		DiskLimit *int64 `json:"disk_limit"` // nil=ignored, 0=unlimited, N=GB
		MaxSites  *int64 `json:"max_sites"`  // nil=ignored, 0=unlimited, N=sites
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	// Only super_admin may update arbitrary users; everyone else (admins and,
	// defensively, service tokens) may only update users they created.
	if callerRole != auth.RoleSuperAdmin {
		u, err := h.store.Users.GetByID(c.Request().Context(), id)
		if err != nil || u == nil || !u.CreatedBy.Valid || u.CreatedBy.Int64 != sess.UserID {
			return echo.NewHTTPError(http.StatusNotFound, "user not found")
		}
		// A user the admin created may have been promoted since: resetting
		// their password would hand the admin that role.
		if u.Role != auth.RoleUser {
			return echo.NewHTTPError(http.StatusForbidden, "only a super admin can change admin accounts")
		}
	}

	// Permission: only super_admin can promote to admin/super_admin
	if callerRole != auth.RoleSuperAdmin && body.Role != "" && (body.Role == auth.RoleAdmin || body.Role == auth.RoleSuperAdmin) {
		return echo.NewHTTPError(http.StatusForbidden, "only super admin can assign admin role")
	}

	if body.Role != "" {
		if err := h.store.Users.UpdateRole(c.Request().Context(), id, body.Role); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
	}
	if body.Password != "" {
		hash, err := auth.HashPassword(body.Password)
		if err != nil {
			slog.Error("api: request failed", "route", c.Path(), "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
		}
		ctx := c.Request().Context()
		if id == sess.UserID {
			// A stolen session must not take the account over: your own
			// password changes through Profile, which asks for the current one.
			return echo.NewHTTPError(http.StatusBadRequest, "change your own password in Profile (it asks for the current password)")
		} else {
			// Chosen by an admin: the user must replace it at the next
			// login, and nobody keeps a session opened with the old one.
			if err := h.store.Users.SetTemporaryPassword(ctx, id, hash); err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
			}
			_ = h.store.Sessions.RevokeAllForUser(ctx, id, 0)
		}
	}
	if body.DiskLimit != nil {
		// disk_limit explicitly provided (0=unlimited, N=GB)
		var dl sql.NullInt64
		if *body.DiskLimit > 0 {
			dl = sql.NullInt64{Int64: *body.DiskLimit, Valid: true}
		}
		// 0 means unlimited → dl.Valid stays false
		if err := h.store.Users.UpdateDiskLimit(c.Request().Context(), id, dl); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
		}
	}
	if body.MaxSites != nil {
		// max_sites explicitly provided (0=unlimited, N=sites)
		var ms sql.NullInt64
		if *body.MaxSites > 0 {
			ms = sql.NullInt64{Int64: *body.MaxSites, Valid: true}
		}
		if err := h.store.Users.UpdateMaxSites(c.Request().Context(), id, ms); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
		}
	}
	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"updated": true, "id": id},
		RequestID: rid,
	})
}

func int64ToNullInt64(v int64) sql.NullInt64 {
	return sql.NullInt64{Int64: v, Valid: true}
}

// limitToNull maps a quota to its column: nil or <= 0 is unlimited (NULL).
func limitToNull(v *int64) sql.NullInt64 {
	if v == nil || *v <= 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *v, Valid: true}
}

// usage returns real disk usage and site count for a user.
func (h *userHandler) usage(c *echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid user id")
	}

	// Get caller's full user to check role
	caller, callerErr := h.store.Users.GetByID(c.Request().Context(), sess.UserID)
	if callerErr != nil || caller == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	user, err := h.store.Users.GetByID(c.Request().Context(), id)
	if err != nil || user == nil {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}
	// Own usage, or a user the caller manages (super_admin: anyone).
	if sess.UserID != id && !callerManagesUser(c, h.store, user) {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}

	ctx := c.Request().Context()
	limitBytes := diskLimitBytes(user)
	ownedSites, err := h.store.Sites.List(ctx, store.SiteListFilter{OwnerID: id, Limit: maxOwnedSites})
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	// What the figure covers:
	//   - a user with a disk limit: the sites they own, against the limit
	//     (what the limit is enforced on; assigned sites count to their owners);
	//   - an admin without a limit: every site on the panel, against the
	//     disk capacity of all servers;
	//   - anyone else without a limit: their own sites, against the capacity
	//     of the servers those sites are on.
	scope := "owned"
	sites := ownedSites
	if limitBytes == 0 && auth.HasRole(user.Role, auth.RoleAdmin) {
		all, err := h.store.Sites.List(ctx, store.SiteListFilter{Limit: maxOwnedSites})
		if err != nil {
			slog.Error("api: request failed", "route", c.Path(), "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
		}
		scope, sites = "all", all
	}
	disk := siteDisk.total(ctx, h.store, sites, siteDiskWait)

	// Disk capacity (latest metrics) of the servers in scope, for users
	// without a limit: shown as "used / capacity".
	var capacity int64
	if limitBytes == 0 {
		servers := map[int64]bool{}
		if scope == "all" {
			if list, err := h.store.Servers.List(ctx); err == nil {
				for _, sv := range list {
					servers[sv.ID] = true
				}
			}
		} else {
			for _, s := range sites {
				servers[s.ServerID] = true
			}
		}
		for sid := range servers {
			if m, err := h.store.Metrics.GetLatest(ctx, sid); err == nil && m != nil && m.DiskTotal.Valid && m.DiskTotal.Int64 > 0 {
				capacity += m.DiskTotal.Int64
			}
		}
	}

	assigned := 0
	if as, err := h.store.UserSites.ListByUser(ctx, id); err == nil {
		owned := make(map[int64]bool, len(ownedSites))
		for _, s := range ownedSites {
			owned[s.ID] = true
		}
		for _, a := range as {
			if !owned[a.SiteID] {
				assigned++
			}
		}
	}
	// Site count follows max_sites (owned sites); without it, an admin sees
	// every site.
	siteCount := len(ownedSites)
	if !user.MaxSites.Valid && scope == "all" {
		siteCount = len(sites)
	}
	var measuredAt any
	if !disk.MeasuredAt.IsZero() {
		measuredAt = disk.MeasuredAt.UTC().Format(time.RFC3339)
	}

	var maxSites *int64
	if user.MaxSites.Valid {
		ms := user.MaxSites.Int64
		maxSites = &ms
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"user_id":               id,
			"disk_used_bytes":       disk.Bytes,
			"disk_measured_sites":   disk.Measured,
			"disk_unmeasured_sites": disk.Unmeasured,
			"disk_measured_at":      measuredAt,
			"disk_limit_bytes":      limitBytes,
			"disk_limit_gb":         user.DiskLimit.Int64,
			"disk_unlimited":        limitBytes == 0,
			"disk_over_limit":       limitBytes > 0 && disk.Bytes >= limitBytes,
			"disk_scope":            scope,
			"disk_capacity_bytes":   capacity,
			"site_count":            siteCount,
			"assigned_site_count":   assigned,
			"max_sites":             maxSites,
		},
		RequestID: rid,
	})
}
