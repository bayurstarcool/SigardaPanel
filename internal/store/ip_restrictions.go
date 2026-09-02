package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// IPRestriction represents an IP restriction rule for a site.
type IPRestriction struct {
	ID        int64     `json:"id"`
	SiteID    int64     `json:"site_id"`
	IP        string    `json:"ip"`
	Action    string    `json:"action"` // allow, deny
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

// IPRestrictionRepository defines IP restriction persistence operations.
type IPRestrictionRepository interface {
	Create(ctx context.Context, restriction *IPRestriction) error
	List(ctx context.Context, siteID int64) ([]IPRestriction, error)
	Delete(ctx context.Context, id int64) error
	DeleteBySite(ctx context.Context, siteID int64) error
	IsAllowed(ctx context.Context, siteID int64, ip string) (bool, error)
}

// SQLiteIPRestrictionStore implements IPRestrictionRepository with SQLite.
type SQLiteIPRestrictionStore struct {
	db *sql.DB
}

// NewSQLiteIPRestrictionStore creates a new SQLiteIPRestrictionStore.
func NewSQLiteIPRestrictionStore(db *sql.DB) *SQLiteIPRestrictionStore {
	return &SQLiteIPRestrictionStore{db: db}
}

// Create adds a new IP restriction rule.
func (s *SQLiteIPRestrictionStore) Create(ctx context.Context, restriction *IPRestriction) error {
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO ip_restrictions (site_id, ip, action, reason, created_at) VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		restriction.SiteID, restriction.IP, restriction.Action, restriction.Reason)
	if err != nil {
		return fmt.Errorf("create ip restriction: %w", err)
	}
	id, _ := result.LastInsertId()
	restriction.ID = id
	return nil
}

// List retrieves all IP restrictions for a site.
func (s *SQLiteIPRestrictionStore) List(ctx context.Context, siteID int64) ([]IPRestriction, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, site_id, ip, action, reason, created_at FROM ip_restrictions WHERE site_id = ? ORDER BY created_at DESC`,
		siteID)
	if err != nil {
		return nil, fmt.Errorf("list ip restrictions: %w", err)
	}
	defer rows.Close()

	var restrictions []IPRestriction
	for rows.Next() {
		var r IPRestriction
		if err := rows.Scan(&r.ID, &r.SiteID, &r.IP, &r.Action, &r.Reason, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan ip restriction: %w", err)
		}
		restrictions = append(restrictions, r)
	}
	return restrictions, rows.Err()
}

// Delete removes an IP restriction rule.
func (s *SQLiteIPRestrictionStore) Delete(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM ip_restrictions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete ip restriction: %w", err)
	}
	return nil
}

// DeleteBySite removes all IP restrictions for a site.
func (s *SQLiteIPRestrictionStore) DeleteBySite(ctx context.Context, siteID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM ip_restrictions WHERE site_id = ?`, siteID)
	if err != nil {
		return fmt.Errorf("delete ip restrictions by site: %w", err)
	}
	return nil
}

// IsAllowed checks if an IP is allowed for a site.
// Returns true if allowed (no restrictions, or explicitly allowed), false if denied.
func (s *SQLiteIPRestrictionStore) IsAllowed(ctx context.Context, siteID int64, ip string) (bool, error) {
	// Check if there are any restrictions for this site
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM ip_restrictions WHERE site_id = ?`, siteID).Scan(&count)
	if err != nil {
		return true, fmt.Errorf("check ip restrictions: %w", err)
	}

	// No restrictions = allow all
	if count == 0 {
		return true, nil
	}

	// Check if IP is explicitly allowed
	var allowed int
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM ip_restrictions WHERE site_id = ? AND ip = ? AND action = 'allow'`,
		siteID, ip).Scan(&allowed)
	if err != nil {
		return false, fmt.Errorf("check ip allowed: %w", err)
	}
	if allowed > 0 {
		return true, nil
	}

	// Check if IP is explicitly denied
	var denied int
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM ip_restrictions WHERE site_id = ? AND ip = ? AND action = 'deny'`,
		siteID, ip).Scan(&denied)
	if err != nil {
		return false, fmt.Errorf("check ip denied: %w", err)
	}
	if denied > 0 {
		return false, nil
	}

	// IP not in any restriction = deny (whitelist mode)
	return false, nil
}
