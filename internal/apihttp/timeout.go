package apihttp

import (
	"context"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

// TimeoutConfig defines the timeout middleware configuration.
type TimeoutConfig struct {
	Timeout time.Duration
	Skipper func(c echo.Context) bool
}

// DefaultTimeoutConfig returns a default timeout configuration.
func DefaultTimeoutConfig() TimeoutConfig {
	return TimeoutConfig{
		Timeout: 30 * time.Second,
		Skipper: func(c echo.Context) bool {
			if c.Request().Header.Get("Upgrade") == "websocket" {
				return true
			}
			// Streaming transfers legitimately outlive the request budget.
			p := c.Request().URL.Path
			return strings.HasSuffix(p, "/download") || strings.HasSuffix(p, "/upload")
		},
	}
}

// TimeoutMiddleware attaches a deadline to the request context. Handlers run
// on the request goroutine, so a panic reaches the Recover middleware and the
// response is never written by two goroutines at once (the previous version
// raced the handler and could append a 504 body to a half-written download).
// Work that honours the context (agent calls, DB queries) is cancelled at the
// deadline.
func TimeoutMiddleware(config ...TimeoutConfig) echo.MiddlewareFunc {
	cfg := DefaultTimeoutConfig()
	if len(config) > 0 {
		cfg = config[0]
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if cfg.Skipper != nil && cfg.Skipper(c) {
				return next(c)
			}
			ctx, cancel := context.WithTimeout(c.Request().Context(), cfg.Timeout)
			defer cancel()
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	}
}
