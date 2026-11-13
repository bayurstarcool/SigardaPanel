// Package selfupdate finds the latest SigardaPanel release and compares
// release versions. Installing a release is done by the agent with the
// embedded installer (installer.Script), which verifies its signature.
package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Repo is the public repository releases are published to (scripts/release.sh).
const Repo = "bayurstarcool/SigardaPanel"

// tagRe matches the tags scripts/release.sh creates (vX.Y.Z).
var tagRe = regexp.MustCompile(`^v(0|[1-9][0-9]{0,5})\.(0|[1-9][0-9]{0,5})\.(0|[1-9][0-9]{0,5})$`)

// ValidTag reports whether tag is a release tag such as "v1.6.2".
func ValidTag(tag string) bool { return tagRe.MatchString(tag) }

// parse turns "1.6.2" or "v1.6.2" into its numbers.
func parse(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimSpace(v)
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	m := tagRe.FindStringSubmatch(v)
	if m == nil {
		return out, false
	}
	for i := 0; i < 3; i++ {
		out[i], _ = strconv.Atoi(m[i+1])
	}
	return out, true
}

// IsRelease reports whether version is a release version (not "dev" or a
// locally built binary). Only release builds can update themselves.
func IsRelease(version string) bool {
	_, ok := parse(version)
	return ok
}

// Newer reports whether candidate is a strictly newer release than current.
// It is false whenever either side is not a release version.
func Newer(candidate, current string) bool {
	a, ok1 := parse(candidate)
	b, ok2 := parse(current)
	if !ok1 || !ok2 {
		return false
	}
	for i := 0; i < 3; i++ {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}

// Tag returns version as a "vX.Y.Z" tag.
func Tag(version string) string {
	if strings.HasPrefix(version, "v") {
		return version
	}
	return "v" + version
}

// Release describes a published release.
type Release struct {
	Tag         string    `json:"tag"`
	Name        string    `json:"name"`
	Notes       string    `json:"notes"`
	URL         string    `json:"url"`
	PublishedAt time.Time `json:"published_at"`
}

// Checker looks up the latest release on GitHub and caches the answer, so
// page loads and the background updater never hammer the GitHub API
// (60 unauthenticated requests per hour per IP).
type Checker struct {
	HTTP    *http.Client
	APIBase string // default https://api.github.com
	Repo    string // default Repo
	// MinInterval is the shortest time between two real lookups, even when
	// a caller asks for a fresh answer (default 60s).
	MinInterval time.Duration
	// ErrorBackoff is how long a failed lookup is remembered before the next
	// caller tries again (default 5m), whatever maxAge that caller passes:
	// one GitHub hiccup must not hide new releases for hours.
	ErrorBackoff time.Duration

	mu        sync.Mutex
	fetching  chan struct{} // non-nil while a lookup runs
	latest    *Release
	lastErr   error
	attemptAt time.Time // last lookup, successful or not
	successAt time.Time // last successful lookup
}

// NewChecker returns a Checker with production defaults.
func NewChecker() *Checker {
	return &Checker{HTTP: &http.Client{Timeout: 15 * time.Second}}
}

// Cached returns the last known release, when it was last fetched
// successfully and the error of the last attempt (nil when it succeeded),
// without doing a lookup.
func (c *Checker) Cached() (*Release, time.Time, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.latest, c.successAt, c.lastErr
}

func (c *Checker) fresh(maxAge time.Duration) bool {
	minInterval := c.MinInterval
	if minInterval <= 0 {
		minInterval = time.Minute
	}
	if maxAge < minInterval {
		maxAge = minInterval
	}
	if c.attemptAt.IsZero() {
		return false
	}
	if c.lastErr != nil {
		backoff := c.ErrorBackoff
		if backoff <= 0 {
			backoff = 5 * time.Minute
		}
		if backoff < minInterval {
			backoff = minInterval
		}
		if maxAge > backoff {
			maxAge = backoff
		}
	}
	return time.Since(c.attemptAt) < maxAge
}

// Latest returns the latest known release, the time it was last fetched
// successfully, and the error of the last attempt. It looks the release up
// when the cached answer is older than maxAge (0 asks for a fresh lookup,
// still bounded by MinInterval; a failed lookup is retried after
// ErrorBackoff). Concurrent callers share one lookup, and a lookup aborted
// because the caller's context ended is not remembered as an error.
func (c *Checker) Latest(ctx context.Context, maxAge time.Duration) (*Release, time.Time, error) {
	for {
		c.mu.Lock()
		if c.fresh(maxAge) {
			defer c.mu.Unlock()
			return c.latest, c.successAt, c.lastErr
		}
		if wait := c.fetching; wait != nil {
			c.mu.Unlock()
			select {
			case <-wait:
				continue // the other lookup finished; use its answer
			case <-ctx.Done():
				c.mu.Lock()
				defer c.mu.Unlock()
				return c.latest, c.successAt, ctx.Err()
			}
		}
		done := make(chan struct{})
		c.fetching = done
		c.mu.Unlock()

		rel, err := c.fetch(ctx)

		c.mu.Lock()
		c.fetching = nil
		close(done)
		if err != nil && ctx.Err() != nil {
			// The caller gave up; do not cache that as GitHub's answer.
			defer c.mu.Unlock()
			return c.latest, c.successAt, err
		}
		c.attemptAt = time.Now()
		c.lastErr = err
		if err == nil {
			c.latest = rel
			c.successAt = c.attemptAt
		}
		defer c.mu.Unlock()
		return c.latest, c.successAt, c.lastErr
	}
}

func (c *Checker) fetch(ctx context.Context) (*Release, error) {
	base := strings.TrimRight(c.APIBase, "/")
	if base == "" {
		base = "https://api.github.com"
	}
	repo := c.Repo
	if repo == "" {
		repo = Repo
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/repos/"+repo+"/releases/latest", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "sigardapanel-updater")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach GitHub: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub answered HTTP %d", resp.StatusCode)
	}
	var body struct {
		TagName     string    `json:"tag_name"`
		Name        string    `json:"name"`
		Body        string    `json:"body"`
		HTMLURL     string    `json:"html_url"`
		PublishedAt time.Time `json:"published_at"`
		Draft       bool      `json:"draft"`
		Prerelease  bool      `json:"prerelease"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return nil, fmt.Errorf("invalid GitHub response: %w", err)
	}
	if body.Draft || body.Prerelease || !ValidTag(body.TagName) {
		return nil, errors.New("the latest GitHub release is not a usable release tag")
	}
	notes := body.Body
	if len(notes) > 20000 {
		notes = notes[:20000]
	}
	return &Release{Tag: body.TagName, Name: body.Name, Notes: notes, URL: body.HTMLURL, PublishedAt: body.PublishedAt}, nil
}
