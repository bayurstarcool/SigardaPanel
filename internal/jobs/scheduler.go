package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"sigardapanel/internal/jobmodel"
	"sigardapanel/internal/notify"
	"sigardapanel/internal/store"
)

// RunScheduler starts a goroutine that polls for due backup schedules
// and creates backup jobs for them.
func RunScheduler(ctx context.Context, s *store.Store, interval time.Duration) {
	go func() {
		slog.Info("scheduler: started", "interval", interval)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// Run cleanup expired on startup
		runCleanup(ctx, s)

		for {
			select {
			case <-ctx.Done():
				slog.Info("scheduler: stopped")
				return
			case <-ticker.C:
				runTick(ctx, s)
			}
		}
	}()
}

func runTick(ctx context.Context, s *store.Store) {
	// Process legacy backup_schedules (DEPRECATED)
	// Deprecated: backup_schedules is the legacy per-site scheduling system.
	// Use backup_configs (global, multi-site) instead.
	// This code path will be removed once all schedules are migrated to
	// backup_configs.
	due, err := s.BackupSchedules.ListDue(ctx)
	if err != nil {
		slog.Error("scheduler: list due schedules", "error", err)
	} else {
		for _, sched := range due {
			if err := processSchedule(ctx, s, sched); err != nil {
				slog.Error("scheduler: process schedule failed", "id", sched.ID, "error", err)
				continue
			}
			if err := s.BackupSchedules.MarkRun(ctx, sched.ID); err != nil {
				slog.Error("scheduler: mark run failed", "id", sched.ID, "error", err)
			}
		}
	}

	// Process backup_configs
	configs, err := s.BackupConfigs.ListDue(ctx)
	if err != nil {
		slog.Error("scheduler: list due backup configs", "error", err)
	} else {
		for _, cfg := range configs {
			if err := processBackupConfig(ctx, s, cfg); err != nil {
				slog.Error("scheduler: process backup config failed", "id", cfg.ID, "error", err)
				continue
			}
			if err := s.BackupConfigs.MarkRun(ctx, cfg.ID); err != nil {
				slog.Error("scheduler: mark run failed (config)", "id", cfg.ID, "error", err)
			}
		}
	}

	// Cleanup expired backups after processing
	runCleanup(ctx, s)

	// Clean up stuck jobs and backups (>30min in 'running')
	runStuckCleanup(ctx, s)

	// Scrub sensitive credentials from finished job payloads (>1h old)
	if n, err := s.Jobs.ScrubTerminalPayloads(ctx, 1*time.Hour); err != nil {
		slog.Warn("scheduler: scrub terminal job payloads failed", "error", err)
	} else if n > 0 {
		slog.Info("scheduler: scrubbed sensitive payloads from finished jobs", "count", n)
	}

	// Metrics retention (hourly): samples arrive every minute per server and
	// were never deleted. The UI shows at most 7 days.
	runMetricsRetention(ctx, s, time.Now())

	// SSL auto-renewal check
	runSSLAutoRenew(ctx, s)
}

// runSSLAutoRenew checks for SSL certificates expiring within 30 days
// and enqueues ssl.renew jobs for them.
// sslRenewRetryBackoff is how long to wait after a failed auto-renewal
// before queuing another one for the same site.
const sslRenewRetryBackoff = 6 * time.Hour

func runSSLAutoRenew(ctx context.Context, s *store.Store) {
	const renewWindow = 30 * 24 * time.Hour // 30 days

	sites, err := s.Sites.ListSSLEpiring(ctx, renewWindow)
	if err != nil {
		slog.Error("scheduler: list ssl expiring", "error", err)
		return
	}
	if len(sites) == 0 {
		return
	}

	slog.Info("scheduler: ssl auto-renew check", "expiring_count", len(sites))

	for _, site := range sites {
		// Skip if site is not active
		if site.Status != "active" {
			continue
		}

		// Check if there's already a queued/running ssl.renew job for this site
		queued, _ := s.Jobs.List(ctx, store.JobListFilter{
			Type:   jobmodel.TypeSSLRenew,
			SiteID: site.ID,
			Status: jobmodel.StatusQueued,
		})
		running, _ := s.Jobs.List(ctx, store.JobListFilter{
			Type:   jobmodel.TypeSSLRenew,
			SiteID: site.ID,
			Status: jobmodel.StatusRunning,
		})
		if len(queued) > 0 || len(running) > 0 {
			slog.Info("scheduler: ssl renew already queued", "site_id", site.ID, "domain", site.Domain)
			continue
		}

		// A renewal that just failed (DNS moved, rate limit, port 80 closed…)
		// would otherwise be re-queued — and alerted — every minute. Back off.
		if failed, _ := s.Jobs.HasRecentFailedJob(ctx, jobmodel.TypeSSLRenew, site.ID, sslRenewRetryBackoff); failed {
			continue
		}

		// Enqueue ssl.renew job
		payload := fmt.Sprintf(`{"site_id":%d,"domain":%q,"auto":true}`, site.ID, site.Domain)
		job := &jobmodel.Job{
			ServerID:    sql.NullInt64{Int64: site.ServerID, Valid: true},
			SiteID:      sql.NullInt64{Int64: site.ID, Valid: true},
			Type:        jobmodel.TypeSSLRenew,
			Status:      jobmodel.StatusQueued,
			Payload:     payload,
			MaxAttempts: 3,
		}
		if err := s.Jobs.Create(ctx, job); err != nil {
			slog.Error("scheduler: enqueue ssl renew", "site_id", site.ID, "error", err)
			continue
		}

		slog.Info("scheduler: ssl auto-renew enqueued", "site_id", site.ID, "domain", site.Domain, "job_id", job.ID, "expires_at", site.SSLExpiresAt.Time.Format(time.RFC3339))

		// Send notification
		sender := notify.NewSender(s)
		daysLeft := int(time.Until(site.SSLExpiresAt.Time).Hours() / 24)
		sender.Send(ctx, "warning", "SSL Certificate Expiring",
			fmt.Sprintf("SSL for %s expires in %d days. Auto-renewal job queued (job #%d).", site.Domain, daysLeft, job.ID))
	}
}

// runStuckCleanup marks jobs and backups stuck in 'running' for >30min as failed.
func runStuckCleanup(ctx context.Context, s *store.Store) {
	staleDuration := 30 * time.Minute

	// Clean up stuck jobs
	if n, err := s.Jobs.CleanupStuckRunning(ctx, staleDuration); err != nil {
		slog.Warn("scheduler: cleanup stuck jobs failed", "error", err)
	} else if n > 0 {
		slog.Warn("scheduler: cleaned up stuck jobs", "count", n)
		sender := notify.NewSender(s)
		sender.Send(ctx, "warning", "Stuck Jobs Cleaned",
			fmt.Sprintf("%d job(s) stuck in 'running' for >30min were marked as failed.", n))
	}

	// Clean up stuck backups
	if n, err := s.Backups.CleanupStuckRunning(ctx, staleDuration); err != nil {
		slog.Warn("scheduler: cleanup stuck backups failed", "error", err)
	} else if n > 0 {
		slog.Warn("scheduler: cleaned up stuck backups", "count", n)
		sender := notify.NewSender(s)
		sender.Send(ctx, "warning", "Stuck Backups Cleaned",
			fmt.Sprintf("%d backup(s) stuck in 'running' for >30min were marked as failed.", n))
	}
}

func processSchedule(ctx context.Context, s *store.Store, sched store.BackupSchedule) error {
	site, err := s.Sites.GetByID(ctx, sched.SiteID)
	if err != nil {
		return fmt.Errorf("get site: %w", err)
	}
	if site == nil {
		return fmt.Errorf("site %d not found", sched.SiteID)
	}

	// Determine provider: schedule-specific > site-specific > default
	var provider *store.BackupStorageProvider
	storageType := "local"

	if sched.ProviderID.Valid {
		provider, _ = s.BackupProviders.GetByID(ctx, sched.ProviderID.Int64)
	} else if site.BackupProviderID.Valid {
		provider, _ = s.BackupProviders.GetByID(ctx, site.BackupProviderID.Int64)
	} else {
		provider, _ = s.BackupProviders.GetDefault(ctx)
	}

	if provider != nil {
		storageType = provider.ProviderType
	}

	// Create backup record
	backup := &store.Backup{
		SiteID:      sched.SiteID,
		Status:      "pending",
		StorageType: storageType,
		BackupType:  sched.BackupType,
	}
	if err := s.Backups.Create(ctx, backup); err != nil {
		return fmt.Errorf("create backup: %w", err)
	}

	// Determine job type based on backup_type
	switch sched.BackupType {
	case "database":
		// Create DB backup jobs for all databases on the site
		databases, err := s.Databases.ListBySite(ctx, sched.SiteID)
		if err != nil || len(databases) == 0 {
			// Fallback: create file backup if no databases
			createBackupJob(ctx, s, backup, site, provider)
		} else {
			for _, db := range databases {
				dbUser, dbPassword, err := s.DatabaseBackupCredentials(ctx, db)
				if err != nil {
					continue
				}
				// Create backup record for this database
				dbBackup := &store.Backup{
					SiteID:      sched.SiteID,
					Status:      "pending",
					StorageType: "database_" + storageType,
					BackupType:  "database",
				}
				if err := s.Backups.Create(ctx, dbBackup); err != nil {
					continue
				}
				payloadMap := map[string]any{
					"backup_id":   dbBackup.ID,
					"site_id":     site.ID,
					"domain":      site.Domain,
					"root_path":   site.RootPath,
					"db_name":     db.Name,
					"db_type":     db.Type,
					"db_user":     dbUser,
					"db_password": dbPassword,
				}
				if provider != nil {
					payloadMap["s3_config"] = map[string]string{
						"bucket":     provider.Bucket,
						"endpoint":   provider.Endpoint,
						"region":     provider.Region,
						"access_key": provider.AccessKey,
						"secret_key": provider.SecretKey,
					}
				}
				payload, _ := json.Marshal(payloadMap)
				job := &jobmodel.Job{
					ServerID: sql.NullInt64{Int64: site.ServerID, Valid: true},
					SiteID:   sql.NullInt64{Int64: site.ID, Valid: true},
					Type:     jobmodel.TypeDBBackupCreate,
					Status:   jobmodel.StatusQueued,
					Payload:  string(payload),
				}
				if err := s.Jobs.Create(ctx, job); err != nil {
					slog.Error("scheduler: create db backup job", "error", err)
				}
			}
		}
	case "both":
		createBackupJob(ctx, s, backup, site, provider)
		// Create DB backup jobs for all databases on the site
		databases, _ := s.Databases.ListBySite(ctx, sched.SiteID)
		for _, db := range databases {
			dbUser, dbPassword, err := s.DatabaseBackupCredentials(ctx, db)
			if err != nil {
				continue
			}
			dbBackup := &store.Backup{
				SiteID:      sched.SiteID,
				Status:      "pending",
				StorageType: "database_" + storageType,
				BackupType:  "database",
			}
			if err := s.Backups.Create(ctx, dbBackup); err != nil {
				continue
			}
			payloadMap := map[string]any{
				"backup_id":   dbBackup.ID,
				"site_id":     site.ID,
				"domain":      site.Domain,
				"root_path":   site.RootPath,
				"db_name":     db.Name,
				"db_type":     db.Type,
				"db_user":     dbUser,
				"db_password": dbPassword,
			}
			if provider != nil {
				payloadMap["s3_config"] = map[string]string{
					"bucket":     provider.Bucket,
					"endpoint":   provider.Endpoint,
					"region":     provider.Region,
					"access_key": provider.AccessKey,
					"secret_key": provider.SecretKey,
				}
			}
			payload, _ := json.Marshal(payloadMap)
			job := &jobmodel.Job{
				ServerID: sql.NullInt64{Int64: site.ServerID, Valid: true},
				SiteID:   sql.NullInt64{Int64: site.ID, Valid: true},
				Type:     jobmodel.TypeDBBackupCreate,
				Status:   jobmodel.StatusQueued,
				Payload:  string(payload),
			}
			if err := s.Jobs.Create(ctx, job); err != nil {
				slog.Error("scheduler: create db backup job", "error", err)
			}
		}
	default: // "files"
		createBackupJob(ctx, s, backup, site, provider)
	}

	slog.Info("scheduler: created backup job", "schedule_id", sched.ID, "backup_id", backup.ID, "site_id", sched.SiteID)
	return nil
}

func createBackupJob(ctx context.Context, s *store.Store, backup *store.Backup, site *store.Site, provider *store.BackupStorageProvider) {
	payloadMap := map[string]any{
		"backup_id": backup.ID,
		"site_id":   site.ID,
		"domain":    site.Domain,
		"root_path": site.RootPath,
		"storage":   backup.StorageType,
		"scheduled": true,
	}

	if provider != nil {
		payloadMap["s3_config"] = map[string]string{
			"bucket":     provider.Bucket,
			"endpoint":   provider.Endpoint,
			"region":     provider.Region,
			"access_key": provider.AccessKey,
			"secret_key": provider.SecretKey,
		}
	}

	payload, _ := json.Marshal(payloadMap)
	job := &jobmodel.Job{
		ServerID: sql.NullInt64{Int64: site.ServerID, Valid: true},
		SiteID:   sql.NullInt64{Int64: site.ID, Valid: true},
		Type:     jobmodel.TypeBackupCreate,
		Status:   jobmodel.StatusQueued,
		Payload:  string(payload),
	}
	if err := s.Jobs.Create(ctx, job); err != nil {
		slog.Error("scheduler: create backup job", "error", err)
	}
}

func processBackupConfig(ctx context.Context, s *store.Store, cfg store.BackupConfig) error {
	// Resolve storage provider
	storageType := "local"
	var provider *store.BackupStorageProvider
	if cfg.ProviderID.Valid {
		provider, _ = s.BackupProviders.GetByID(ctx, cfg.ProviderID.Int64)
	} else {
		provider, _ = s.BackupProviders.GetDefault(ctx)
	}
	if provider != nil {
		storageType = provider.ProviderType
	}

	// Per-config retention: use config's RetentionDays, default to 30
	retentionDays := cfg.RetentionDays
	if retentionDays <= 0 {
		retentionDays = 30
	}

	siteIDs, err := s.ResolveBackupConfigSiteIDs(ctx, cfg)
	if err != nil {
		return fmt.Errorf("resolve backup config sites: %w", err)
	}
	for _, siteID := range siteIDs {
		if siteID <= 0 {
			continue
		}
		site, err := s.Sites.GetByID(ctx, siteID)
		if err != nil || site == nil || site.Status != "active" {
			continue
		}

		// Deduplicate: skip if backup job already active for this site
		active, _ := s.Jobs.HasActiveJob(ctx, jobmodel.TypeBackupCreate, siteID)
		dbActive, _ := s.Jobs.HasActiveJob(ctx, jobmodel.TypeDBBackupCreate, siteID)
		if active || dbActive {
			slog.Info("scheduler: skip backup (already active)", "config_id", cfg.ID, "site_id", siteID)
			continue
		}

		if cfg.ScopeType == "both" {
			dbPayloads := []map[string]string{}
			databases, _ := s.Databases.ListBySite(ctx, siteID)
			for _, db := range databases {
				if len(cfg.DatabaseIDs) > 0 && !schedConfigDBIDInSlice(db.ID, cfg.DatabaseIDs) {
					continue
				}
				exists, err := s.BackupDatabaseExists(ctx, db)
				if err != nil || !exists {
					slog.Warn("scheduler: skip stale database record", "config_id", cfg.ID, "site_id", siteID, "database_id", db.ID, "database", db.Name)
					continue
				}
				dbUser, dbPassword, err := databaseBackupCredentials(ctx, s, db)
				if err != nil {
					slog.Warn("scheduler: skip database in full backup (credential unavailable)", "config_id", cfg.ID, "site_id", siteID, "database_id", db.ID, "error", err)
					continue
				}
				dbPayloads = append(dbPayloads, map[string]string{"name": db.Name, "type": db.Type, "user": dbUser, "password": dbPassword})
			}
			backup := &store.Backup{SiteID: siteID, Status: "pending", StorageType: "full_" + storageType, BackupType: "both", ExpiresAt: sql.NullTime{Time: time.Now().Add(time.Duration(retentionDays) * 24 * time.Hour), Valid: true}}
			if err := s.Backups.Create(ctx, backup); err != nil {
				continue
			}
			payloadMap := map[string]any{"backup_id": backup.ID, "site_id": site.ID, "domain": site.Domain, "root_path": site.RootPath, "storage": storageType, "backup_type": "both", "databases": dbPayloads, "scheduled": true}
			if provider != nil {
				payloadMap["s3_config"] = map[string]string{"bucket": provider.Bucket, "endpoint": provider.Endpoint, "region": provider.Region, "access_key": provider.AccessKey, "secret_key": provider.SecretKey}
			}
			payload, _ := json.Marshal(payloadMap)
			job := &jobmodel.Job{ServerID: sql.NullInt64{Int64: site.ServerID, Valid: true}, SiteID: sql.NullInt64{Int64: site.ID, Valid: true}, Type: jobmodel.TypeBackupCreate, Status: jobmodel.StatusQueued, Payload: string(payload)}
			if err := s.Jobs.Create(ctx, job); err != nil {
				continue
			}
			slog.Info("scheduler: created full backup job from config", "config_id", cfg.ID, "backup_id", backup.ID, "site_id", siteID)
			continue
		}

		// Files backup
		if cfg.ScopeType == "files" {
			backup := &store.Backup{SiteID: siteID, Status: "pending", StorageType: storageType, BackupType: "files", ExpiresAt: sql.NullTime{Time: time.Now().Add(time.Duration(retentionDays) * 24 * time.Hour), Valid: true}}
			if err := s.Backups.Create(ctx, backup); err != nil {
				slog.Error("scheduler: create backup config backup record", "config_id", cfg.ID, "site_id", siteID, "error", err)
				continue
			}
			createBackupJob(ctx, s, backup, site, provider)
			slog.Info("scheduler: created backup job from config", "config_id", cfg.ID, "backup_id", backup.ID, "site_id", siteID)
		}

		// Database backup
		if cfg.ScopeType == "database" {
			databases, err := s.Databases.ListBySite(ctx, siteID)
			if err != nil || len(databases) == 0 {
				continue
			}
			for _, db := range databases {
				if len(cfg.DatabaseIDs) > 0 && !schedConfigDBIDInSlice(db.ID, cfg.DatabaseIDs) {
					continue
				}
				exists, err := s.BackupDatabaseExists(ctx, db)
				if err != nil || !exists {
					slog.Warn("scheduler: skip stale database record", "config_id", cfg.ID, "site_id", siteID, "database_id", db.ID, "database", db.Name)
					continue
				}
				dbUser, dbPassword, err := databaseBackupCredentials(ctx, s, db)
				if err != nil {
					slog.Warn("scheduler: skip database backup (credential unavailable)", "config_id", cfg.ID, "site_id", siteID, "database_id", db.ID, "error", err)
					continue
				}
				dbStorageType := "database_" + storageType
				backup := &store.Backup{SiteID: siteID, Status: "pending", StorageType: dbStorageType, BackupType: "database", ExpiresAt: sql.NullTime{Time: time.Now().Add(time.Duration(retentionDays) * 24 * time.Hour), Valid: true}}
				if err := s.Backups.Create(ctx, backup); err != nil {
					continue
				}
				payloadMap := map[string]any{
					"backup_id": backup.ID, "site_id": site.ID, "domain": site.Domain, "root_path": site.RootPath,
					"db_name": db.Name, "db_type": db.Type, "db_user": dbUser, "db_password": dbPassword,
				}
				if provider != nil {
					payloadMap["s3_config"] = map[string]string{
						"bucket": provider.Bucket, "endpoint": provider.Endpoint,
						"region": provider.Region, "access_key": provider.AccessKey, "secret_key": provider.SecretKey,
					}
				}
				payload, _ := json.Marshal(payloadMap)
				job := &jobmodel.Job{
					ServerID: sql.NullInt64{Int64: site.ServerID, Valid: true},
					SiteID:   sql.NullInt64{Int64: site.ID, Valid: true},
					Type:     jobmodel.TypeDBBackupCreate, Status: jobmodel.StatusQueued, Payload: string(payload),
				}
				if err := s.Jobs.Create(ctx, job); err != nil {
					slog.Error("scheduler: create db backup job from config", "error", err)
					continue
				}
				slog.Info("scheduler: created db backup job from config", "config_id", cfg.ID, "db", db.Name, "site_id", siteID)
			}
		}
	}
	return nil
}

// effectiveBackupSiteIDs auto-discovers active sites for database/full configs
// when site_ids is empty. File-only configs stay explicit to avoid silently
// archiving newly created sites.
func effectiveBackupSiteIDs(cfg store.BackupConfig, sites []store.Site) []int64 {
	if len(cfg.SiteIDs) > 0 {
		return append([]int64(nil), cfg.SiteIDs...)
	}
	if cfg.ScopeType != "database" && cfg.ScopeType != "both" {
		return nil
	}
	ids := make([]int64, 0, len(sites))
	for _, site := range sites {
		if site.Status == "active" {
			ids = append(ids, site.ID)
		}
	}
	return ids
}

func databaseBackupCredentials(ctx context.Context, s *store.Store, database store.Database) (string, string, error) {
	return s.DatabaseBackupCredentials(ctx, database)
}

func schedConfigDBIDInSlice(val int64, slice []int64) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}

// enqueueLocalBackupDeletes creates backup.delete jobs so that agents remove
// expired local backup files from disk. Without this, expired records are
// deleted from the DB but the .tar.gz files remain on the VPS forever, filling
// the disk. Jobs are created before the DB rows are removed by CleanupExpired.
func enqueueLocalBackupDeletes(ctx context.Context, s *store.Store, keepLast int) {
	expired, err := s.Backups.ListExpiredLocal(ctx, keepLast)
	if err != nil {
		slog.Warn("scheduler: list expired local backups failed", "error", err)
		return
	}
	for _, e := range expired {
		if !e.ServerID.Valid {
			// No known server (e.g. panel-local backup); skip agent dispatch.
			continue
		}
		payload, _ := json.Marshal(map[string]any{
			"backup_id":    e.ID,
			"storage_path": e.StoragePath,
		})
		job := &jobmodel.Job{
			ServerID: e.ServerID,
			SiteID:   sql.NullInt64{Int64: e.SiteID, Valid: true},
			Type:     jobmodel.TypeBackupDelete,
			Status:   jobmodel.StatusQueued,
			Payload:  string(payload),
		}
		if err := s.Jobs.Create(ctx, job); err != nil {
			slog.Warn("scheduler: create backup.delete job failed", "backup_id", e.ID, "error", err)
		}
	}
}

func runCleanup(ctx context.Context, s *store.Store) {
	// Apply retention: set expires_at on backups that don't have it yet
	// based on global retention_days setting
	retentionStr, err := store.GetSetting(ctx, s.DB(), "retention_days")
	keepLastStr, _ := store.GetSetting(ctx, s.DB(), "keep_last_per_site")
	var keepLast int
	fmt.Sscanf(keepLastStr, "%d", &keepLast)
	if keepLast <= 0 {
		keepLast = 3
	}
	if err != nil || retentionStr == "" || retentionStr == "0" {
		// No retention configured, just clean up expired ones
		enqueueLocalBackupDeletes(ctx, s, keepLast)
		n, err := s.Backups.CleanupExpired(ctx, keepLast)
		if err != nil {
			slog.Warn("scheduler: cleanup expired failed", "error", err)
		} else if n > 0 {
			slog.Info("scheduler: cleaned up expired backups", "count", n)
		}
		return
	}

	var retentionDays int
	if _, err := fmt.Sscanf(retentionStr, "%d", &retentionDays); err != nil || retentionDays <= 0 {
		return
	}

	// Set expires_at for completed backups that don't have it set yet
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	_, err = s.DB().ExecContext(ctx,
		`UPDATE backups SET expires_at = ? WHERE status = 'completed' AND expires_at IS NULL AND created_at < ?`,
		cutoff, cutoff,
	)
	if err != nil {
		slog.Warn("scheduler: set expires_at failed", "error", err)
	}

	enqueueLocalBackupDeletes(ctx, s, keepLast)
	n, err := s.Backups.CleanupExpired(ctx, keepLast)
	if err != nil {
		slog.Warn("scheduler: cleanup expired failed", "error", err)
	} else if n > 0 {
		slog.Info("scheduler: cleaned up expired backups", "count", n)
	}

	// Clean up local files that have already been uploaded to S3
	cleaned, err := s.Backups.CleanupLocalAfterS3(ctx)
	if err != nil {
		slog.Warn("scheduler: cleanup local-after-s3 failed", "error", err)
	} else if cleaned > 0 {
		slog.Info("scheduler: removed local backups already on S3", "count", cleaned)
	}

	// Also clean up orphaned local files via os for any that CleanupLocalAfterS3 missed
	rows, err := s.DB().QueryContext(ctx, `SELECT id, storage_path FROM backups WHERE status = 'completed' AND s3_path IS NOT NULL AND s3_path != '' AND storage_path IS NOT NULL AND storage_path != '' AND storage_path NOT LIKE 's3://%'`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id int64
			var path string
			if err := rows.Scan(&id, &path); err != nil {
				continue
			}
			if err := os.Remove(path); err != nil {
				slog.Warn("scheduler: failed to remove local backup (already on S3)", "path", path, "error", err)
			} else {
				slog.Info("scheduler: removed local backup (already on S3)", "path", path, "backup_id", id)
				_, _ = s.DB().ExecContext(ctx, `UPDATE backups SET storage_path = NULL WHERE id = ?`, id)
			}
		}
	}

	// Clean up orphaned pending backups (no job created for them)
	if n, err := s.Backups.CleanupOrphanedPending(ctx, 1*time.Hour); err != nil {
		slog.Warn("scheduler: cleanup orphaned pending backups failed", "error", err)
	} else if n > 0 {
		slog.Info("scheduler: cleaned up orphaned pending backups", "count", n)
	}
}

// metricsRetention is how long server metrics are kept (UI max range is 7d).
const metricsRetention = 8 * 24 * time.Hour

var lastMetricsPrune time.Time

// runMetricsRetention prunes old metrics at most once an hour.
func runMetricsRetention(ctx context.Context, s *store.Store, now time.Time) {
	if now.Sub(lastMetricsPrune) < time.Hour {
		return
	}
	lastMetricsPrune = now
	if n, err := s.PruneMetrics(ctx, metricsRetention); err != nil {
		slog.Warn("scheduler: prune metrics failed", "error", err)
	} else if n > 0 {
		slog.Info("scheduler: pruned old metrics", "rows", n)
	}
}
