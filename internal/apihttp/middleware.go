package apihttp

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"sigardapanel/internal/audit"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
	"sigardapanel/internal/auth"
	"sigardapanel/internal/rbac"
	"sigardapanel/internal/store"
)

const (
	sessionKey            = "session"
	userIDKey             = "user_id"
	roleKey               = "role"
	serviceTokenScopesKey = "service_token_scopes"
	// apiKeyIDKey is set (to the key's ID) when the request authenticated
	// with a personal API key instead of a login session.
	apiKeyIDKey = "api_key_id"
)

// RequireAuth returns Echo middleware that validates a Bearer token.
// It sets the session and user_id in the Echo context.
func RequireAuth(s *store.Store) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			// Prefer the Authorization header; fall back to the HttpOnly
			// session cookie so browser clients never need the token in JS.
			var token string
			header := c.Request().Header.Get("Authorization")
			if header != "" {
				parts := strings.SplitN(header, " ", 2)
				if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
					return echo.NewHTTPError(http.StatusUnauthorized, "invalid authorization header format")
				}
				token = parts[1]
			} else if ck, err := c.Cookie(sessionCookieName); err == nil && ck.Value != "" {
				token = ck.Value
			}
			if token == "" {
				return echo.NewHTTPError(http.StatusUnauthorized, "missing authorization header")
			}
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
				go s.ServiceTokens.UpdateLastUsed(context.Background(), serviceToken.ID)
				// Store scopes in context for downstream role checks
				c.Set(serviceTokenScopesKey, serviceToken.Scopes)
				c.Set(sessionKey, &store.Session{ID: serviceToken.ID, UserID: 0})
				c.Set(userIDKey, int64(0))
				return next(c)
			}

			// Then: personal API keys. They act as their owner, but carry a
			// synthetic session with ID 0 so session-ID based operations
			// (logout, impersonation stop) can never touch a real session row.
			apiKey, err := s.APIKeys.GetByKeyHash(c.Request().Context(), tokenHash)
			if err != nil {
				slog.Error("api: api key lookup failed", "route", c.Path(), "error", err)
				return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
			}
			if apiKey != nil {
				if apiKey.ExpiresAt.Valid && time.Now().After(apiKey.ExpiresAt.Time) {
					return echo.NewHTTPError(http.StatusUnauthorized, "api key expired")
				}
				// The owner still has to replace a password someone else
				// chose: refuse the key outright (it cannot change it).
				if blocked, err := passwordChangePending(c, s, apiKey.UserID); err != nil {
					return err
				} else if blocked {
					return passwordChangeRequiredError(c, "the account of this API key must change its password first: sign in to the web panel (Profile) or run 'sigardapanel passwd' after 'sigardapanel login', then use the key again")
				}
				go s.APIKeys.UpdateLastUsed(context.Background(), apiKey.ID)
				c.Set(apiKeyIDKey, apiKey.ID)
				c.Set(sessionKey, &store.Session{ID: 0, UserID: apiKey.UserID})
				c.Set(userIDKey, apiKey.UserID)
				return next(c)
			}

			// Then: check sessions table
			sess, err := s.Sessions.GetByTokenHash(c.Request().Context(), tokenHash)
			if err != nil {
				slog.Error("api: request failed", "route", c.Path(), "error", err)
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

			// An impersonation lasts only while the impersonator may still
			// start it: still an admin (or super_admin) who manages the
			// target. Demoting or deleting the admin ends it at once.
			if sess.ImpersonatedBy.Valid {
				if ok, err := impersonationStillAllowed(c.Request().Context(), s, sess); err != nil {
					slog.Error("api: request failed", "route", c.Path(), "error", err)
					return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
				} else if !ok {
					_ = s.Sessions.RevokeByTokenHash(c.Request().Context(), tokenHash)
					return echo.NewHTTPError(http.StatusUnauthorized, "session revoked")
				}
			}

			// Until the user replaces a password someone else chose, a login
			// session may only read itself, change the password and log out.
			// Impersonation sessions are exempt: the admin must not be made
			// to change the user's password.
			if !sess.ImpersonatedBy.Valid && !allowedDuringPasswordChange(c) {
				if blocked, err := passwordChangePending(c, s, sess.UserID); err != nil {
					return err
				} else if blocked {
					return passwordChangeRequiredError(c, "you must change your password before using the panel: open Profile in the web panel, or run 'sigardapanel passwd'")
				}
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
		return func(c *echo.Context) error {
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

// isAPIKeyRequest reports whether the request authenticated with a personal
// API key rather than an interactive login session.
func isAPIKeyRequest(c *echo.Context) bool {
	_, ok := c.Get(apiKeyIDKey).(int64)
	return ok
}

// RequireUserSession returns Echo middleware that rejects service tokens and
// personal API keys: only interactive login sessions pass.
// Use this on endpoints that must only be accessed by real user sessions
// (e.g., impersonation, service token management).
func RequireUserSession(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		sess, ok := c.Get(sessionKey).(*store.Session)
		if !ok {
			return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
		}
		if sess.UserID == 0 {
			return echo.NewHTTPError(http.StatusForbidden, "service token cannot access this endpoint")
		}
		if isAPIKeyRequest(c) {
			return echo.NewHTTPError(http.StatusForbidden, "api key cannot access this endpoint")
		}
		return next(c)
	}
}

func sessionFromEcho(c *echo.Context) *store.Session {
	sess, _ := c.Get(sessionKey).(*store.Session)
	return sess
}

// RequireInternalIP restricts endpoint to localhost only.
func RequireInternalIP(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
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
		return func(c *echo.Context) error {
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
					"error":         "feature_not_available",
					"feature":       feature,
					"required_tier": featureMinTier(feature),
					"current_tier":  currentTier,
					"upgrade_url":   "/settings/billing",
					"request_id":    rid,
				})
			}
			return next(c)
		}
	}
}

// RequireTier returns Echo middleware that requires minimum tier level.
func RequireTier(s *store.Store, minTier string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			rid := c.Response().Header().Get(echo.HeaderXRequestID)

			license, err := s.Licenses.GetActive(c.Request().Context())
			currentTier := "free"
			if err == nil && license != nil {
				currentTier = license.Tier
			}

			if tierLevelNum(currentTier) < tierLevelNum(minTier) {
				return c.JSON(http.StatusForbidden, map[string]interface{}{
					"error":         "tier_not_sufficient",
					"required_tier": minTier,
					"current_tier":  currentTier,
					"upgrade_url":   "/settings/billing",
					"request_id":    rid,
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

// scopeAuditLogs admits admins to the audit log; a non-super admin only
// sees their own entries (the log holds every tenant's actions, including
// super_admin command lines).
func scopeAuditLogs(s *store.Store) echo.MiddlewareFunc {
	admin := RequirePermission(s, rbac.AuditView)
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return admin(func(c *echo.Context) error {
			if !sessionCan(c.Request().Context(), s, sessionFromEcho(c), rbac.AuditViewAll) {
				sess := sessionFromEcho(c)
				// Service tokens have no user (UserID 0, which the store
				// reads as "no filter"): they would see every entry.
				if sess == nil || sess.UserID == 0 {
					return echo.NewHTTPError(http.StatusForbidden, "access denied")
				}
				c.Set(audit.ForceUserIDKey, sess.UserID)
			}
			return next(c)
		})
	}
}

// requireSuperAdminOnPanelHost guards actions that are root on the server
// (container create/exec with host mounts, firewall, fail2ban, pm2, mail):
// on the panel host they are as good as super_admin power (database,
// encryption key), so only a super_admin may run them there. Remote servers
// keep the route's own role check.
func requireSuperAdminOnPanelHost(s *store.Store) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			id, err := strconv.ParseInt(c.Param("id"), 10, 64)
			if err != nil || id <= 0 {
				return next(c)
			}
			server, err := s.Servers.GetByID(c.Request().Context(), id)
			if err != nil || server == nil || !isLocalServer(*server) {
				return next(c)
			}
			if sess := sessionFromEcho(c); sess != nil && sessionCan(c.Request().Context(), s, sess, rbac.ServerPanelHost) {
				return next(c)
			}
			return echo.NewHTTPError(http.StatusForbidden, "on the panel host this action requires a super admin")
		}
	}
}

// auditServerAction records a successful server-level action (firewall,
// fail2ban, containers, mail, ...) in the audit log: these change the
// server for everyone and used to leave no trace.
func auditServerAction(rec *audit.Recorder) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			err := next(c)
			if err != nil || responseStatus(c) >= 400 {
				return err
			}
			id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
			var uid int64
			if sess := sessionFromEcho(c); sess != nil {
				uid = sess.UserID
			}
			path := c.Path() // e.g. /api/v1/servers/:id/firewall/deny
			action := "server." + strings.ReplaceAll(strings.TrimPrefix(path, "/api/v1/servers/:id/"), "/", ".")
			rec.RecordAction(c.Request().Context(), uid, action, audit.ResourceServer, id, map[string]any{"method": c.Request().Method})
			return nil
		}
	}
}

// rejectImpersonation refuses requests made while impersonating another
// user (e.g. an admin changing that user's 2FA).
func rejectImpersonation(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		if sess := sessionFromEcho(c); sess != nil && sess.ImpersonatedBy.Valid {
			return echo.NewHTTPError(http.StatusForbidden, "not available while impersonating a user")
		}
		return next(c)
	}
}

// cookieCSRFGuard protects state-changing requests authenticated by the
// session cookie alone (requests sent straight to /api/v1 by a browser):
// the browser's Origin (or Sec-Fetch-Site) must be this site. SameSite=Strict
// still sends the cookie from sibling subdomains, and some handlers parse
// bodies regardless of Content-Type, so a cross-site text/plain form POST
// would otherwise work. Requests with an Authorization header (the web
// app's proxy, the CLI, API keys) are not cookie-based and pass.
func cookieCSRFGuard(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		r := c.Request()
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			return next(c)
		}
		if r.Header.Get("Authorization") != "" {
			return next(c)
		}
		if _, err := r.Cookie(sessionCookieName); err != nil {
			return next(c)
		}
		if site := r.Header.Get("Sec-Fetch-Site"); site != "" {
			if site == "same-origin" || site == "none" {
				return next(c)
			}
			return echo.NewHTTPError(http.StatusForbidden, "cross-site request refused")
		}
		origin := r.Header.Get("Origin")
		if origin == "" {
			if ref := r.Header.Get("Referer"); ref != "" {
				origin = ref
			}
		}
		if origin == "" {
			return next(c) // not a browser
		}
		u, err := url.Parse(origin)
		if err != nil || !strings.EqualFold(u.Host, r.Host) {
			return echo.NewHTTPError(http.StatusForbidden, "cross-site request refused")
		}
		return next(c)
	}
}

// impersonationStillAllowed re-applies the impersonate rules to a running
// impersonation session (see authHandler.impersonate).
func impersonationStillAllowed(ctx context.Context, s *store.Store, sess *store.Session) (bool, error) {
	admin, err := s.Users.GetByID(ctx, sess.ImpersonatedBy.Int64)
	if err != nil {
		return false, err
	}
	if admin == nil {
		return false, nil
	}
	target, err := s.Users.GetByID(ctx, sess.UserID)
	if err != nil {
		return false, err
	}
	return canImpersonate(ctx, s, admin, target), nil
}
