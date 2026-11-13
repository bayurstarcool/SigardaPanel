package apihttp

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v5"

	"sigardapanel/internal/store"
)

// Per-user disk quota (users.disk_limit, in GB; NULL or 0 = unlimited).
//
// A user's usage is the summed disk usage of the sites they own (sites.owner_id),
// measured on each site's server (see disk_usage.go). Sites a user is only
// assigned to count against their owner. The panel enforces the limit by
// refusing actions that add data to a site of an owner who is at or over it,
// and a background check notifies the owner and the admins at 80% and 100%.
// Writes outside the panel (SSH/FTP, cron, the site's own PHP) are not blocked,
// they are caught by the background check.

const (
	diskQuotaCheckWait = 10 * time.Second // how long an action waits for measurements
	diskQuotaInterval  = 5 * time.Minute
	diskQuotaWarnPct   = 80
	maxOwnedSites      = 10000
)

// diskLimitBytes returns the user's disk limit in bytes, 0 when unlimited.
func diskLimitBytes(u *store.User) int64 {
	if u == nil || !u.DiskLimit.Valid || u.DiskLimit.Int64 <= 0 {
		return 0
	}
	return u.DiskLimit.Int64 * 1024 * 1024 * 1024
}

// ownedDiskUsage measures the sites userID owns.
func ownedDiskUsage(ctx context.Context, st *store.Store, userID int64, wait time.Duration) (siteDiskTotal, int, error) {
	sites, err := st.Sites.List(ctx, store.SiteListFilter{OwnerID: userID, Limit: maxOwnedSites})
	if err != nil {
		return siteDiskTotal{}, 0, err
	}
	return siteDisk.total(ctx, st, sites, wait), len(sites), nil
}

// checkDiskQuota refuses (403) an action that adds extra bytes to a site owned
// by ownerID when the owner is at their disk limit or the action would take
// them over it. Sites that cannot be measured (server offline) do not count:
// an unreachable agent must not lock users out.
func checkDiskQuota(ctx context.Context, st *store.Store, ownerID, extra int64) error {
	if ownerID <= 0 {
		return nil
	}
	owner, err := st.Users.GetByID(ctx, ownerID)
	if err != nil {
		slog.Error("disk quota: owner lookup failed", "user_id", ownerID, "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	limit := diskLimitBytes(owner)
	if limit == 0 {
		return nil
	}
	used, _, err := ownedDiskUsage(ctx, st, ownerID, diskQuotaCheckWait)
	if err != nil {
		slog.Error("disk quota: measuring failed", "user_id", ownerID, "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if used.Bytes >= limit || (extra > 0 && used.Bytes+extra > limit) {
		msg := fmt.Sprintf("disk limit reached: %s of %s used", humanBytes(used.Bytes), humanBytes(limit))
		if extra > 0 && used.Bytes < limit {
			msg = fmt.Sprintf("not enough disk space: %s of %s used, %s needed", humanBytes(used.Bytes), humanBytes(limit), humanBytes(extra))
		}
		msg += " by " + owner.Username + "'s sites. Free up space or ask an administrator to raise the limit"
		return echo.NewHTTPError(http.StatusForbidden, msg)
	}
	return nil
}

// checkSiteDiskQuota is checkDiskQuota for the owner of site.
func checkSiteDiskQuota(ctx context.Context, st *store.Store, site *store.Site, extra int64) error {
	if site == nil || !site.OwnerID.Valid {
		return nil
	}
	return checkDiskQuota(ctx, st, site.OwnerID.Int64, extra)
}

func humanBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

// diskQuotaNotifier remembers the level each user was last notified at
// (0 = below 80%, 1 = 80%+, 2 = 100%+) so each level is announced once until
// usage drops below it again.
type diskQuotaNotifier struct {
	mu    sync.Mutex
	level map[int64]int
}

func diskQuotaLevel(used, limit int64) int {
	switch {
	case limit <= 0:
		return 0
	case used >= limit:
		return 2
	case used*100 >= limit*diskQuotaWarnPct:
		return 1
	}
	return 0
}

// check measures every user with a disk limit and notifies on a new level.
func (n *diskQuotaNotifier) check(ctx context.Context, st *store.Store) {
	users, err := st.Users.List(ctx, store.UserListFilter{})
	if err != nil {
		slog.Warn("disk quota check: listing users failed", "error", err)
		return
	}
	for i := range users {
		u := &users[i]
		limit := diskLimitBytes(u)
		level := 0
		var used siteDiskTotal
		if limit > 0 {
			used, _, err = ownedDiskUsage(ctx, st, u.ID, time.Minute)
			if err != nil {
				continue
			}
			level = diskQuotaLevel(used.Bytes, limit)
		}
		n.mu.Lock()
		prev := n.level[u.ID]
		n.level[u.ID] = level
		n.mu.Unlock()
		if level <= prev {
			continue
		}
		pct := used.Bytes * 100 / limit
		title := fmt.Sprintf("Disk usage at %d%%", pct)
		severity := "warning"
		msg := fmt.Sprintf("%s's sites use %s of the %s disk limit.", u.Username, humanBytes(used.Bytes), humanBytes(limit))
		if level == 2 {
			title = "Disk limit reached"
			severity = "error"
			msg += " New sites, uploads, file edits, deploys and restores are blocked until space is freed or the limit is raised."
		}
		st.Notify(ctx, u.ID, "disk_quota", title, msg, severity, "user", u.ID)
		st.NotifyAdmins(ctx, "disk_quota", title+": "+u.Username, msg, severity, "user", u.ID)
	}
}

// StartDiskQuotaChecker checks users' disk usage against their limits every
// few minutes, notifying at 80% and 100%.
func StartDiskQuotaChecker(ctx context.Context, st *store.Store) {
	n := &diskQuotaNotifier{level: map[int64]int{}}
	go func() {
		t := time.NewTimer(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				n.check(ctx, st)
				t.Reset(diskQuotaInterval)
			}
		}
	}()
}
