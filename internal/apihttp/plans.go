package apihttp

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/auth"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/store"
)

type planHandler struct {
	store *store.Store
}

func newPlanHandler(s *store.Store) *planHandler {
	return &planHandler{store: s}
}

// listPlans returns all plans (active only for non-admin).
func (h *planHandler) listPlans(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)

	// Non-admin users only see active plans
	isAdmin := hasAdminRole(c, h.store)
	activeOnly := !isAdmin

	plans, err := h.store.Plans.List(c.Request().Context(), activeOnly)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list plans")
	}

	var out []planJSON
	for _, p := range plans {
		out = append(out, planToJSON(p))
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"plans": out, "count": len(out)},
		RequestID: rid,
	})
}

// createPlan creates a new plan (admin only).
func (h *planHandler) createPlan(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)

	var body struct {
		Name         string `json:"name"`
		Slug         string `json:"slug"`
		PriceIDR     int64  `json:"price_idr"`
		BillingCycle string `json:"billing_cycle"`
		Features     string `json:"features"`
		IsActive     *bool  `json:"is_active"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if body.Name == "" || body.Slug == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name and slug required")
	}
	if body.BillingCycle == "" {
		body.BillingCycle = "monthly"
	}
	isActive := true
	if body.IsActive != nil {
		isActive = *body.IsActive
	}
	if body.Features == "" {
		body.Features = "{}"
	}

	plan := &store.Plan{
		Name:         body.Name,
		Slug:         body.Slug,
		PriceIDR:     body.PriceIDR,
		BillingCycle: body.BillingCycle,
		Features:     body.Features,
		IsActive:     isActive,
	}
	if err := h.store.Plans.Create(c.Request().Context(), plan); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusCreated, httpjson.Success{
		Data:      planToJSON(*plan),
		RequestID: rid,
	})
}

// getPlan returns a single plan by ID.
func (h *planHandler) getPlan(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid plan id")
	}

	plan, err := h.store.Plans.GetByID(c.Request().Context(), id)
	if err != nil || plan == nil {
		return echo.NewHTTPError(http.StatusNotFound, "plan not found")
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      planToJSON(*plan),
		RequestID: rid,
	})
}

// updatePlan updates an existing plan (admin only).
func (h *planHandler) updatePlan(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid plan id")
	}

	plan, err := h.store.Plans.GetByID(c.Request().Context(), id)
	if err != nil || plan == nil {
		return echo.NewHTTPError(http.StatusNotFound, "plan not found")
	}

	var body struct {
		Name         *string `json:"name"`
		Slug         *string `json:"slug"`
		PriceIDR     *int64  `json:"price_idr"`
		BillingCycle *string `json:"billing_cycle"`
		Features     *string `json:"features"`
		IsActive     *bool   `json:"is_active"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if body.Name != nil {
		plan.Name = *body.Name
	}
	if body.Slug != nil {
		plan.Slug = *body.Slug
	}
	if body.PriceIDR != nil {
		plan.PriceIDR = *body.PriceIDR
	}
	if body.BillingCycle != nil {
		plan.BillingCycle = *body.BillingCycle
	}
	if body.Features != nil {
		plan.Features = *body.Features
	}
	if body.IsActive != nil {
		plan.IsActive = *body.IsActive
	}

	if err := h.store.Plans.Update(c.Request().Context(), plan); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      planToJSON(*plan),
		RequestID: rid,
	})
}

// deletePlan removes a plan (admin only).
func (h *planHandler) deletePlan(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid plan id")
	}

	if err := h.store.Plans.Delete(c.Request().Context(), id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete plan")
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"deleted": true, "id": id},
		RequestID: rid,
	})
}

// assignPlan assigns a plan to a user (admin only).
func (h *userHandler) assignPlan(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || userID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid user id")
	}

	// Verify user exists
	user, err := h.store.Users.GetByID(c.Request().Context(), userID)
	if err != nil || user == nil {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}

	var body struct {
		PlanID int64 `json:"plan_id"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	var planID sql.NullInt64
	if body.PlanID > 0 {
		// Verify plan exists
		plan, err := h.store.Plans.GetByID(c.Request().Context(), body.PlanID)
		if err != nil || plan == nil {
			return echo.NewHTTPError(http.StatusNotFound, "plan not found")
		}
		planID = sql.NullInt64{Int64: body.PlanID, Valid: true}
	}
	// planID 0 or negative = unassign (Valid stays false)

	if err := h.store.Users.UpdatePlan(c.Request().Context(), userID, planID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to assign plan")
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"updated": true, "user_id": userID, "plan_id": body.PlanID},
		RequestID: rid,
	})
}

// hasAdminRole checks if the current user has admin or super_admin role.
// The role is only put in the context by RequireRole, so on routes guarded by
// RequireAuth alone it is looked up from the session's user record. Service
// tokens are never treated as admins here.
func hasAdminRole(c echo.Context, s *store.Store) bool {
	if role, ok := c.Get(roleKey).(string); ok && role != "" {
		return role == auth.RoleAdmin || role == auth.RoleSuperAdmin
	}
	return isSessionAdmin(c.Request().Context(), s, sessionFromEcho(c))
}

type planJSON struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	PriceIDR     int64  `json:"price_idr"`
	BillingCycle string `json:"billing_cycle"`
	Features     string `json:"features"`
	IsActive     bool   `json:"is_active"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

func planToJSON(p store.Plan) planJSON {
	return planJSON{
		ID:           p.ID,
		Name:         p.Name,
		Slug:         p.Slug,
		PriceIDR:     p.PriceIDR,
		BillingCycle: p.BillingCycle,
		Features:     p.Features,
		IsActive:     p.IsActive,
		CreatedAt:    p.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:    p.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}
