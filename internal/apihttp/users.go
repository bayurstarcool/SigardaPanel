package apihttp

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
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

func (h *userHandler) list(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	role := c.Get(roleKey).(string)

	var filter store.UserListFilter
	// Admin sees only users they created; super_admin sees all
	if role == auth.RoleAdmin {
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

func (h *userHandler) create(c echo.Context) error {
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
	if callerRole == auth.RoleAdmin && body.Role != auth.RoleUser {
		return echo.NewHTTPError(http.StatusForbidden, "admins can only create users")
	}

	user := &store.User{
		Username:     body.Username,
		Email:        body.Email,
		PasswordHash: body.Password,
		Role:         body.Role,
		CreatedBy:    int64ToNullInt64(sess.UserID),
		DiskLimit:    int64PtrToNullInt64(body.DiskLimit),
		MaxSites:     int64PtrToNullInt64(body.MaxSites),
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

func (h *userHandler) delete(c echo.Context) error {
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

	// Admin can only delete users they created
	if callerRole == auth.RoleAdmin {
		u, err := h.store.Users.GetByID(c.Request().Context(), id)
		if err != nil || u == nil || !u.CreatedBy.Valid || u.CreatedBy.Int64 != sess.UserID {
			return echo.NewHTTPError(http.StatusNotFound, "user not found")
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

	if err := h.store.Users.Delete(c.Request().Context(), id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"deleted": true, "id": id},
		RequestID: rid,
	})
}

func (h *userHandler) update(c echo.Context) error {
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

	// Admin can only update users they created
	if callerRole == auth.RoleAdmin {
		u, err := h.store.Users.GetByID(c.Request().Context(), id)
		if err != nil || u == nil || !u.CreatedBy.Valid || u.CreatedBy.Int64 != sess.UserID {
			return echo.NewHTTPError(http.StatusNotFound, "user not found")
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
		if err := h.store.Users.UpdatePassword(c.Request().Context(), id, hash); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
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

func int64PtrToNullInt64(v *int64) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{Valid: false}
	}
	return sql.NullInt64{Int64: *v, Valid: true}
}

// usage returns real disk usage and site count for a user.
func (h *userHandler) usage(c echo.Context) error {
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

	// Regular users can only check their own usage
	if !auth.HasRole(caller.Role, auth.RoleAdmin) && sess.UserID != id {
		return echo.NewHTTPError(http.StatusForbidden, "forbidden")
	}

	user, err := h.store.Users.GetByID(c.Request().Context(), id)
	if err != nil || user == nil {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}

	isAdminUser := auth.HasRole(user.Role, auth.RoleAdmin)

	// For admin/super_admin: show total server disk usage and all sites.
	// For regular users: show only their owned + assigned sites.
	var sitesToMeasure []store.Site
	var totalSites int

	if isAdminUser {
		// Admin sees all sites
		allSites, err := h.store.Sites.List(c.Request().Context(), store.SiteListFilter{Limit: 500})
		if err == nil {
			sitesToMeasure = allSites
			totalSites = len(allSites)
		}
	} else {
		// Regular user: owned + assigned
		ownedSites, err := h.store.Sites.List(c.Request().Context(), store.SiteListFilter{OwnerID: id, Limit: 500})
		if err == nil {
			sitesToMeasure = append(sitesToMeasure, ownedSites...)
		}
		assignedSites, _ := h.store.UserSites.ListByUser(c.Request().Context(), id)
		totalSites = len(ownedSites) + len(assignedSites)

		// Add assigned sites to measure (avoid double-count)
		countedIDs := make(map[int64]bool, len(ownedSites))
		for _, s := range ownedSites {
			countedIDs[s.ID] = true
		}
		for _, as := range assignedSites {
			if countedIDs[as.SiteID] {
				continue
			}
			site, err := h.store.Sites.GetByID(c.Request().Context(), as.SiteID)
			if err != nil || site == nil {
				continue
			}
			sitesToMeasure = append(sitesToMeasure, *site)
			countedIDs[as.SiteID] = true
		}
	}

	// Calculate real disk usage from site root_path directories
	var diskUsedBytes int64
	for _, site := range sitesToMeasure {
		if site.RootPath == "" {
			continue
		}
		out, err := execCommand("du", "-sb", site.RootPath)
		if err == nil {
			parts := strings.SplitN(out, "	", 2)
			if len(parts) >= 1 {
				if sz, parseErr := strconv.ParseInt(parts[0], 10, 64); parseErr == nil {
					diskUsedBytes += sz
				}
			}
		}
	}

	// Convert disk_limit from GB to bytes for the response
	var diskLimitBytes int64
	if user.DiskLimit.Valid && user.DiskLimit.Int64 > 0 {
		diskLimitBytes = user.DiskLimit.Int64 * 1024 * 1024 * 1024
	}

	var maxSites *int64
	if user.MaxSites.Valid {
		ms := user.MaxSites.Int64
		maxSites = &ms
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"user_id":          id,
			"disk_used_bytes":  diskUsedBytes,
			"disk_limit_bytes": diskLimitBytes,
			"disk_limit_gb":    user.DiskLimit.Int64,
			"disk_unlimited":   !user.DiskLimit.Valid || user.DiskLimit.Int64 == 0,
			"site_count":       totalSites,
			"max_sites":        maxSites,
		},
		RequestID: rid,
	})
}

// execCommand runs a command and returns stdout as string.
func execCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
