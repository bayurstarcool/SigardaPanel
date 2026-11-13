package member

import (
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// validPlans are the plan names a member may choose.
var validPlans = map[string]bool{"community": true, "pro": true, "enterprise": true}

// dummyHash keeps a login for an unknown email as slow as a real one.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("sigardapanel-member-dummy"), bcrypt.DefaultCost)

// failureWindow counts failed member logins per key over 15 minutes.
type failureWindow struct {
	mu    sync.Mutex
	fails map[string][]time.Time
}

var loginFailures = &failureWindow{fails: map[string][]time.Time{}}

func (f *failureWindow) recent(key string, now time.Time) []time.Time {
	var keep []time.Time
	for _, t := range f.fails[key] {
		if now.Sub(t) < 15*time.Minute {
			keep = append(keep, t)
		}
	}
	if len(keep) == 0 {
		delete(f.fails, key)
	} else {
		f.fails[key] = keep
	}
	return keep
}

func (f *failureWindow) blocked(key string, limit int) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.recent(key, time.Now())) >= limit
}

func (f *failureWindow) fail(keys ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	for _, k := range keys {
		f.fails[k] = append(f.recent(k, now), now)
	}
}

func (f *failureWindow) reset(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.fails, key)
}
