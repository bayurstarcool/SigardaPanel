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
