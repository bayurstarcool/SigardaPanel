package apihttp

import (
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newTestLimiter(rate int, interval time.Duration) (*RateLimiter, *fakeClock) {
	clk := &fakeClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	rl := newRateLimiterNoCleanup(rate, interval)
	rl.now = clk.now
	return rl, clk
}

func TestRateLimiter_BurstThenDeny(t *testing.T) {
	t.Setenv("SIGARDAPANEL_DEV", "")
	rl, _ := newTestLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		if !rl.Allow("1.2.3.4") {
			t.Fatalf("request %d denied within burst", i)
		}
	}
	if rl.Allow("1.2.3.4") {
		t.Fatal("4th request allowed; bucket should be empty")
	}
	if !rl.Allow("5.6.7.8") {
		t.Fatal("other IP must have its own bucket")
	}
}

// Regression: 10 per 15 min means one token every 90s. A client retrying every
// 60s used to be locked out forever because each call truncated the refill to
// 0 and reset the refill clock.
func TestRateLimiter_SlowRateRefills(t *testing.T) {
	t.Setenv("SIGARDAPANEL_DEV", "")
	rl, clk := newTestLimiter(10, 15*time.Minute)
	for i := 0; i < 10; i++ {
		rl.Allow("ip")
	}
	if rl.Allow("ip") {
		t.Fatal("expected bucket to be empty")
	}
	allowed := 0
	for i := 0; i < 6; i++ { // 6 minutes at one attempt per minute
		clk.advance(time.Minute)
		if rl.Allow("ip") {
			allowed++
		}
	}
	// 6 minutes refill 4 tokens (one per 90s).
	if allowed != 4 {
		t.Fatalf("allowed %d requests after 6 minutes, want 4", allowed)
	}
}

func TestRateLimiter_RefillCapsAtRate(t *testing.T) {
	t.Setenv("SIGARDAPANEL_DEV", "")
	rl, clk := newTestLimiter(5, time.Minute)
	rl.Allow("ip")
	clk.advance(time.Hour)
	n := 0
	for rl.Allow("ip") {
		n++
		if n > 100 {
			break
		}
	}
	if n != 5 {
		t.Fatalf("allowed %d after long idle, want capacity 5", n)
	}
}
