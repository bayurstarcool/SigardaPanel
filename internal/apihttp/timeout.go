package apihttp

import (
	"context"
	"net/http"
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
			// Skip timeout for WebSocket connections
			return c.Request().Header.Get("Upgrade") == "websocket"
		},
	}
}

// TimeoutMiddleware returns an echo middleware that adds request timeout.
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

			done := make(chan error, 1)
			go func() {
				done <- next(c)
			}()

			select {
			case err := <-done:
				return err
			case <-ctx.Done():
				return c.JSON(http.StatusGatewayTimeout, map[string]string{
					"error": "Request timeout",
				})
			}
		}
	}
}
