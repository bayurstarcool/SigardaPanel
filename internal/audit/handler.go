package audit

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/httpjson"
)

// AuditLogStore defines persistence operations for audit logs.
type AuditLogStore interface {
	List(ctx context.Context, filter AuditLogFilter) ([]Entry, error)
	NullifyUser(ctx context.Context, userID int64) error
}

// AuditLogFilter holds query parameters for listing audit logs.
type AuditLogFilter struct {
	Limit    int
	Offset   int
	UserID   int64
	Resource string
	Action   string
}

// Handler provides HTTP endpoints for audit logs.
type Handler struct {
	store   AuditLogStore
	adminMW echo.MiddlewareFunc
}

// NewHandler returns a Handler wired with the given store and admin middleware.
func NewHandler(store AuditLogStore, adminMW echo.MiddlewareFunc) *Handler {
	return &Handler{store: store, adminMW: adminMW}
}

// Register attaches audit routes to the echo group.
func (h *Handler) Register(g *echo.Group) {
	g.GET("/audit-logs", h.List, h.adminMW)
}

// List returns recent audit logs. Admin only.
func (h *Handler) List(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)

	limit := 100
	if v := c.QueryParam("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 1000 {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid limit (1-1000)")
		}
		limit = n
	}

	offset := 0
	if v := c.QueryParam("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid offset")
		}
		offset = n
	}

	filter := AuditLogFilter{
		Limit:  limit,
		Offset: offset,
	}

	if v := c.QueryParam("user_id"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid user_id")
		}
		filter.UserID = n
	}

	filter.Resource = c.QueryParam("resource")
	filter.Action = c.QueryParam("action")

	entries, err := h.store.List(c.Request().Context(), filter)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		out = append(out, map[string]any{
			"id":          e.ID,
			"user_id":     nullInt64Value(e.UserID),
			"action":      e.Action,
			"resource":    e.Resource,
			"resource_id": nullInt64Value(e.ResourceID),
			"details":     nullStringValue(e.Details),
			"created_at":  e.CreatedAt,
		})
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"entries": out,
			"limit":   limit,
			"offset":  offset,
			"count":   len(out),
		},
		RequestID: rid,
	})
}

func nullInt64Value(n sql.NullInt64) any {
	if !n.Valid {
		return nil
	}
	return n.Int64
}

func nullStringValue(n sql.NullString) any {
	if !n.Valid {
		return nil
	}
	return n.String
}
