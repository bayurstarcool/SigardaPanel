package apihttp

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"
	"sigardapanel/internal/audit"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/rbac"
	"sigardapanel/internal/store"
)

// roleHandler serves the permission registry and custom roles (RBAC).
type roleHandler struct {
	store *store.Store
	audit *audit.Recorder
}

func newRoleHandler(s *store.Store, a *audit.Recorder) *roleHandler {
	return &roleHandler{store: s, audit: a}
}

// permissions lists the registry (names, categories, dangerous flags).
func (h *roleHandler) permissions(c *echo.Context) error {
	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"permissions": rbac.All()},
		RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
	})
}

func (h *roleHandler) roleJSON(c *echo.Context, caller *store.User, r store.Role) map[string]any {
	ctx := c.Request().Context()
	n, _ := h.store.Roles.CountUsers(ctx, r.Name)
	return map[string]any{
		"id":          r.ID,
		"name":        r.Name,
		"description": r.Description,
		"built_in":    r.BuiltIn,
		"permissions": rbac.Sorted(r.Permissions),
		"dangerous":   rbac.HasDangerous(r.Permissions),
		"users":       n,
		// Whether the caller may give this role to a user it manages.
		"assignable": assignableRole(ctx, h.store, caller, r.Name),
	}
}

// list returns every role with what the caller may do with it.
func (h *roleHandler) list(c *echo.Context) error {
	ctx := c.Request().Context()
	caller := sessionUser(ctx, h.store, sessionFromEcho(c))
	roles, err := h.store.Roles.List(ctx)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	out := make([]map[string]any, 0, len(roles))
	for _, r := range roles {
		out = append(out, h.roleJSON(c, caller, r))
	}
	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"roles": out},
		RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
	})
}

type roleBody struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

// checkPermissions validates a requested permission set against the
// registry and the caller: no unknown permission, nothing the caller lacks,
// dangerous ones only with user.manage_all (super_admin).
func (h *roleHandler) checkPermissions(c *echo.Context, caller *store.User, names []string) (map[string]bool, error) {
	set := map[string]bool{}
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if _, ok := rbac.Get(n); !ok {
			return nil, echo.NewHTTPError(http.StatusBadRequest, "unknown permission "+n)
		}
		set[n] = true
	}
	mine := rolePermissions(c.Request().Context(), h.store, caller.Role)
	if !rbac.Subset(set, mine) {
		return nil, echo.NewHTTPError(http.StatusForbidden, "a role cannot hold permissions you do not have")
	}
	if rbac.HasDangerous(set) && !mine[rbac.UserManageAll] {
		return nil, echo.NewHTTPError(http.StatusForbidden, "only a super admin can grant dangerous permissions")
	}
	return set, nil
}

func (h *roleHandler) create(c *echo.Context) error {
	ctx := c.Request().Context()
	sess := sessionFromEcho(c)
	caller := sessionUser(ctx, h.store, sess)
	if caller == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "user session required")
	}
	var body roleBody
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	body.Name = strings.TrimSpace(body.Name)
	if !store.ValidCustomRoleName(body.Name) {
		return echo.NewHTTPError(http.StatusBadRequest, "role name must be 2-32 lowercase letters, digits, _ or - (starting with a letter), not a built-in role")
	}
	if len(body.Description) > 200 {
		return echo.NewHTTPError(http.StatusBadRequest, "description too long (200 characters)")
	}
	if exists, err := h.store.Roles.Exists(ctx, body.Name); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	} else if exists {
		return echo.NewHTTPError(http.StatusConflict, "a role with this name already exists")
	}
	perms, err := h.checkPermissions(c, caller, body.Permissions)
	if err != nil {
		return err
	}
	r := &store.Role{Name: body.Name, Description: strings.TrimSpace(body.Description), Permissions: perms}
	r.CreatedBy.Int64, r.CreatedBy.Valid = caller.ID, true
	if err := h.store.Roles.Create(ctx, r); err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	h.audit.RecordAction(ctx, caller.ID, "role_create", "role", r.ID, map[string]any{"name": r.Name, "permissions": rbac.Sorted(perms)})
	stored, _ := h.store.Roles.GetByID(ctx, r.ID)
	if stored == nil {
		stored = r
	}
	return c.JSON(http.StatusCreated, httpjson.Success{
		Data:      h.roleJSON(c, caller, *stored),
		RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
	})
}

func (h *roleHandler) roleParam(c *echo.Context) (*store.Role, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return nil, echo.NewHTTPError(http.StatusBadRequest, "invalid role id")
	}
	r, err := h.store.Roles.GetByID(c.Request().Context(), id)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if r == nil {
		return nil, echo.NewHTTPError(http.StatusNotFound, "role not found")
	}
	if r.BuiltIn {
		return nil, echo.NewHTTPError(http.StatusForbidden, "built-in roles cannot be changed")
	}
	return r, nil
}

func (h *roleHandler) update(c *echo.Context) error {
	ctx := c.Request().Context()
	caller := sessionUser(ctx, h.store, sessionFromEcho(c))
	if caller == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "user session required")
	}
	r, err := h.roleParam(c)
	if err != nil {
		return err
	}
	var body roleBody
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if len(body.Description) > 200 {
		return echo.NewHTTPError(http.StatusBadRequest, "description too long (200 characters)")
	}
	perms, err := h.checkPermissions(c, caller, body.Permissions)
	if err != nil {
		return err
	}
	if err := h.store.Roles.Update(ctx, r.ID, strings.TrimSpace(body.Description), perms); err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	h.audit.RecordAction(ctx, caller.ID, "role_update", "role", r.ID, map[string]any{"name": r.Name, "permissions": rbac.Sorted(perms)})
	stored, _ := h.store.Roles.GetByID(ctx, r.ID)
	if stored == nil {
		stored = r
	}
	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      h.roleJSON(c, caller, *stored),
		RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
	})
}

func (h *roleHandler) delete(c *echo.Context) error {
	ctx := c.Request().Context()
	caller := sessionUser(ctx, h.store, sessionFromEcho(c))
	if caller == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "user session required")
	}
	r, err := h.roleParam(c)
	if err != nil {
		return err
	}
	if err := h.store.Roles.Delete(ctx, r.ID); err != nil {
		if errors.Is(err, store.ErrRoleInUse) {
			return echo.NewHTTPError(http.StatusConflict, "the role is assigned to users; give them another role first")
		}
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	h.audit.RecordAction(ctx, caller.ID, "role_delete", "role", r.ID, map[string]any{"name": r.Name})
	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"deleted": true},
		RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
	})
}
