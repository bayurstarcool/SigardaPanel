package member
import "sigardapanel/internal/auth"

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// RegisterRoutes registers all member API routes.
func RegisterRoutes(e *echo.Echo, db *sql.DB) {
	h := NewHandler(db)

	member := e.Group("/api/v1/member")

	// Public routes
	member.POST("/register", h.Register)
	member.POST("/login", h.Login)

	// Protected routes (require member session)
	authGroup := member.Group("")
	authGroup.Use(requireMemberAuth(db))
	authGroup.GET("/me", h.Me)
	authGroup.PUT("/plan", h.UpdatePlan)
}

// requireMemberAuth is middleware that validates member session tokens.
func requireMemberAuth(db *sql.DB) echo.MiddlewareFunc {
	store := NewStore(db)
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			authHeader := c.Request().Header.Get("Authorization")
			if authHeader == "" {
				return echo.NewHTTPError(http.StatusUnauthorized, "missing authorization header")
			}

			token := strings.TrimPrefix(authHeader, "Bearer ")
			if token == authHeader {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid authorization format")
			}

			sess, err := store.GetSession(auth.HashToken(token))
			if err != nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid or expired session")
			}

			c.Set("member_id", sess.MemberID)
			return next(c)
		}
	}
}
