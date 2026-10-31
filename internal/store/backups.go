package store

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

var ErrDuplicateBackup = fmt.Errorf("backup already in progress")

// Backup represents a site file backup record.
type Backup struct {
	ID          int64
	SiteID      int64
	Status      string
	StorageType string
	StoragePath sql.NullString
	BackupType  string
	S3Path      sql.NullString
	Progress    string
	SizeBytes   int64
	FileCount   int
	Error       sql.NullString
	CreatedAt   time.Time
	CompletedAt sql.NullTime
	ExpiresAt   sql.NullTime
	SiteName    string
	SiteDomain  string
}

// BackupRepository defines backup persistence operations.
type BackupRepository interface {
	Create(ctx context.Context, b *Backup) error
	GetByID(ctx context.Context, id int64) (*Backup, error)
	ListBySite(ctx context.Context, siteID int64, limit, offset int) ([]Backup, error)
	ListAll(ctx context.Context, limit, offset int) ([]Backup, error)
	UpdateStatus(ctx context.Context, id int64, status string) error
	UpdateProgress(ctx context.Context, id int64, progress string) error
	UpdateS3Path(ctx context.Context, id int64, s3Path string) error
	MarkCompleted(ctx context.Context, id int64, sizeBytes int64, fileCount int, path string) error
	MarkFailed(ctx context.Context, id int64, errMsg string) error
	Delete(ctx context.Context, id int64) error
	DeleteOldest(ctx context.Context, siteID int64, keep int) ([]Backup, error)
	CleanupExpired(ctx context.Context, keepLast int) (int64, error)
	ListExpiredLocal(ctx context.Context, keepLast int) ([]ExpiredLocalBackup, error)
	ClearStoragePath(ctx context.Context, id int64) error
	CleanupLocalAfterS3(ctx context.Context) (int, error)
	// Backup helper methods
	CleanupStuckRunning(ctx context.Context, staleAfter time.Duration) (int64, error)
	CleanupOrphanedPending(ctx context.Context, olderThan time.Duration) (int64, error)
}

// SQLiteBackupStore implements BackupRepository against SQLite.
type SQLiteBackupStore struct {
	db *sql.DB
}

// NewBackupStore returns a SQLite-backed backup store.
func NewBackupStore(db *sql.DB) *SQLiteBackupStore {
	return &SQLiteBackupStore{db: db}
}

// Create inserts a new backup record. Defaults StorageType to "local" and Status to "pending".
// Returns ErrDuplicate if a pending/running backup already exists for this site.
func (s *SQLiteBackupStore) Create(ctx context.Context, b *Backup) error {
	if b.StorageType == "" {
		b.StorageType = "local"
	}
	if b.Status == "" {
		b.Status = "pending"
	}
	if b.BackupType == "" {
		b.BackupType = "files"
	}
	// Prevent duplicate file backups: only insert if no pending/running file
	// backup exists for this site. Database backups are created one per
	// database in a loop, so a per-(site, type) guard would drop every
	// database after the first; they are not de-duplicated here.
	var expiresAt any
	if b.ExpiresAt.Valid {
		expiresAt = b.ExpiresAt.Time.UTC().Format("2006-01-02 15:04:05")
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO backups (site_id, status, storage_type, storage_path, backup_type, progress, expires_at)
		SELECT ?, ?, ?, ?, ?, ?, ?
		WHERE ? = 'database' OR ? NOT IN ('pending','running','archiving') OR NOT EXISTS (
			SELECT 1 FROM backups WHERE site_id = ? AND backup_type = ? AND status IN ('pending','running','archiving')
		)`,
		b.SiteID, b.Status, b.StorageType, b.StoragePath, b.BackupType, "", expiresAt,
		b.BackupType, b.Status, b.SiteID, b.BackupType,
	)
	if err != nil {
		return fmt.Errorf("insert backup: %w", err)
	}
	// RowsAffected, not LastInsertId: when the guarded INSERT inserts nothing,
	// SQLite still reports the connection's previous rowid (possibly from
	// another table), which handed out an unrelated backup's id.
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return ErrDuplicateBackup
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("last insert id: %w", err)
	}
	b.ID = id
	return nil
}

// GetByID retrieves a backup by ID. Returns (nil, nil) if not found.
func (s *SQLiteBackupStore) GetByID(ctx context.Context, id int64) (*Backup, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT b.id, b.site_id, b.status, b.storage_type, b.storage_path, COALESCE(b.backup_type,'files'), COALESCE(b.s3_path,''), COALESCE(b.progress,''), b.size_bytes, b.file_count, b.error, b.created_at, b.completed_at, b.expires_at, COALESCE(s.name,''), COALESCE(s.domain,'') FROM backups b LEFT JOIN sites s ON s.id = b.site_id WHERE b.id = ?`, id)
	return scanBackup(row)
}

// ListBySite returns backups for a site ordered by created_at DESC.
func (s *SQLiteBackupStore) ListBySite(ctx context.Context, siteID int64, limit, offset int) ([]Backup, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT b.id, b.site_id, b.status, b.storage_type, b.storage_path, COALESCE(b.backup_type,'files'), COALESCE(b.s3_path,''), COALESCE(b.progress,''), b.size_bytes, b.file_count, b.error, b.created_at, b.completed_at, b.expires_at, COALESCE(s.name,''), COALESCE(s.domain,'') FROM backups b LEFT JOIN sites s ON s.id = b.site_id WHERE b.site_id = ? ORDER BY b.created_at DESC LIMIT ? OFFSET ?`,
		siteID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("query backups by site: %w", err)
	}
	defer rows.Close()

	var out []Backup
	for rows.Next() {
		var b Backup
		if err := rows.Scan(
			&b.ID, &b.SiteID, &b.Status, &b.StorageType, &b.StoragePath, &b.BackupType, &b.S3Path, &b.Progress,
			&b.SizeBytes, &b.FileCount,
			&b.Error, &b.CreatedAt, &b.CompletedAt, &b.ExpiresAt, &b.SiteName, &b.SiteDomain,
		); err != nil {
			return nil, fmt.Errorf("scan backup: %w", err)
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return out, nil
}

// ListAll returns all backups ordered by created_at DESC.
func (s *SQLiteBackupStore) ListAll(ctx context.Context, limit, offset int) ([]Backup, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT b.id, b.site_id, b.status, b.storage_type, b.storage_path, COALESCE(b.backup_type,'files'), COALESCE(b.s3_path,''), COALESCE(b.progress,''), b.size_bytes, b.file_count, b.error, b.created_at, b.completed_at, b.expires_at, COALESCE(s.name,''), COALESCE(s.domain,'') FROM backups b LEFT JOIN sites s ON s.id = b.site_id ORDER BY b.created_at DESC LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("query backups: %w", err)
	}
	defer rows.Close()

	var out []Backup
	for rows.Next() {
		var b Backup
		if err := rows.Scan(
			&b.ID, &b.SiteID, &b.Status, &b.StorageType, &b.StoragePath, &b.BackupType, &b.S3Path, &b.Progress,
			&b.SizeBytes, &b.FileCount,
			&b.Error, &b.CreatedAt, &b.CompletedAt, &b.ExpiresAt, &b.SiteName, &b.SiteDomain,
		); err != nil {
			return nil, fmt.Errorf("scan backup: %w", err)
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return out, nil
}

// UpdateStatus updates a backup's status.
func (s *SQLiteBackupStore) UpdateStatus(ctx context.Context, id int64, status string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE backups SET status = ?, started_at = CASE WHEN ? = 'running' AND started_at IS NULL THEN CURRENT_TIMESTAMP ELSE started_at END WHERE id = ?`,
		status, status, id,
	)
	if err != nil {
		return fmt.Errorf("update backup status: %w", err)
	}
	return nil
}

// UpdateProgress sets the progress text for a backup record.
func (s *SQLiteBackupStore) UpdateProgress(ctx context.Context, id int64, progress string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE backups SET progress = ? WHERE id = ?`,
		progress, id,
	)
	if err != nil {
		return fmt.Errorf("update backup progress: %w", err)
	}
	return nil
}

// UpdateS3Path sets the s3_path for a backup record.
func (s *SQLiteBackupStore) UpdateS3Path(ctx context.Context, id int64, s3Path string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE backups SET s3_path = ? WHERE id = ?`,
		s3Path, id,
	)
	if err != nil {
		return fmt.Errorf("update backup s3_path: %w", err)
	}
	return nil
}

// MarkCompleted updates a backup as completed with size, file count, and path.
func (s *SQLiteBackupStore) MarkCompleted(ctx context.Context, id int64, sizeBytes int64, fileCount int, path string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE backups SET status = ?, size_bytes = ?, file_count = ?, storage_path = ?, progress = 'completed', completed_at = CURRENT_TIMESTAMP WHERE id = ?`,
		"completed", sizeBytes, fileCount, sql.NullString{String: path, Valid: path != ""}, id,
	)
	if err != nil {
		return fmt.Errorf("mark backup completed: %w", err)
	}
	return nil
}

// MarkFailed updates a backup as failed with an error message and completed_at.
func (s *SQLiteBackupStore) MarkFailed(ctx context.Context, id int64, errMsg string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE backups SET status = ?, error = ?, progress = ?, completed_at = CURRENT_TIMESTAMP WHERE id = ?`,
		"failed", sql.NullString{String: errMsg, Valid: errMsg != ""}, "failed", id,
	)
	if err != nil {
		return fmt.Errorf("mark backup failed: %w", err)
	}
	return nil
}

// DeleteOldest removes older backups, keeping only the latest `keep` per site+type.
// Returns the deleted backups (with S3 paths) so callers can clean up cloud storage.
func (s *SQLiteBackupStore) DeleteOldest(ctx context.Context, siteID int64, keep int) ([]Backup, error) {
	// First, query which backups will be deleted
	rows, err := s.db.QueryContext(ctx,
		`SELECT b.id, b.site_id, b.status, b.storage_type, b.storage_path, COALESCE(b.backup_type,'files'), b.s3_path, COALESCE(b.progress,''), b.size_bytes, b.file_count, b.error, b.created_at, b.completed_at, b.expires_at, COALESCE(s.name,''), COALESCE(s.domain,'')
		FROM backups b
		LEFT JOIN sites s ON s.id = b.site_id
		WHERE b.id IN (
			SELECT id FROM (
			SELECT id, ROW_NUMBER() OVER (
				PARTITION BY site_id, COALESCE(backup_type,'files')
				ORDER BY id DESC
			) as rn FROM backups WHERE site_id = ?
			) WHERE rn > ?
		)`,
		siteID, keep,
	)
	if err != nil {
		return nil, fmt.Errorf("query oldest backups: %w", err)
	}
	defer rows.Close()

	var deleted []Backup
	for rows.Next() {
		var b Backup
		if err := rows.Scan(&b.ID, &b.SiteID, &b.Status, &b.StorageType, &b.StoragePath, &b.BackupType, &b.S3Path, &b.Progress, &b.SizeBytes, &b.FileCount, &b.Error, &b.CreatedAt, &b.CompletedAt, &b.ExpiresAt, &b.SiteName, &b.SiteDomain); err != nil {
			continue
		}
		deleted = append(deleted, b)
	}
	rows.Close()

	if len(deleted) == 0 {
		return nil, nil
	}

	// Collect IDs to delete
	ids := make([]any, len(deleted))
	placeholders := make([]string, len(deleted))
	for i, b := range deleted {
		ids[i] = b.ID
		placeholders[i] = "?"
	}

	// Use parameterized query
	_, err = s.db.ExecContext(ctx, `DELETE FROM backups WHERE id IN (`+strings.Join(placeholders, ",")+`)`, ids...)
	if err != nil {
		return nil, fmt.Errorf("delete oldest backups: %w", err)
	}

	slog.Info("store: cleaned up old backups", "site_id", siteID, "kept", keep, "deleted", len(deleted))
	return deleted, nil
}

// Delete removes a backup record.
func (s *SQLiteBackupStore) Delete(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM backups WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete backup: %w", err)
	}
	return nil
}

// ExpiredLocalBackup identifies an expired backup that still has a local file
// on an agent, so the panel can dispatch a delete job before removing the row.
type ExpiredLocalBackup struct {
	ID          int64
	ServerID    sql.NullInt64
	SiteID      int64
	StoragePath string
}

// ListExpiredLocal returns expired backups (same selection as CleanupExpired)
// that still reference a local (non-S3) file, along with the owning server so
// the caller can enqueue agent-side file deletion. keepLast backups per site
// are always retained.
func (s *SQLiteBackupStore) ListExpiredLocal(ctx context.Context, keepLast int) ([]ExpiredLocalBackup, error) {
	if keepLast <= 0 {
		keepLast = 3
	}
	query := `
		SELECT b.id, sv.id, b.site_id, b.storage_path
		FROM backups b
		LEFT JOIN sites s ON s.id = b.site_id
		LEFT JOIN servers sv ON sv.id = s.server_id
		WHERE b.expires_at IS NOT NULL
		  AND b.expires_at < CURRENT_TIMESTAMP
		  AND b.storage_path IS NOT NULL
		  AND b.storage_path != ''
		  AND b.storage_path NOT LIKE 's3://%'
		  AND b.id NOT IN (
			SELECT id FROM (
				SELECT id,
				       ROW_NUMBER() OVER (PARTITION BY site_id ORDER BY COALESCE(completed_at, created_at) DESC, id DESC) AS rn
				FROM backups
				WHERE status = 'completed'
			) ranked
			WHERE rn <= ?
		)`
	rows, err := s.db.QueryContext(ctx, query, keepLast)
	if err != nil {
		return nil, fmt.Errorf("list expired local backups: %w", err)
	}
	defer rows.Close()
	var out []ExpiredLocalBackup
	for rows.Next() {
		var e ExpiredLocalBackup
		if err := rows.Scan(&e.ID, &e.ServerID, &e.SiteID, &e.StoragePath); err != nil {
			return nil, fmt.Errorf("scan expired local backup: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// CleanupExpired deletes backups where expires_at is past CURRENT_TIMESTAMP.
// Returns the number of rows deleted.
func (s *SQLiteBackupStore) CleanupExpired(ctx context.Context, keepLast int) (int64, error) {
	if keepLast <= 0 {
		keepLast = 3
	}
	query := `
		DELETE FROM backups
		WHERE expires_at IS NOT NULL
		  AND expires_at < CURRENT_TIMESTAMP
		  AND id NOT IN (
			SELECT id FROM (
				SELECT id,
				       ROW_NUMBER() OVER (PARTITION BY site_id ORDER BY COALESCE(completed_at, created_at) DESC, id DESC) AS rn
				FROM backups
				WHERE status = 'completed'
			) ranked
			WHERE rn <= ?
		)`
	res, err := s.db.ExecContext(ctx, query, keepLast)
	if err != nil {
		return 0, fmt.Errorf("cleanup expired backups: %w", err)
	}
	return res.RowsAffected()
}

func scanBackup(row *sql.Row) (*Backup, error) {
	var b Backup
	err := row.Scan(
		&b.ID, &b.SiteID, &b.Status, &b.StorageType, &b.StoragePath, &b.BackupType, &b.S3Path, &b.Progress,
		&b.SizeBytes, &b.FileCount,
		&b.Error, &b.CreatedAt, &b.CompletedAt, &b.ExpiresAt, &b.SiteName, &b.SiteDomain,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan backup: %w", err)
	}
	return &b, nil
}

// ClearStoragePath sets storage_path to NULL for a backup (used after local file is deleted post-S3 upload).
func (s *SQLiteBackupStore) ClearStoragePath(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE backups SET storage_path = NULL WHERE id = ?`, id)
	return err
}

// CleanupLocalAfterS3 removes local files for completed backups that have been uploaded to S3.
func (s *SQLiteBackupStore) CleanupLocalAfterS3(ctx context.Context) (int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, storage_path FROM backups WHERE status = 'completed' AND s3_path IS NOT NULL AND s3_path != '' AND storage_path IS NOT NULL AND storage_path != '' AND storage_path NOT LIKE 's3://%'`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var cleaned int
	for rows.Next() {
		var id int64
		var path string
		if err := rows.Scan(&id, &path); err != nil {
			continue
		}
		if err := os.Remove(path); err != nil {
			slog.Warn("store: failed to remove local backup (already on S3)", "path", path, "error", err)
		} else {
			slog.Info("store: removed local backup (already on S3)", "s3_path", path, "backup_id", id)
			_, _ = s.db.ExecContext(ctx, `UPDATE backups SET storage_path = NULL WHERE id = ?`, id)
			cleaned++
		}
	}
	return cleaned, nil
}

// CleanupStuckRunning marks backups stuck in 'running' for longer than staleAfter as failed.
func (s *SQLiteBackupStore) CleanupStuckRunning(ctx context.Context, staleAfter time.Duration) (int64, error) {
	cutoff := time.Now().Add(-staleAfter)
	res, err := s.db.ExecContext(ctx,
		`UPDATE backups SET status = 'failed', error = 'cleanup: stuck in running state', progress = 'failed', completed_at = CURRENT_TIMESTAMP
		 WHERE status = 'running' AND COALESCE(started_at, created_at) < ?`, cutoff.UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		return 0, fmt.Errorf("cleanup stuck backups: %w", err)
	}
	return res.RowsAffected()
}

// CleanupOrphanedPending deletes backup records stuck in 'pending' for longer than olderThan.
func (s *SQLiteBackupStore) CleanupOrphanedPending(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM backups WHERE status = 'pending' AND created_at < ?
		   AND NOT EXISTS (
		     SELECT 1 FROM jobs
		      WHERE jobs.status IN ('queued','running')
		        AND json_valid(jobs.payload)
		        AND json_extract(jobs.payload, '$.backup_id') = backups.id
		   )`, cutoff.UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		return 0, fmt.Errorf("cleanup orphaned pending backups: %w", err)
	}
	return res.RowsAffected()
}
