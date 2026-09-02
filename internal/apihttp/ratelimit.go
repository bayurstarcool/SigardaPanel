package apihttp

import (
	"os"
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
)

// RateLimiter implements a token bucket rate limiter
type RateLimiter struct {
	visitors map[string]*visitor
	mu       sync.RWMutex
	rate     int           // requests per interval
	interval time.Duration // time interval
}

type visitor struct {
	tokens   int
	lastSeen time.Time
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(rate int, interval time.Duration) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     rate,
		interval: interval,
	}

	// Cleanup old visitors every minute
	go func() {
		for {
			time.Sleep(time.Minute)
			rl.cleanup()
		}
	}()

	return rl
}

// cleanup removes old visitors
func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	for ip, v := range rl.visitors {
		if time.Since(v.lastSeen) > rl.interval*2 {
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

	v, exists := rl.visitors[ip]
	if !exists {
		rl.visitors[ip] = &visitor{
			tokens:   rl.rate - 1,
			lastSeen: time.Now(),
		}
		return true
	}

	// Refill tokens based on time elapsed
	elapsed := time.Since(v.lastSeen)
	tokensToAdd := int(elapsed.Seconds() * float64(rl.rate) / rl.interval.Seconds())
	v.tokens += tokensToAdd
	if v.tokens > rl.rate {
		v.tokens = rl.rate
	}
	v.lastSeen = time.Now()

	if v.tokens <= 0 {
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
