package apihttp

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"sigardapanel/internal/store"
)

// Disk usage of a site is measured by `du` on the site's own server, through
// its agent: the API process is sandboxed (ProtectHome) and a site may live on
// another server, so a local du always saw nothing.
//
// du over a large tree is slow, and the sidebar asks for a user's total on
// every page, so results are cached per site and measurements are bounded:
// at most siteDiskWorkers run at once across the panel, one per site at a time.

const (
	siteDiskFreshFor   = 5 * time.Minute  // a good measurement is reused this long
	siteDiskRetryAfter = time.Minute      // a failed one is retried after this
	siteDiskTimeout    = 2 * time.Minute  // one du on the agent
	siteDiskWorkers    = 8                // concurrent measurements, panel-wide
	siteDiskWait       = 20 * time.Second // how long a request waits for results
)

type siteDiskEntry struct {
	bytes int64
	ok    bool // the last measurement succeeded; bytes is only valid then
	at    time.Time
}

type siteDiskCache struct {
	mu       sync.Mutex
	entries  map[int64]siteDiskEntry
	inflight map[int64]chan struct{}
	// gen changes when a site's value is forgotten, so a measurement started
	// before that (e.g. before files were deleted) does not store its result.
	gen map[int64]uint64
	sem chan struct{}
	// measure is the agent call; tests replace it.
	measure func(ctx context.Context, st *store.Store, site store.Site) (int64, error)
	now     func() time.Time
}

var siteDisk = newSiteDiskCache()

func newSiteDiskCache() *siteDiskCache {
	return &siteDiskCache{
		entries:  map[int64]siteDiskEntry{},
		inflight: map[int64]chan struct{}{},
		gen:      map[int64]uint64{},
		sem:      make(chan struct{}, siteDiskWorkers),
		measure:  measureSiteDiskOnAgent,
		now:      time.Now,
	}
}

// measureSiteDiskOnAgent asks the site's agent for `du -sb root_path`.
func measureSiteDiskOnAgent(ctx context.Context, st *store.Store, site store.Site) (int64, error) {
	h := &siteHandler{store: st}
	code, out, err := h.siteAgentCall(ctx, &site, http.MethodPost, "/tasks/site/usage", map[string]any{"root_path": site.RootPath}, siteDiskTimeout)
	if err != nil {
		return 0, err
	}
	if code >= 400 {
		return 0, fmt.Errorf("agent answered %d", code)
	}
	v, ok := out["disk_used_bytes"].(float64)
	if !ok || v < 0 {
		return 0, fmt.Errorf("agent returned no disk_used_bytes")
	}
	return int64(v), nil
}

// store records a measurement made elsewhere (the per-site usage endpoint).
func (c *siteDiskCache) store(siteID, bytes int64) {
	c.mu.Lock()
	c.entries[siteID] = siteDiskEntry{bytes: bytes, ok: true, at: c.now()}
	c.mu.Unlock()
}

// grew records data a panel action added to a site: n known bytes are added to
// the cached value at once (so a run of uploads cannot slip past the quota
// between measurements), and the entry is marked for re-measuring.
func (c *siteDiskCache) grew(siteID, n int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[siteID]
	if !ok || !e.ok {
		return
	}
	if n > 0 {
		e.bytes += n
	}
	e.at = time.Time{} // stale: the next read refreshes it
	c.entries[siteID] = e
}

// forget drops the cached value of a site (after data was removed).
func (c *siteDiskCache) forget(siteID int64) {
	c.mu.Lock()
	delete(c.entries, siteID)
	delete(c.inflight, siteID)
	c.gen[siteID]++
	c.mu.Unlock()
}

// start returns a channel closed when the site's measurement is done, starting
// one unless the cached entry is still fresh (then it returns nil).
func (c *siteDiskCache) start(st *store.Store, site store.Site) <-chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.entries[site.ID]; ok {
		age := c.now().Sub(e.at)
		if (e.ok && age < siteDiskFreshFor) || (!e.ok && age < siteDiskRetryAfter) {
			return nil
		}
	}
	if ch, ok := c.inflight[site.ID]; ok {
		return ch
	}
	ch := make(chan struct{})
	c.inflight[site.ID] = ch
	gen := c.gen[site.ID]
	go func() {
		defer close(ch)
		c.sem <- struct{}{}
		// Detached from the request: a finished measurement fills the cache
		// even when the page that asked for it has gone.
		ctx, cancel := context.WithTimeout(context.Background(), siteDiskTimeout+10*time.Second)
		size, err := c.measure(ctx, st, site)
		cancel()
		<-c.sem
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.gen[site.ID] != gen {
			return // forgotten meanwhile: the result may predate the change
		}
		delete(c.inflight, site.ID)
		if err != nil {
			c.entries[site.ID] = siteDiskEntry{at: c.now()}
			return
		}
		c.entries[site.ID] = siteDiskEntry{bytes: size, ok: true, at: c.now()}
	}()
	return ch
}

func (c *siteDiskCache) hasValue(siteID int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.entries[siteID].ok
}

// siteDiskTotal is the summed disk usage of sites, as measured on their servers.
type siteDiskTotal struct {
	Bytes int64
	// Measured sites have a value in Bytes; Unmeasured ones (agent offline,
	// du failed or still running) are not in it, so Bytes is then a lower bound.
	Measured   int
	Unmeasured int
	// MeasuredAt is the oldest measurement in Bytes.
	MeasuredAt time.Time
}

// total measures sites (cached), waiting at most until ctx ends or wait passes.
func (c *siteDiskCache) total(ctx context.Context, st *store.Store, sites []store.Site, wait time.Duration) siteDiskTotal {
	waitCtx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	seen := make(map[int64]bool, len(sites))
	var pending []<-chan struct{}
	var measure []store.Site
	for _, s := range sites {
		if s.RootPath == "" || seen[s.ID] {
			continue
		}
		seen[s.ID] = true
		measure = append(measure, s)
		if ch := c.start(st, s); ch != nil && !c.hasValue(s.ID) {
			// Wait only for sites with no value yet; a stale value is
			// returned now and refreshed in the background.
			pending = append(pending, ch)
		}
	}
	for _, ch := range pending {
		select {
		case <-ch:
		case <-waitCtx.Done():
		}
	}
	var t siteDiskTotal
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range measure {
		// A stale value being re-measured still counts; a failed measurement
		// does not (the site is reported as unmeasured, not as 0 bytes).
		e, ok := c.entries[s.ID]
		if !ok || !e.ok {
			t.Unmeasured++
			continue
		}
		t.Measured++
		t.Bytes += e.bytes
		if !e.at.IsZero() && (t.MeasuredAt.IsZero() || e.at.Before(t.MeasuredAt)) {
			t.MeasuredAt = e.at
		}
	}
	return t
}
