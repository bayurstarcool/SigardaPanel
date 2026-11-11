package apihttp

import (
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/store"
)

// codePasswordChangeRequired is the error code of the 403 returned while an
// account still uses a password someone else chose (installer-generated or
// set by an administrator: users.password_changed_at IS NULL).
const codePasswordChangeRequired = "password_change_required"

// passwordChangeAllowedRoutes are the only routes a login session may use
// until the password is changed: read the account, change the password
// (PUT /auth/profile with old_password/new_password), see the 2FA status the
// profile page shows, and log out.
var passwordChangeAllowedRoutes = map[string]bool{
	http.MethodGet + " /api/v1/auth/me":         true,
	http.MethodPut + " /api/v1/auth/profile":    true,
	http.MethodPost + " /api/v1/auth/logout":    true,
	http.MethodGet + " /api/v1/auth/2fa/status": true,
}

func allowedDuringPasswordChange(c echo.Context) bool {
	return passwordChangeAllowedRoutes[c.Request().Method+" "+c.Path()]
}

// passwordChangePending reports whether userID must still change its
// password. A missing user is left to the handlers (they answer 401/404).
func passwordChangePending(c echo.Context, s *store.Store, userID int64) (bool, error) {
	if userID <= 0 {
		return false, nil
	}
	user, err := s.Users.GetByID(c.Request().Context(), userID)
	if err != nil {
		if store.IsNotFound(err) {
			return false, nil
		}
		slog.Error("api: password-change check: get user", "route", c.Path(), "error", err)
		return false, echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	return user != nil && user.PasswordChangeRequired(), nil
}

func passwordChangeRequiredError(c echo.Context, message string) error {
	return c.JSON(http.StatusForbidden, httpjson.ErrorBody{
		Error:     httpjson.APIError{Code: codePasswordChangeRequired, Message: message},
		RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
	})
}
