package store

import (
	"context"
	"fmt"
)

// accessibleSitesSubquery selects the IDs of every site a user owns or is
// assigned to (any permission level). It takes the user ID twice.
const accessibleSitesSubquery = `SELECT id FROM sites WHERE owner_id = ? UNION SELECT site_id FROM user_sites WHERE user_id = ?`

// ListBackupsForUser returns backups (newest first) of the sites the user owns
// or is assigned to. It is the non-super_admin counterpart of
// Backups.ListAll, which returns every tenant's backups.
func (s *Store) ListBackupsForUser(ctx context.Context, userID int64, limit, offset int) ([]Backup, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT b.id, b.site_id, b.status, b.storage_type, b.storage_path, COALESCE(b.backup_type,'files'), COALESCE(b.s3_path,''), COALESCE(b.progress,''), b.size_bytes, b.file_count, b.error, b.created_at, b.completed_at, b.expires_at, COALESCE(s.name,''), COALESCE(s.domain,'')
		 FROM backups b LEFT JOIN sites s ON s.id = b.site_id
		 WHERE b.site_id IN (`+accessibleSitesSubquery+`)
		 ORDER BY b.created_at DESC LIMIT ? OFFSET ?`,
		userID, userID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("query user backups: %w", err)
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
