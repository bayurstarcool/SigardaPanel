package apihttp

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"sigardapanel/internal/store"
)

func TestSiteDiskTotal(t *testing.T) {
	c := newSiteDiskCache()
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	c.now = func() time.Time { mu.Lock(); defer mu.Unlock(); return clock }
	advance := func(d time.Duration) { mu.Lock(); clock = clock.Add(d); mu.Unlock() }

	var calls atomic.Int32
	sizes := map[int64]int64{1: 1000, 2: 2500}
	failing := map[int64]bool{3: true}
	c.measure = func(_ context.Context, _ *store.Store, s store.Site) (int64, error) {
		calls.Add(1)
		if failing[s.ID] {
			return 0, errors.New("agent offline")
		}
		return sizes[s.ID], nil
	}
	sites := []store.Site{
		{ID: 1, RootPath: "/home/a/htdocs/a.test"},
		{ID: 2, RootPath: "/home/b/htdocs/b.test"},
		{ID: 3, RootPath: "/home/c/htdocs/c.test"},
		{ID: 1, RootPath: "/home/a/htdocs/a.test"}, // duplicate: counted once
		{ID: 4}, // no root path: skipped
	}

	got := c.total(context.Background(), nil, sites, 5*time.Second)
	if got.Bytes != 3500 || got.Measured != 2 || got.Unmeasured != 1 {
		t.Fatalf("first total = %+v, want 3500 bytes, 2 measured, 1 unmeasured", got)
	}
	if calls.Load() != 3 {
		t.Fatalf("measure calls = %d, want 3", calls.Load())
	}

	// Fresh entries (and the recent failure) are not measured again.
	got = c.total(context.Background(), nil, sites, 5*time.Second)
	if got.Bytes != 3500 || calls.Load() != 3 {
		t.Fatalf("cached total = %+v after %d calls, want 3500 and no new calls", got, calls.Load())
	}

	// After the failure retry window the offline site is retried.
	failing[3] = false
	sizes[3] = 500
	advance(siteDiskRetryAfter + time.Second)
	got = c.total(context.Background(), nil, sites, 5*time.Second)
	if got.Bytes != 4000 || got.Unmeasured != 0 || calls.Load() != 4 {
		t.Fatalf("after retry = %+v (%d calls), want 4000 bytes, none unmeasured, 4 calls", got, calls.Load())
	}

	// A value set by the per-site endpoint is used as is.
	c.store(2, 9000)
	got = c.total(context.Background(), nil, sites, 5*time.Second)
	if got.Bytes != 1000+9000+500 {
		t.Fatalf("after store = %+v, want 10500", got)
	}
}

func TestSiteDiskTotalStaleReturnsWithoutWaiting(t *testing.T) {
	c := newSiteDiskCache()
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	c.now = func() time.Time { mu.Lock(); defer mu.Unlock(); return clock }
	release := make(chan struct{})
	var calls atomic.Int32
	c.measure = func(context.Context, *store.Store, store.Site) (int64, error) {
		if calls.Add(1) > 1 {
			<-release // the refresh hangs (slow du)
			return 7, nil
		}
		return 5, nil
	}
	sites := []store.Site{{ID: 1, RootPath: "/home/a/htdocs/a.test"}}
	if got := c.total(context.Background(), nil, sites, time.Second); got.Bytes != 5 {
		t.Fatalf("first = %+v", got)
	}
	mu.Lock()
	clock = clock.Add(siteDiskFreshFor + time.Second)
	mu.Unlock()

	start := time.Now()
	got := c.total(context.Background(), nil, sites, 3*time.Second)
	if time.Since(start) > time.Second {
		t.Fatalf("stale total waited for the refresh")
	}
	if got.Bytes != 5 || got.Measured != 1 {
		t.Fatalf("stale total = %+v, want the previous 5 bytes", got)
	}
	close(release)
}

func TestSiteDiskTotalSlowSiteIsUnmeasured(t *testing.T) {
	c := newSiteDiskCache()
	release := make(chan struct{})
	defer close(release)
	c.measure = func(_ context.Context, _ *store.Store, s store.Site) (int64, error) {
		if s.ID == 2 {
			<-release
		}
		return 100, nil
	}
	sites := []store.Site{{ID: 1, RootPath: "/x/1"}, {ID: 2, RootPath: "/x/2"}}
	got := c.total(context.Background(), nil, sites, 200*time.Millisecond)
	if got.Bytes != 100 || got.Measured != 1 || got.Unmeasured != 1 {
		t.Fatalf("total = %+v, want 100 bytes with the slow site unmeasured", got)
	}
}
