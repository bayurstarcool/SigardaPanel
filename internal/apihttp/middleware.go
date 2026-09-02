package apihttp

import (
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/auth"
	"sigardapanel/internal/store"
)

const (
	sessionKey          = "session"
	userIDKey           = "user_id"
	roleKey             = "role"
	serviceTokenScopesKey = "service_token_scopes"
)

// RequireAuth returns Echo middleware that validates a Bearer token.
// It sets the session and user_id in the Echo context.
func RequireAuth(s *store.Store) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			header := c.Request().Header.Get("Authorization")
			if header == "" {
				return echo.NewHTTPError(http.StatusUnauthorized, "missing authorization header")
			}
			parts := strings.SplitN(header, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid authorization header format")
			}
			token := parts[1]
			if len(token) != 64 {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid token")
			}

			tokenHash := auth.HashToken(token)

			// First: check service_tokens table
			serviceToken, _ := s.ServiceTokens.GetByKeyHash(c.Request().Context(), tokenHash)
			if serviceToken != nil {
				// Check expiry
				if serviceToken.ExpiresAt.Valid && time.Now().After(serviceToken.ExpiresAt.Time) {
					return echo.NewHTTPError(http.StatusUnauthorized, "service token expired")
				}
				// Update last_used_at (fire-and-forget)
				go s.ServiceTokens.UpdateLastUsed(c.Request().Context(), serviceToken.ID)
				// Store scopes in context for downstream role checks
				c.Set(serviceTokenScopesKey, serviceToken.Scopes)
				c.Set(sessionKey, &store.Session{ID: serviceToken.ID, UserID: 0})
				c.Set(userIDKey, int64(0))
				return next(c)
			}

			// Then: check sessions table
			sess, err := s.Sessions.GetByTokenHash(c.Request().Context(), tokenHash)
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
			}
			if sess == nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid token")
			}
			if sess.RevokedAt.Valid {
				return echo.NewHTTPError(http.StatusUnauthorized, "session revoked")
			}
			if !sess.ExpiresAt.IsZero() && time.Now().After(sess.ExpiresAt) {
				return echo.NewHTTPError(http.StatusUnauthorized, "session expired")
			}

			c.Set(sessionKey, sess)
			c.Set(userIDKey, sess.UserID)
			return next(c)
		}
	}
}

// RequireRole returns Echo middleware that enforces a minimum role.
// Uses hierarchy: super_admin > admin > user
// For service tokens (UserID == 0), checks token scopes instead of user role.
// Service tokens can never be super_admin.
func RequireRole(s *store.Store, required string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			sess, ok := c.Get(sessionKey).(*store.Session)
			if !ok {
				return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
			}

			// Service token path: check scopes instead of user role
			if sess.UserID == 0 {
				scopes, _ := c.Get(serviceTokenScopesKey).(string)
				if scopes == "" {
					return echo.NewHTTPError(http.StatusForbidden, "service token has no scopes")
				}
				if !serviceTokenHasScope(scopes, required) {
					return echo.NewHTTPError(http.StatusForbidden, "service token lacks required scope")
				}
				c.Set(roleKey, "service_token")
				return next(c)
			}

			// Regular user path
			user, err := s.Users.GetByID(c.Request().Context(), sess.UserID)
			if err != nil || user == nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "user not found")
			}
			if !auth.HasRole(user.Role, required) {
				return echo.NewHTTPError(http.StatusForbidden, "insufficient permissions")
			}
			c.Set(roleKey, user.Role)
			return next(c)
		}
	}
}

// serviceTokenHasScope checks if a comma-separated scopes string grants access
// to the required role level.
// Scope "wildcard" grants admin-level access (but never super_admin).
// Scope "admin" grants admin-level access.
// Scope "user" grants user-level access.
// A token with scope "sites" can access endpoints that only need authentication
// (no RequireRole), but NOT endpoints requiring admin.
func serviceTokenHasScope(scopes, required string) bool {
	for _, s := range strings.Split(scopes, ",") {
		s = strings.TrimSpace(s)
		switch {
		case s == "*":
			// Wildcard grants admin-level access (never super_admin)
			if required == auth.RoleAdmin || required == auth.RoleUser {
				return true
			}
		case s == auth.RoleAdmin:
			if required == auth.RoleAdmin || required == auth.RoleUser {
				return true
			}
		case s == auth.RoleUser:
			if required == auth.RoleUser {
				return true
			}
		}
	}
	return false
}

// RequireAdmin returns Echo middleware that requires admin or super_admin role.
func RequireAdmin(s *store.Store) echo.MiddlewareFunc {
	return RequireRole(s, auth.RoleAdmin)
}

// RequireUserSession returns Echo middleware that rejects service tokens.
// Use this on endpoints that must only be accessed by real user sessions
// (e.g., impersonation, service token management).
func RequireUserSession(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		sess, ok := c.Get(sessionKey).(*store.Session)
		if !ok {
			return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
		}
		if sess.UserID == 0 {
			return echo.NewHTTPError(http.StatusForbidden, "service token cannot access this endpoint")
		}
		return next(c)
	}
}

func sessionFromEcho(c echo.Context) *store.Session {
	sess, _ := c.Get(sessionKey).(*store.Session)
	return sess
}

// RequireInternalIP restricts endpoint to localhost only.
func RequireInternalIP(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		addr := c.Request().RemoteAddr
		// Allow only 127.0.0.1 and ::1
		if !strings.HasPrefix(addr, "127.0.0.1:") && !strings.HasPrefix(addr, "[::1]:") {
			return echo.NewHTTPError(http.StatusForbidden, "internal endpoint")
		}
		return next(c)
	}
}

// ─── Feature Gating Middleware ──────────────────────────────────────

// RequireFeature returns Echo middleware that checks if the active license tier
// has access to the specified feature. Returns 403 with upgrade info if denied.
func RequireFeature(s *store.Store, feature string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			rid := c.Response().Header().Get(echo.HeaderXRequestID)

			license, err := s.Licenses.GetActive(c.Request().Context())
			currentTier := "free"
			if err == nil && license != nil {
				currentTier = license.Tier
			}

			// Import features package at the handler level to avoid circular deps
			// Uses the same logic as features.HasFeature
			if !featureHasAccess(currentTier, feature) {
				return c.JSON(http.StatusForbidden, map[string]interface{}{
					"error":        "feature_not_available",
					"feature":      feature,
					"required_tier": featureMinTier(feature),
					"current_tier": currentTier,
					"upgrade_url":  "/settings/billing",
					"request_id":   rid,
				})
			}
			return next(c)
		}
	}
}

// RequireTier returns Echo middleware that requires minimum tier level.
func RequireTier(s *store.Store, minTier string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			rid := c.Response().Header().Get(echo.HeaderXRequestID)

			license, err := s.Licenses.GetActive(c.Request().Context())
			currentTier := "free"
			if err == nil && license != nil {
				currentTier = license.Tier
			}

			if tierLevelNum(currentTier) < tierLevelNum(minTier) {
				return c.JSON(http.StatusForbidden, map[string]interface{}{
					"error":        "tier_not_sufficient",
					"required_tier": minTier,
					"current_tier": currentTier,
					"upgrade_url":  "/settings/billing",
					"request_id":   rid,
				})
			}
			return next(c)
		}
	}
}

// featureHasAccess checks if tier grants access to feature.
// Avoids importing features package in middleware to keep it lightweight.
func featureHasAccess(tier, feature string) bool {
	req := featureMinTier(feature)
	return tierLevelNum(tier) >= tierLevelNum(req)
}

// tierLevelNum returns numeric tier level for comparison.
func tierLevelNum(tier string) int {
	switch tier {
	case "free":
		return 0
	case "pro":
		return 1
	case "enterprise":
		return 2
	default:
		return 0
	}
}

// featureMinTier returns minimum tier for a feature.
func featureMinTier(feature string) string {
	switch feature {
	case "single_server", "static_sites", "php", "ssl_certificates",
		"basic_monitoring", "file_manager", "terminal", "job_history":
		return "free"
	case "multi_server", "advanced_runtimes", "database_management",
		"cloud_backups", "user_management", "alerting",
		"scheduled_backups", "agent_management", "custom_domains", "docker":
		return "pro"
	case "white_label", "api_access", "audit_log", "priority_support":
		return "enterprise"
	default:
		return "free"
	}
}
