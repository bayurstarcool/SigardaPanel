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

	mu        sync.Mutex
	latest    *Release
	lastErr   error
	checkedAt time.Time
}

// NewChecker returns a Checker with production defaults.
func NewChecker() *Checker {
	return &Checker{HTTP: &http.Client{Timeout: 20 * time.Second}}
}

// Cached returns the last answer without doing a lookup.
func (c *Checker) Cached() (*Release, time.Time, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.latest, c.checkedAt, c.lastErr
}

// Latest returns the latest release, looking it up when the cached answer is
// older than maxAge (maxAge 0 asks for a fresh lookup, still bounded by
// MinInterval).
func (c *Checker) Latest(ctx context.Context, maxAge time.Duration) (*Release, time.Time, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	minInterval := c.MinInterval
	if minInterval <= 0 {
		minInterval = time.Minute
	}
	if maxAge < minInterval {
		maxAge = minInterval
	}
	if !c.checkedAt.IsZero() && time.Since(c.checkedAt) < maxAge {
		return c.latest, c.checkedAt, c.lastErr
	}
	rel, err := c.fetch(ctx)
	c.checkedAt = time.Now()
	c.lastErr = err
	if err == nil {
		c.latest = rel
	}
	return c.latest, c.checkedAt, err
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
		client = &http.Client{Timeout: 20 * time.Second}
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
