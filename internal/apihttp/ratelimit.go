package apihttp

import (
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
)

// RateLimiter implements a token bucket rate limiter
type RateLimiter struct {
	visitors map[string]*visitor
	mu       sync.RWMutex
	rate     int           // requests per interval (bucket capacity)
	interval time.Duration // time interval
	now      func() time.Time
}

type visitor struct {
	// tokens is fractional: at slow request rates each refill step is less
	// than one whole token, and truncating it to int (while also resetting
	// the refill clock) meant the bucket never refilled at all.
	tokens     float64
	lastRefill time.Time
	lastSeen   time.Time
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(rate int, interval time.Duration) *RateLimiter {
	rl := newRateLimiterNoCleanup(rate, interval)

	// Cleanup old visitors every minute
	go func() {
		for {
			time.Sleep(time.Minute)
			rl.cleanup()
		}
	}()

	return rl
}

func newRateLimiterNoCleanup(rate int, interval time.Duration) *RateLimiter {
	return &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     rate,
		interval: interval,
		now:      time.Now,
	}
}

// cleanup removes old visitors
func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := rl.now()
	for ip, v := range rl.visitors {
		// A visitor idle for a full interval has a full bucket again, so
		// forgetting it is equivalent to keeping it.
		if now.Sub(v.lastSeen) > rl.interval*2 {
			delete(rl.visitors, ip)
		}
	}
}

// Allow checks if a request from the given IP is allowed
func (rl *RateLimiter) Allow(ip string) bool {
	if os.Getenv("SIGARDAPANEL_DEV") == "true" {
		return true
	}
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := rl.now()
	v, exists := rl.visitors[ip]
	if !exists {
		rl.visitors[ip] = &visitor{
			tokens:     float64(rl.rate) - 1,
			lastRefill: now,
			lastSeen:   now,
		}
		return true
	}
	v.lastSeen = now

	// Refill tokens based on time elapsed since the last refill, keeping the
	// fractional part so slow, steady traffic still accumulates tokens.
	if elapsed := now.Sub(v.lastRefill); elapsed > 0 && rl.interval > 0 {
		v.tokens += elapsed.Seconds() * float64(rl.rate) / rl.interval.Seconds()
		if v.tokens > float64(rl.rate) {
			v.tokens = float64(rl.rate)
		}
	}
	v.lastRefill = now

	// Small epsilon so float rounding (e.g. 0.333…+0.666… = 0.9999…) does not
	// deny a request that has exactly earned its token.
	if v.tokens < 1-1e-9 {
		return false
	}

	v.tokens--
	return true
}

// Middleware returns an echo middleware that rate limits requests
func (rl *RateLimiter) Middleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ip := c.RealIP()
			if ip == "" {
				ip = c.Request().RemoteAddr
			}

			if !rl.Allow(ip) {
				return c.JSON(http.StatusTooManyRequests, map[string]any{
					"error":   "rate limit exceeded",
					"message": "Too many requests. Please try again later.",
				})
			}

			return next(c)
		}
	}
}

// StrictLimiter for login endpoints (10 attempts per 15 minutes)
func StrictLimiter() *RateLimiter {
	return NewRateLimiter(10, 15*time.Minute)
}

// APIlimiter for general API (100 requests per minute)
func APIlimiter() *RateLimiter {
	return NewRateLimiter(100, time.Minute)
}
