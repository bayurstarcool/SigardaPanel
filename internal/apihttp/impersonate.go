package apihttp

import (
	"database/sql"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/auth"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/store"
)

const impersonationSessionTTL = 24 * time.Hour

// impersonate creates a new session acting as another user.
// POST /api/v1/users/:id/impersonate
func (ah *authHandler) impersonate(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	if sess == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}

	// Cannot impersonate while already impersonating
	if sess.ImpersonatedBy.Valid {
		return echo.NewHTTPError(http.StatusBadRequest, "cannot impersonate while already impersonating")
	}

	targetID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || targetID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid user id")
	}

	// Fetch caller role from context
	callerRole := c.Get(roleKey).(string)

	// Fetch target user
	targetUser, err := ah.store.Users.GetByID(c.Request().Context(), targetID)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if targetUser == nil {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}

	// Permission checks
	if callerRole == auth.RoleSuperAdmin {
		// Super admin can impersonate anyone
	} else if callerRole == auth.RoleAdmin {
		// Admin can only impersonate users they created
		if !targetUser.CreatedBy.Valid || targetUser.CreatedBy.Int64 != sess.UserID {
			return echo.NewHTTPError(http.StatusForbidden, "you can only impersonate users you created")
		}
		// Admin cannot impersonate other admins or super_admins
		if targetUser.Role == auth.RoleAdmin || targetUser.Role == auth.RoleSuperAdmin {
			return echo.NewHTTPError(http.StatusForbidden, "cannot impersonate admin or super admin")
		}
	} else {
		return echo.NewHTTPError(http.StatusForbidden, "insufficient permissions")
	}

	// Generate a new token for the impersonation session
	token, err := auth.NewToken()
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	impersonatedSess := &store.Session{
		UserID:         targetUser.ID,
		TokenHash:      auth.HashToken(token),
		ExpiresAt:      time.Now().Add(impersonationSessionTTL),
		ImpersonatedBy: sql.NullInt64{Int64: sess.UserID, Valid: true},
	}
	if err := ah.store.Sessions.Create(c.Request().Context(), impersonatedSess); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	ah.audit.RecordAction(c.Request().Context(), sess.UserID, "impersonate", "user",
		targetUser.ID, map[string]any{
			"target_username": targetUser.Username,
			"target_role":     targetUser.Role,
			"impersonator":    sess.UserID,
		})

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"token": token,
			"user": map[string]any{
				"id":       targetUser.ID,
				"username": targetUser.Username,
				"email":    targetUser.Email,
				"role":     targetUser.Role,
			},
			"impersonated_by": sess.UserID,
			"expires_in":      int(impersonationSessionTTL.Seconds()),
		},
		RequestID: rid,
	})
}

// stopImpersonation revokes the current impersonation session.
// POST /api/v1/auth/stop-impersonation
func (ah *authHandler) stopImpersonation(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	if sess == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}

	if !sess.ImpersonatedBy.Valid {
		return echo.NewHTTPError(http.StatusBadRequest, "not currently impersonating")
	}

	if err := ah.store.Sessions.Revoke(c.Request().Context(), sess.ID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	ah.audit.RecordAction(c.Request().Context(), sess.ImpersonatedBy.Int64, "stop_impersonation", "user",
		sess.UserID, map[string]any{
			"target_session_id": sess.ID,
		})

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"ok": true},
		RequestID: rid,
	})
}
