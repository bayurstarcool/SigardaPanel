package apihttp

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/store"
)

type orderHandler struct {
	store *store.Store
}

func newOrderHandler(s *store.Store) *orderHandler {
	return &orderHandler{store: s}
}

// listOrders returns orders. Admin sees all, regular user sees own only.
func (h *orderHandler) listOrders(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)

	var filter store.OrderListFilter

	// Non-admin users can only see their own orders
	if !hasAdminRole(c, h.store) {
		filter.UserID = sess.UserID
	}

	// Optional status filter
	if status := c.QueryParam("status"); status != "" {
		filter.Status = status
	}

	orders, err := h.store.Orders.List(c.Request().Context(), filter)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list orders")
	}

	// Resolve usernames and plan names
	userCache := make(map[int64]string)
	planCache := make(map[int64]string)
	var out []orderJSON
	for _, o := range orders {
		username := userCache[o.UserID]
		if username == "" {
			if u, err := h.store.Users.GetByID(c.Request().Context(), o.UserID); err == nil && u != nil {
				username = u.Username
			}
			userCache[o.UserID] = username
		}
		planName := planCache[o.PlanID]
		if planName == "" {
			if p, err := h.store.Plans.GetByID(c.Request().Context(), o.PlanID); err == nil && p != nil {
				planName = p.Name
			}
			planCache[o.PlanID] = planName
		}
		out = append(out, orderToJSON(o, username, planName))
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"orders": out, "count": len(out)},
		RequestID: rid,
	})
}

// createOrder creates a new license order (authenticated users).
func (h *orderHandler) createOrder(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)

	var body struct {
		PlanID        int64  `json:"plan_id"`
		PaymentMethod string `json:"payment_method"`
		PaymentProof  string `json:"payment_proof"`
		Notes         string `json:"notes"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if body.PlanID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "plan_id required")
	}

	// Verify plan exists and get price
	plan, err := h.store.Plans.GetByID(c.Request().Context(), body.PlanID)
	if err != nil || plan == nil {
		return echo.NewHTTPError(http.StatusNotFound, "plan not found")
	}
	if !plan.IsActive {
		return echo.NewHTTPError(http.StatusBadRequest, "plan is not active")
	}

	order := &store.LicenseOrder{
		UserID:    sess.UserID,
		PlanID:    body.PlanID,
		Status:    "pending",
		AmountIDR: plan.PriceIDR,
	}
	if body.PaymentMethod != "" {
		order.PaymentMethod = sql.NullString{String: body.PaymentMethod, Valid: true}
	}
	if body.PaymentProof != "" {
		order.PaymentProof = sql.NullString{String: body.PaymentProof, Valid: true}
	}
	if body.Notes != "" {
		order.Notes = sql.NullString{String: body.Notes, Valid: true}
	}

	if err := h.store.Orders.Create(c.Request().Context(), order); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create order")
	}

	return c.JSON(http.StatusCreated, httpjson.Success{
		Data:      orderToJSON(*order, "", plan.Name),
		RequestID: rid,
	})
}

// getOrder returns a single order by ID.
func (h *orderHandler) getOrder(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid order id")
	}

	order, err := h.store.Orders.GetByID(c.Request().Context(), id)
	if err != nil || order == nil {
		return echo.NewHTTPError(http.StatusNotFound, "order not found")
	}

	// Regular users can only view their own orders
	if !hasAdminRole(c, h.store) && order.UserID != sess.UserID {
		return echo.NewHTTPError(http.StatusNotFound, "order not found")
	}

	username := ""
	if u, err := h.store.Users.GetByID(c.Request().Context(), order.UserID); err == nil && u != nil {
		username = u.Username
	}
	planName := ""
	if p, err := h.store.Plans.GetByID(c.Request().Context(), order.PlanID); err == nil && p != nil {
		planName = p.Name
	}
	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      orderToJSON(*order, username, planName),
		RequestID: rid,
	})
}

// updateOrderStatus updates an order's status (admin only).
func (h *orderHandler) updateOrderStatus(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid order id")
	}

	var body struct {
		Status     string `json:"status"`
		AdminNotes string `json:"admin_notes"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	// Validate status
	validStatuses := map[string]bool{
		"pending": true, "approved": true, "rejected": true, "expired": true,
	}
	if !validStatuses[body.Status] {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid status: must be pending, approved, rejected, or expired")
	}

	// Verify order exists
	order, err := h.store.Orders.GetByID(c.Request().Context(), id)
	if err != nil || order == nil {
		return echo.NewHTTPError(http.StatusNotFound, "order not found")
	}

	if err := h.store.Orders.UpdateStatus(c.Request().Context(), id, body.Status, body.AdminNotes, sess.UserID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update order")
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"updated": true, "id": id, "status": body.Status},
		RequestID: rid,
	})
}

type orderJSON struct {
	ID            int64   `json:"id"`
	UserID        int64   `json:"user_id"`
	Username      string  `json:"username"`
	PlanID        int64   `json:"plan_id"`
	PlanName      string  `json:"plan_name"`
	Status        string  `json:"status"`
	PaymentMethod *string `json:"payment_method"`
	PaymentProof  *string `json:"payment_proof"`
	AmountIDR     int64   `json:"amount_idr"`
	Notes         *string `json:"notes"`
	AdminNotes    *string `json:"admin_notes"`
	ApprovedBy    *int64  `json:"approved_by"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
}

func orderToJSON(o store.LicenseOrder, username, planName string) orderJSON {
	var pm, pp, notes, adminNotes *string
	var approvedBy *int64
	if o.PaymentMethod.Valid {
		pm = &o.PaymentMethod.String
	}
	if o.PaymentProof.Valid {
		pp = &o.PaymentProof.String
	}
	if o.Notes.Valid {
		notes = &o.Notes.String
	}
	if o.AdminNotes.Valid {
		adminNotes = &o.AdminNotes.String
	}
	if o.ApprovedBy.Valid {
		approvedBy = &o.ApprovedBy.Int64
	}
	return orderJSON{
		ID:            o.ID,
		UserID:        o.UserID,
		Username:      username,
		PlanID:        o.PlanID,
		PlanName:      planName,
		Status:        o.Status,
		PaymentMethod: pm,
		PaymentProof:  pp,
		AmountIDR:     o.AmountIDR,
		Notes:         notes,
		AdminNotes:    adminNotes,
		ApprovedBy:    approvedBy,
		CreatedAt:     o.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:     o.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}
