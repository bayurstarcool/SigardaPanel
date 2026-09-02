package apihttp

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
)

// CSRFProtection provides CSRF token generation and validation.
type CSRFProtection struct {
	tokens map[string]csrfToken
	mu     sync.RWMutex
}

type csrfToken struct {
	token     string
	expiresAt time.Time
}

// NewCSRFProtection creates a new CSRF protection instance.
func NewCSRFProtection() *CSRFProtection {
	csrf := &CSRFProtection{
		tokens: make(map[string]csrfToken),
	}

	// Cleanup expired tokens every 5 minutes
	go func() {
		for {
			time.Sleep(5 * time.Minute)
			csrf.cleanup()
		}
	}()

	return csrf
}

// cleanup removes expired tokens.
func (c *CSRFProtection) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for sessionID, token := range c.tokens {
		if now.After(token.expiresAt) {
			delete(c.tokens, sessionID)
		}
	}
}

// GenerateToken generates a new CSRF token for a session.
func (c *CSRFProtection) GenerateToken(sessionID string) string {
	b := make([]byte, 32)
	rand.Read(b)
	token := hex.EncodeToString(b)

	c.mu.Lock()
	c.tokens[sessionID] = csrfToken{
		token:     token,
		expiresAt: time.Now().Add(1 * time.Hour),
	}
	c.mu.Unlock()

	return token
}

// ValidateToken validates a CSRF token for a session.
func (c *CSRFProtection) ValidateToken(sessionID, token string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stored, exists := c.tokens[sessionID]
	if !exists {
		return false
	}

	if time.Now().After(stored.expiresAt) {
		return false
	}

	return stored.token == token
}

// Middleware returns an echo middleware that validates CSRF tokens.
func (c *CSRFProtection) Middleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(ctx echo.Context) error {
			// Skip safe methods
			method := ctx.Request().Method
			if method == "GET" || method == "HEAD" || method == "OPTIONS" {
				return next(ctx)
			}

			// Skip public endpoints
			path := ctx.Request().URL.Path
			if path == "/api/v1/auth/login" || 
			   path == "/api/v1/auth/register" ||
			   path == "/health" ||
			   path == "/api/v1/health" {
				return next(ctx)
			}

			// Get session ID from auth header or cookie
			sessionID := getSessionID(ctx)
			if sessionID == "" {
				return next(ctx) // Allow if no session (API key auth)
			}

			// Get CSRF token from header
			token := ctx.Request().Header.Get("X-CSRF-Token")
			if token == "" {
				// Try form data
				token = ctx.FormValue("csrf_token")
			}

			if token == "" {
				return ctx.JSON(http.StatusForbidden, map[string]string{
					"error": "CSRF token missing",
				})
			}

			if !c.ValidateToken(sessionID, token) {
				return ctx.JSON(http.StatusForbidden, map[string]string{
					"error": "Invalid CSRF token",
				})
			}

			return next(ctx)
		}
	}
}

// getSessionID extracts session ID from the request.
func getSessionID(ctx echo.Context) string {
	// Try to get from cookie
	cookie, err := ctx.Cookie("session_id")
	if err == nil {
		return cookie.Value
	}

	// Try to get from auth header (token hash)
	auth := ctx.Request().Header.Get("Authorization")
	if len(auth) > 7 && auth[:7] == "Bearer " {
		return auth[7:]
	}

	return ""
}
