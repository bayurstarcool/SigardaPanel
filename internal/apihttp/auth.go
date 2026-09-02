package apihttp

import (
	"os"
	"log/slog"
	"database/sql"
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pquerna/otp/totp"
	"sigardapanel/internal/audit"
	"sigardapanel/internal/auth"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/store"
)

// DefaultSessionTTL is the default session token lifetime.
const DefaultSessionTTL = 7 * 24 * time.Hour // 7 days
const RememberMeSessionTTL = 90 * 24 * time.Hour // 90 days

// rateLimiter implements a simple per-IP login attempt limiter.
type rateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	limit    int
	window   time.Duration
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		attempts: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
}

func (rl *rateLimiter) allow(ip string) bool {
	if os.Getenv("SIGARDAPANEL_DEV") == "true" {
		return true
	}
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	// Filter old attempts
	var valid []time.Time
	for _, t := range rl.attempts[ip] {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	rl.attempts[ip] = valid

	if len(valid) >= rl.limit {
		return false
	}
	rl.attempts[ip] = append(valid, now)
	return true
}

type authHandler struct {
	store       *store.Store
	audit       *audit.Recorder
	rateLimiter *rateLimiter
	sessionTTL  time.Duration
}

func newAuthHandler(s *store.Store, a *audit.Recorder) *authHandler {
	return &authHandler{
		store:       s,
		audit:       a,
		rateLimiter: newRateLimiter(10, 15*time.Minute),
		sessionTTL:  DefaultSessionTTL,
	}
}

func (ah *authHandler) login(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	ip := c.RealIP()
	if !ah.rateLimiter.allow(ip) {
		return echo.NewHTTPError(http.StatusTooManyRequests, "too many login attempts")
	}

	var body struct {
		Username   string `json:"username"`
		Password   string `json:"password"`
		TOTPCode   string `json:"totp_code"`   // 2FA code
		RememberMe bool   `json:"remember_me"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	user, err := ah.store.Users.GetByUsername(c.Request().Context(), body.Username)
	if err != nil {
		slog.Error("login: get user", "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if user == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid credentials")
	}

	ok, err := auth.VerifyPassword(user.PasswordHash, body.Password)
	if err != nil {
		slog.Error("login: verify password", "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if !ok {
		return c.JSON(http.StatusUnauthorized, httpjson.ErrorBody{
			Error:     httpjson.APIError{Code: "invalid_credentials", Message: "invalid credentials"},
			RequestID: rid,
		})
	}

	// Check if 2FA is enabled for this user
	if user.TOTPEnabled {
		if body.TOTPCode == "" {
			return c.JSON(http.StatusUnauthorized, httpjson.ErrorBody{
				Error:     httpjson.APIError{Code: "2fa_required", Message: "Two-factor authentication code required"},
				RequestID: rid,
			})
		}

		// Validate TOTP code
		valid := false
		if user.TOTPSecret != "" {
			valid = totp.Validate(body.TOTPCode, user.TOTPSecret)
		}
		if !valid {
			return c.JSON(http.StatusUnauthorized, httpjson.ErrorBody{
				Error:     httpjson.APIError{Code: "invalid_2fa_code", Message: "Invalid two-factor authentication code"},
				RequestID: rid,
			})
		}
	}

	token, err := auth.NewToken()
	if err != nil {
		slog.Error("login: new token", "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	sessionTTL := ah.sessionTTL
	if body.RememberMe {
		sessionTTL = RememberMeSessionTTL
	}
	slog.Info("login: session TTL", "ttl", sessionTTL, "hours", sessionTTL.Hours(), "remember_me", body.RememberMe)

	expiresAt := time.Now().Add(sessionTTL)
	slog.Info("login: session expires", "expires_at", expiresAt.Format("2006-01-02 15:04:05"), "expires_utc", expiresAt.UTC().Format("2006-01-02 15:04:05"), "session_ttl_hours", sessionTTL.Hours())

	sess := &store.Session{
		UserID:    user.ID,
		TokenHash: auth.HashToken(token),
		ExpiresAt: expiresAt,
	}
	if err := ah.store.Sessions.Create(c.Request().Context(), sess); err != nil {
		slog.Error("login: create session", "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	// Enforce max sessions per user — revoke oldest if over limit
	const maxSessionsPerUser = 10
	if err := ah.store.Sessions.RevokeOldest(c.Request().Context(), user.ID, maxSessionsPerUser); err != nil {
		slog.Warn("login: failed to revoke old sessions", "error", err)
	}

	slog.Info("user logged in", "user_id", user.ID, "username", user.Username)

	ah.audit.RecordAction(c.Request().Context(), user.ID, audit.ActionLogin, audit.ResourceSession, sess.ID, map[string]any{
		"ip": c.RealIP(),
	})

	// Check if password needs to be changed
	passwordChangeRequired := !user.PasswordChangedAt.Valid

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"token": token,
			"user": map[string]any{
				"id":                       user.ID,
				"username":                 user.Username,
				"email":                    user.Email,
				"role":                     user.Role,
				"password_change_required": passwordChangeRequired,
			},
		},
		RequestID: rid,
	})
}

// register creates a new user account (public endpoint).
func (ah *authHandler) register(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)

	var body struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
		Company  string `json:"company"`
		Plan     string `json:"plan"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	// Validate
	if body.Name == "" || body.Email == "" || body.Password == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name, email, and password are required")
	}
	if len(body.Password) < 8 {
		return echo.NewHTTPError(http.StatusBadRequest, "password must be at least 8 characters")
	}

	// Check if email already exists
	existing, _ := ah.store.Users.GetByEmail(c.Request().Context(), body.Email)
	if existing != nil {
		return echo.NewHTTPError(http.StatusConflict, "email already registered")
	}

	// Create user
	user := &store.User{
		Username:     body.Email,
		Email:        body.Email,
		Role:         auth.RoleUser,
		CreatedBy:    sql.NullInt64{Int64: 1, Valid: true},
		PasswordHash: body.Password,
	}
	if err := ah.store.Users.Create(c.Request().Context(), user); err != nil {
		slog.Error("register: create user", "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create user")
	}

	slog.Info("user registered", "user_id", user.ID, "email", body.Email, "plan", body.Plan)

	return c.JSON(http.StatusCreated, httpjson.Success{
		Data: map[string]any{
			"id":       user.ID,
			"username": user.Username,
			"email":    user.Email,
			"role":     user.Role,
			"message":  "account created successfully",
		},
		RequestID: rid,
	})
}

func (ah *authHandler) logout(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	if sess == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	if err := ah.store.Sessions.Revoke(c.Request().Context(), sess.ID); err != nil {
		slog.Error("logout: revoke session", "error", err)
	} else {
		ah.audit.RecordAction(c.Request().Context(), sess.UserID, audit.ActionLogout, audit.ResourceSession, sess.ID, nil)
	}
	slog.Info("session revoked", "session_id", sess.ID, "user_id", sess.UserID)
	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"ok": true},
		RequestID: rid,
	})
}

func (ah *authHandler) me(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	if sess == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	user, err := ah.store.Users.GetByID(c.Request().Context(), sess.UserID)
	if err != nil {
		slog.Error("me: get user", "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if user == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "user not found")
	}

	data := map[string]any{
		"id":       user.ID,
		"username": user.Username,
		"email":    user.Email,
		"role":     user.Role,
	}

	// Include impersonation info if this is an impersonated session
	if sess.ImpersonatedBy.Valid {
		impersonator, err := ah.store.Users.GetByID(c.Request().Context(), sess.ImpersonatedBy.Int64)
		if err == nil && impersonator != nil {
			data["impersonated_by"] = sess.ImpersonatedBy.Int64
			data["impersonator"] = map[string]any{
				"id":       impersonator.ID,
				"username": impersonator.Username,
				"email":    impersonator.Email,
				"role":     impersonator.Role,
			}
		}
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:     data,
		RequestID: rid,
	})
}

// updateProfile allows the authenticated user to update their own profile (email, password).
// PUT /api/v1/auth/profile
func (ah *authHandler) updateProfile(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	if sess == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}

	var body struct {
		Email       string `json:"email"`
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	ctx := c.Request().Context()
	user, err := ah.store.Users.GetByID(ctx, sess.UserID)
	if err != nil || user == nil {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}

	// Build update details for audit log
	details := map[string]any{}

	if body.Email != "" && body.Email != user.Email {
		if err := ah.store.Users.UpdateEmail(ctx, sess.UserID, body.Email); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		details["email"] = body.Email
	}

	if body.NewPassword != "" {
		if body.OldPassword == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "old_password is required to change password")
		}
		// Verify current password
		match, err := auth.VerifyPassword(user.PasswordHash, body.OldPassword)
		if err != nil || !match {
			return echo.NewHTTPError(http.StatusBadRequest, "current password is incorrect")
		}
		// Hash new password
		hash, err := auth.HashPassword(body.NewPassword)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
		}
		if err := ah.store.Users.UpdatePassword(ctx, sess.UserID, hash); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
		}
		details["password_changed"] = true
	}

	if len(details) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "nothing to update")
	}

	ah.audit.RecordAction(ctx, sess.UserID, audit.ActionUserUpdate, audit.ResourceUser, sess.UserID, details)

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"updated": true},
		RequestID: rid,
	})
}
