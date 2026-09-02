package apihttp

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/store"
)

type notificationHandler struct {
	s *store.Store
}

func newNotificationHandler(s *store.Store) *notificationHandler {
	return &notificationHandler{s: s}
}

func (h *notificationHandler) rid(c echo.Context) string {
	return c.Response().Header().Get(echo.HeaderXRequestID)
}

// list returns notifications for the current user.
// GET /api/v1/notifications?limit=50&offset=0
func (h *notificationHandler) list(c echo.Context) error {
	sess := sessionFromEcho(c)
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	offset, _ := strconv.Atoi(c.QueryParam("offset"))
	if limit <= 0 {
		limit = 50
	}

	notifications, total, err := h.s.Notifications.ListByUser(c.Request().Context(), sess.UserID, limit, offset)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	unread, _ := h.s.Notifications.CountUnread(c.Request().Context(), sess.UserID)

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"notifications": notifications,
			"total":         total,
			"unread":        unread,
		},
		RequestID: h.rid(c),
	})
}

// count returns the unread count for the current user.
// GET /api/v1/notifications/unread-count
func (h *notificationHandler) count(c echo.Context) error {
	sess := sessionFromEcho(c)
	count, err := h.s.Notifications.CountUnread(c.Request().Context(), sess.UserID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"count": count},
		RequestID: h.rid(c),
	})
}

// markRead marks a single notification as read.
// POST /api/v1/notifications/:id/read
func (h *notificationHandler) markRead(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid notification id")
	}
	if err := h.s.Notifications.MarkRead(c.Request().Context(), id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"marked_read": true},
		RequestID: h.rid(c),
	})
}

// markAllRead marks all of the user's notifications as read.
// POST /api/v1/notifications/read-all
func (h *notificationHandler) markAllRead(c echo.Context) error {
	sess := sessionFromEcho(c)
	if err := h.s.Notifications.MarkAllRead(c.Request().Context(), sess.UserID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"marked_all_read": true},
		RequestID: h.rid(c),
	})
}

// delete removes a single notification.
// DELETE /api/v1/notifications/:id
func (h *notificationHandler) delete(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid notification id")
	}
	if err := h.s.Notifications.Delete(c.Request().Context(), id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"deleted": true},
		RequestID: h.rid(c),
	})
}

// deleteAllRead removes all read notifications for the current user.
// DELETE /api/v1/notifications/read
func (h *notificationHandler) deleteAllRead(c echo.Context) error {
	sess := sessionFromEcho(c)
	if err := h.s.Notifications.DeleteAllRead(c.Request().Context(), sess.UserID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"deleted_all_read": true},
		RequestID: h.rid(c),
	})
}
