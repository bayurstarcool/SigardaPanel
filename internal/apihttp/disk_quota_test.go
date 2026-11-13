package apihttp

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"sigardapanel/internal/auth"
	"sigardapanel/internal/store"
)

const gib = int64(1024 * 1024 * 1024)

// withFakeSiteDisk swaps the panel-wide cache for one whose measurements come
// from sizes (site ID -> bytes; missing = agent offline).
func withFakeSiteDisk(t *testing.T, sizes map[int64]int64) {
	t.Helper()
	prev := siteDisk
	c := newSiteDiskCache()
	c.measure = func(_ context.Context, _ *store.Store, s store.Site) (int64, error) {
		if v, ok := sizes[s.ID]; ok {
			return v, nil
		}
		return 0, errors.New("agent offline")
	}
	siteDisk = c
	t.Cleanup(func() { siteDisk = prev })
}

func quotaFixture(t *testing.T, s *store.Store, ctx context.Context, limitGB int64) (*store.User, []store.Site) {
	t.Helper()
	u := &store.User{Username: "quota", Email: "quota@example.com", PasswordHash: "x", Role: "user"}
	if limitGB > 0 {
		u.DiskLimit = sql.NullInt64{Int64: limitGB, Valid: true}
	}
	// Managed by the test admin, when there is one (canAccess, callerManagesUser).
	if admin, _ := s.Users.GetByEmail(ctx, "admin@example.com"); admin != nil {
		u.CreatedBy = sql.NullInt64{Int64: admin.ID, Valid: true}
	}
	if err := s.Users.Create(ctx, u); err != nil {
		t.Fatalf("create user: %v", err)
	}
	srv := createServer(t, s, ctx)
	var sites []store.Site
	for i := 0; i < 2; i++ {
		site := &store.Site{ServerID: srv, Name: "s", Domain: fmt.Sprintf("q%d.example.com", i), Runtime: "static",
			RootPath: fmt.Sprintf("/home/quota/htdocs/q%d.example.com", i), OwnerID: nullInt(u.ID)}
		if err := s.Sites.Create(ctx, site); err != nil {
			t.Fatalf("create site: %v", err)
		}
		sites = append(sites, *site)
	}
	return u, sites
}

func sessionTokenFor(t *testing.T, s *store.Store, ctx context.Context, userID int64) string {
	t.Helper()
	token, err := auth.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Sessions.Create(ctx, &store.Session{UserID: userID, TokenHash: auth.HashToken(token), ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	return token
}

func quotaStatus(err error) int {
	var he *echo.HTTPError
	if errors.As(err, &he) {
		return he.Code
	}
	if err != nil {
		return -1
	}
	return 0
}

func TestCheckDiskQuota(t *testing.T) {
	_, s, ctx := newTestAppWithAllMigrations(t)
	u, sites := quotaFixture(t, s, ctx, 1)
	sizes := map[int64]int64{sites[0].ID: 300 << 20, sites[1].ID: 400 << 20} // 700 MiB of 1 GiB
	withFakeSiteDisk(t, sizes)

	if err := checkDiskQuota(ctx, s, u.ID, 0); err != nil {
		t.Fatalf("under the limit: %v", err)
	}
	if err := checkDiskQuota(ctx, s, u.ID, 200<<20); err != nil {
		t.Fatalf("fits: %v", err)
	}
	err := checkDiskQuota(ctx, s, u.ID, 400<<20)
	if quotaStatus(err) != http.StatusForbidden || !strings.Contains(err.Error(), "not enough disk space") {
		t.Fatalf("would exceed: got %v", err)
	}

	// At the limit: everything that adds data is refused.
	siteDisk.store(sites[1].ID, gib-(300<<20))
	if err := checkDiskQuota(ctx, s, u.ID, 0); quotaStatus(err) != http.StatusForbidden || !strings.Contains(err.Error(), "disk limit reached") {
		t.Fatalf("at the limit: got %v", err)
	}

	// Deleting files frees space at once (cache forgotten, re-measured).
	sizes[sites[1].ID] = 10 << 20
	siteDisk.forget(sites[1].ID)
	if err := checkDiskQuota(ctx, s, u.ID, 0); err != nil {
		t.Fatalf("after freeing space: %v", err)
	}
}

func TestCheckDiskQuotaUnlimitedAndOffline(t *testing.T) {
	_, s, ctx := newTestAppWithAllMigrations(t)
	u, sites := quotaFixture(t, s, ctx, 0)
	withFakeSiteDisk(t, map[int64]int64{sites[0].ID: 50 * gib})
	if err := checkDiskQuota(ctx, s, u.ID, 10*gib); err != nil {
		t.Fatalf("unlimited user refused: %v", err)
	}

	// Limited user whose servers are all offline: not locked out.
	_, s2, ctx2 := newTestAppWithAllMigrations(t)
	u2, _ := quotaFixture(t, s2, ctx2, 1)
	withFakeSiteDisk(t, map[int64]int64{})
	if err := checkDiskQuota(ctx2, s2, u2.ID, 0); err != nil {
		t.Fatalf("offline servers locked the user out: %v", err)
	}
}

func TestUploadGrowthCountsBeforeRemeasure(t *testing.T) {
	_, s, ctx := newTestAppWithAllMigrations(t)
	u, sites := quotaFixture(t, s, ctx, 1)
	withFakeSiteDisk(t, map[int64]int64{sites[0].ID: 0, sites[1].ID: 0})
	if err := checkDiskQuota(ctx, s, u.ID, 600<<20); err != nil {
		t.Fatalf("first upload: %v", err)
	}
	siteDisk.grew(sites[0].ID, 600<<20) // the agent still reports 0 until re-measured
	if err := checkDiskQuota(ctx, s, u.ID, 600<<20); quotaStatus(err) != http.StatusForbidden {
		t.Fatalf("second upload should exceed the limit, got %v", err)
	}
}

func TestFileWriteRefusedOverQuota(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	u, sites := quotaFixture(t, s, ctx, 1)
	token := sessionTokenFor(t, s, ctx, u.ID) // the owner
	withFakeSiteDisk(t, map[int64]int64{sites[0].ID: gib, sites[1].ID: 0})

	rec := doRequest(t, e, "POST", fmt.Sprintf("/api/v1/sites/%d/files/write", sites[1].ID),
		map[string]any{"path": "index.html", "content": "hi"}, token)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "disk limit reached") {
		t.Fatalf("write over quota: %d %s", rec.Code, rec.Body.String())
	}
}

func TestUserUsageCountsOwnedSites(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)
	u, sites := quotaFixture(t, s, ctx, 1)
	withFakeSiteDisk(t, map[int64]int64{sites[0].ID: 100, sites[1].ID: 200})

	rec := doRequest(t, e, "GET", fmt.Sprintf("/api/v1/users/%d/usage", u.ID), nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("usage: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Data struct {
			DiskUsed   int64 `json:"disk_used_bytes"`
			DiskLimit  int64 `json:"disk_limit_bytes"`
			SiteCount  int   `json:"site_count"`
			Unmeasured int   `json:"disk_unmeasured_sites"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Data.DiskUsed != 300 || out.Data.DiskLimit != gib || out.Data.SiteCount != 2 || out.Data.Unmeasured != 0 {
		t.Fatalf("usage = %+v", out.Data)
	}
}

func TestDiskQuotaNotifier(t *testing.T) {
	_, s, ctx := newTestAppWithAllMigrations(t)
	u, sites := quotaFixture(t, s, ctx, 1)
	sizes := map[int64]int64{sites[0].ID: 500 << 20, sites[1].ID: 400 << 20} // ~88%
	withFakeSiteDisk(t, sizes)
	n := &diskQuotaNotifier{level: map[int64]int{}}

	count := func() int {
		_, total, err := s.Notifications.ListByUser(ctx, u.ID, 100, 0)
		if err != nil {
			t.Fatal(err)
		}
		return total
	}
	n.check(ctx, s)
	if c := count(); c != 1 {
		t.Fatalf("after 80%%: %d notifications, want 1", c)
	}
	n.check(ctx, s) // same level: no repeat
	if c := count(); c != 1 {
		t.Fatalf("repeat: %d notifications, want 1", c)
	}
	siteDisk.store(sites[1].ID, gib)
	n.check(ctx, s)
	if c := count(); c != 2 {
		t.Fatalf("after 100%%: %d notifications, want 2", c)
	}
}

func TestUserUsageAdminWithoutLimitCoversAllSites(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	_, sites := quotaFixture(t, s, ctx, 1) // sites owned by another user
	admin := &store.User{Username: "boss", Email: "boss@example.com", PasswordHash: "x", Role: "super_admin"}
	if err := s.Users.Create(ctx, admin); err != nil {
		t.Fatal(err)
	}
	if err := s.Metrics.Insert(ctx, &store.ServerMetric{ServerID: sites[0].ServerID, CollectedAt: time.Now(),
		DiskUsed: sql.NullInt64{Int64: 10 * gib, Valid: true}, DiskTotal: sql.NullInt64{Int64: 100 * gib, Valid: true}}); err != nil {
		t.Fatal(err)
	}
	withFakeSiteDisk(t, map[int64]int64{sites[0].ID: 100, sites[1].ID: 200})

	rec := doRequest(t, e, "GET", fmt.Sprintf("/api/v1/users/%d/usage", admin.ID), nil, sessionTokenFor(t, s, ctx, admin.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("usage: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Data struct {
			DiskUsed  int64  `json:"disk_used_bytes"`
			Capacity  int64  `json:"disk_capacity_bytes"`
			Scope     string `json:"disk_scope"`
			SiteCount int    `json:"site_count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Data.Scope != "all" || out.Data.DiskUsed != 300 || out.Data.Capacity != 100*gib || out.Data.SiteCount != 2 {
		t.Fatalf("admin usage = %+v", out.Data)
	}
}
