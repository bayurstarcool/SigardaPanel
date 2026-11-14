package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"sigardapanel/internal/buildinfo"
	"sigardapanel/internal/jobmodel"
	"sigardapanel/internal/store"
)

// Per-site PHP-FPM pools for sites created before agents had them.
//
// A PHP site moves to its own pool on its next vhost regeneration. Sites that
// nobody touches are moved here: the scheduler queues one vhost regeneration
// (site.create with vhost_regen) per active PHP site whose server runs an
// agent of this panel's version and that has no php_pool_site_<id> setting
// yet. The worker records the setting from the agent's answer ("1" own pool,
// "0" shared pool kept because the host has no manageable pool directory), so
// each site is moved once. A failed regeneration is retried after 6 hours.

const phpPoolMigrateBatch = 20

func phpPoolSettingKey(siteID int64) string {
	return "php_pool_site_" + strconv.FormatInt(siteID, 10)
}

func phpPoolTriedKey(siteID int64) string {
	return "php_pool_tried_" + strconv.FormatInt(siteID, 10)
}

// agentKnowsPools: the server's agent reports this panel's release (dev
// panels move dev agents).
func agentKnowsPools(srv *store.Server) bool {
	return srv != nil && srv.AgentVersion.Valid && srv.AgentVersion.String != "" &&
		srv.AgentVersion.String == buildinfo.Version
}

func migratePHPPools(ctx context.Context, s *store.Store, now time.Time) {
	sites, err := s.Sites.List(ctx, store.SiteListFilter{Runtime: "php", Status: "active", Limit: 10000})
	if err != nil {
		slog.Error("scheduler: list PHP sites for pool migration", "error", err)
		return
	}
	servers := map[int64]*store.Server{}
	queued := 0
	for _, site := range sites {
		if queued >= phpPoolMigrateBatch {
			return
		}
		if v, _ := store.GetSetting(ctx, s.DB(), phpPoolSettingKey(site.ID)); v != "" {
			continue
		}
		srv, ok := servers[site.ServerID]
		if !ok {
			srv, _ = s.Servers.GetByID(ctx, site.ServerID)
			servers[site.ServerID] = srv
		}
		if !agentKnowsPools(srv) {
			continue
		}
		if t, _ := store.GetSetting(ctx, s.DB(), phpPoolTriedKey(site.ID)); t != "" {
			if at, err := time.Parse(time.RFC3339, t); err == nil && now.Sub(at) < 6*time.Hour {
				continue
			}
		}
		if busy, _ := s.Jobs.HasActiveJob(ctx, jobmodel.TypeSiteCreate, site.ID); busy {
			continue
		}
		// The worker completes the payload from the stored site (PHP
		// version and settings, system user, security, vhost_regen).
		payload, _ := json.Marshal(map[string]any{
			"site_id": site.ID, "name": site.Name, "domain": site.Domain,
			"runtime": site.Runtime, "root_path": site.RootPath,
		})
		job := &jobmodel.Job{
			ServerID:    sql.NullInt64{Int64: site.ServerID, Valid: true},
			SiteID:      sql.NullInt64{Int64: site.ID, Valid: true},
			Type:        jobmodel.TypeSiteCreate,
			Description: fmt.Sprintf("Move %s to its own PHP-FPM pool", site.Domain),
			Status:      jobmodel.StatusQueued,
			Payload:     string(payload),
		}
		if err := s.Jobs.Create(ctx, job); err != nil {
			slog.Error("scheduler: queue PHP pool migration", "site_id", site.ID, "error", err)
			continue
		}
		_ = store.SetSetting(ctx, s.DB(), phpPoolTriedKey(site.ID), now.UTC().Format(time.RFC3339))
		queued++
	}
}
