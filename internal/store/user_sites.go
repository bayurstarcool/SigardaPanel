package store

import (
	"context"
	"database/sql"
)

// UserSite maps a panel user to a site with a permission level.
type UserSite struct {
	ID         int64  `json:"id"`
	UserID     int64  `json:"user_id"`
	SiteID     int64  `json:"site_id"`
	Permission string `json:"permission"`
	CreatedAt  string `json:"created_at"`
	Username   string `json:"username,omitempty"`
	Domain     string `json:"domain,omitempty"`
}

// UserSiteRepository defines operations for user-site access mappings.
type UserSiteRepository interface {
	Assign(ctx context.Context, userID, siteID int64, permission string) error
	Unassign(ctx context.Context, userID, siteID int64) error
	ListByUser(ctx context.Context, userID int64) ([]UserSite, error)
	ListBySite(ctx context.Context, siteID int64) ([]UserSite, error)
	HasAccess(ctx context.Context, userID, siteID int64) (bool, error)
	GetPermission(ctx context.Context, userID, siteID int64) (string, error)
}

// SQLiteUserSiteStore implements UserSiteRepository with SQLite.
type SQLiteUserSiteStore struct {
	db *sql.DB
}

// NewUserSiteStore creates a new SQLiteUserSiteStore.
func NewUserSiteStore(db *sql.DB) *SQLiteUserSiteStore {
	return &SQLiteUserSiteStore{db: db}
}

// Assign grants a user access to a site with the given permission.
func (s *SQLiteUserSiteStore) Assign(ctx context.Context, userID, siteID int64, permission string) error {
	if permission == "" {
		permission = "read"
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO user_sites (user_id, site_id, permission) VALUES (?, ?, ?)
		 ON CONFLICT(user_id, site_id) DO UPDATE SET permission = excluded.permission`,
		userID, siteID, permission,
	)
	if err != nil {
		return err
	}
	_, err = res.RowsAffected()
	return err
}

// Unassign revokes a user's access to a site.
func (s *SQLiteUserSiteStore) Unassign(ctx context.Context, userID, siteID int64) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM user_sites WHERE user_id = ? AND site_id = ?`, userID, siteID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// UpdatePermission changes the permission level for an existing assignment.
func (s *SQLiteUserSiteStore) UpdatePermission(ctx context.Context, userID, siteID int64, permission string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE user_sites SET permission = ? WHERE user_id = ? AND site_id = ?`,
		permission, userID, siteID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ListByUser returns all sites a user has been granted access to.
func (s *SQLiteUserSiteStore) ListByUser(ctx context.Context, userID int64) ([]UserSite, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT us.id, us.user_id, us.site_id, us.permission, us.created_at, u.username, si.domain
		 FROM user_sites us
		 JOIN users u ON u.id = us.user_id
		 JOIN sites si ON si.id = us.site_id
		 WHERE us.user_id = ?
		 ORDER BY si.domain`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []UserSite
	for rows.Next() {
		var us UserSite
		if err := rows.Scan(&us.ID, &us.UserID, &us.SiteID, &us.Permission, &us.CreatedAt, &us.Username, &us.Domain); err != nil {
			return nil, err
		}
		out = append(out, us)
	}
	return out, rows.Err()
}

// ListBySite returns all users who have access to a given site.
func (s *SQLiteUserSiteStore) ListBySite(ctx context.Context, siteID int64) ([]UserSite, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT us.id, us.user_id, us.site_id, us.permission, us.created_at, u.username, si.domain
		 FROM user_sites us
		 JOIN users u ON u.id = us.user_id
		 JOIN sites si ON si.id = us.site_id
		 WHERE us.site_id = ?
		 ORDER BY u.username`, siteID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []UserSite
	for rows.Next() {
		var us UserSite
		if err := rows.Scan(&us.ID, &us.UserID, &us.SiteID, &us.Permission, &us.CreatedAt, &us.Username, &us.Domain); err != nil {
			return nil, err
		}
		out = append(out, us)
	}
	return out, rows.Err()
}

// HasAccess checks if a user has any access to a site.
func (s *SQLiteUserSiteStore) HasAccess(ctx context.Context, userID, siteID int64) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM user_sites WHERE user_id = ? AND site_id = ?)`,
		userID, siteID,
	).Scan(&exists)
	return exists, err
}

// GetPermission returns the permission level for a user on a site.
func (s *SQLiteUserSiteStore) GetPermission(ctx context.Context, userID, siteID int64) (string, error) {
	var perm string
	err := s.db.QueryRowContext(ctx,
		`SELECT permission FROM user_sites WHERE user_id = ? AND site_id = ?`,
		userID, siteID,
	).Scan(&perm)
	if err != nil {
		return "", err
	}
	return perm, nil
}
