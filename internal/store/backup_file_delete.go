package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"sigardapanel/internal/jobmodel"
)

// QueueBackupFileDelete queues a backup.delete job: the archive is a
// root-owned file on the site's server, so only that server's agent can
// remove it (the panel's own os.Remove never could, and runs sandboxed).
// clearPath asks the worker to clear the backup's storage_path once the file
// is gone (the record stays, e.g. because the copy on S3 remains). Nothing
// is queued for S3-only paths, or when a delete for this backup is already
// pending or failed within the last hour.
func (s *Store) QueueBackupFileDelete(ctx context.Context, backupID, siteID int64, path string, clearPath bool) error {
	path = strings.TrimSpace(path)
	if path == "" || strings.HasPrefix(path, "s3://") {
		return nil
	}
	var pending int
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM jobs WHERE type = ? AND json_extract(payload, '$.backup_id') = ?
		AND (status IN ('queued', 'running') OR (status = 'failed' AND created_at > datetime('now', '-1 hour')))`,
		jobmodel.TypeBackupDelete, backupID).Scan(&pending)
	if pending > 0 {
		return nil
	}
	var serverID sql.NullInt64
	_ = s.db.QueryRowContext(ctx, `SELECT server_id FROM sites WHERE id = ?`, siteID).Scan(&serverID)
	payload, _ := json.Marshal(map[string]any{"backup_id": backupID, "storage_path": path, "clear_storage_path": clearPath})
	return s.Jobs.Create(ctx, &jobmodel.Job{
		ServerID: serverID,
		SiteID:   sql.NullInt64{Int64: siteID, Valid: siteID > 0},
		Type:     jobmodel.TypeBackupDelete,
		Status:   jobmodel.StatusQueued,
		Payload:  string(payload),
	})
}

// ListLocalAfterS3 returns completed backups uploaded to S3 that still
// reference a local file.
func (s *SQLiteBackupStore) ListLocalAfterS3(ctx context.Context) ([]ExpiredLocalBackup, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT b.id, s.server_id, b.site_id, b.storage_path FROM backups b
		LEFT JOIN sites s ON s.id = b.site_id
		WHERE b.status = 'completed' AND b.s3_path IS NOT NULL AND b.s3_path != ''
		  AND b.storage_path IS NOT NULL AND b.storage_path != '' AND b.storage_path NOT LIKE 's3://%'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ExpiredLocalBackup
	for rows.Next() {
		var e ExpiredLocalBackup
		if err := rows.Scan(&e.ID, &e.ServerID, &e.SiteID, &e.StoragePath); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ProviderIDForS3 finds the storage provider with this bucket, endpoint and
// access key (what a backup job's s3_config carries); 0 when none matches.
func (s *Store) ProviderIDForS3(ctx context.Context, bucket, endpoint, accessKey string) int64 {
	providers, err := s.BackupProviders.List(ctx)
	if err != nil {
		return 0
	}
	for _, p := range providers {
		if p.Bucket == bucket && p.Endpoint == endpoint && p.AccessKey == accessKey {
			return p.ID
		}
	}
	return 0
}

// BackupProviderFor returns the provider holding a backup's S3 copy: the one
// recorded with the backup, else (older backups) the site's, else the default.
func (s *Store) BackupProviderFor(ctx context.Context, b *Backup, site *Site) *BackupStorageProvider {
	if b != nil && b.ProviderID.Valid {
		if p, err := s.BackupProviders.GetByID(ctx, b.ProviderID.Int64); err == nil && p != nil {
			return p
		}
	}
	if site != nil && site.BackupProviderID.Valid && site.BackupProviderID.Int64 > 0 {
		if p, err := s.BackupProviders.GetByID(ctx, site.BackupProviderID.Int64); err == nil && p != nil {
			return p
		}
	}
	p, _ := s.BackupProviders.GetDefault(ctx)
	return p
}

// HasRecentLoginFrom reports whether userID logged in successfully from ip
// within the last 90 days (per the audit log).
func (s *Store) HasRecentLoginFrom(ctx context.Context, userID int64, ip string) bool {
	if ip == "" {
		return false
	}
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_logs WHERE user_id = ? AND action = 'login'
		AND json_valid(details) AND json_extract(details, '$.ip') = ? AND created_at > datetime('now', '-90 days')`, userID, ip).Scan(&n)
	return err == nil && n > 0
}
