package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// License represents an active or inactive license for the panel.
type License struct {
	ID          int64
	Key         string
	Tier        string
	ActivatedAt sql.NullTime
	ExpiresAt   sql.NullTime
	MaxServers  int
	Features    string
	CreatedAt   time.Time
}

// LicenseRepository defines license persistence operations.
type LicenseRepository interface {
	GetActive(ctx context.Context) (*License, error)
	Activate(ctx context.Context, key, tier string, maxServers int) (*License, error)
	Deactivate(ctx context.Context) error
	GetByKeyHash(ctx context.Context, keyHash string) (*License, error)
}

// SQLiteLicenseStore implements LicenseRepository backed by SQLite.
type SQLiteLicenseStore struct {
	db *sql.DB
}

// NewLicenseStore creates a new SQLiteLicenseStore.
func NewLicenseStore(db *sql.DB) *SQLiteLicenseStore {
	return &SQLiteLicenseStore{db: db}
}

// GetActive returns the currently active license, or nil if none.
func (s *SQLiteLicenseStore) GetActive(ctx context.Context) (*License, error) {
	var l License
	var createdAtStr string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, key, tier, activated_at, expires_at, max_servers, features, created_at
		 FROM licenses
		 WHERE activated_at IS NOT NULL AND (expires_at IS NULL OR expires_at > datetime('now'))
		 ORDER BY activated_at DESC LIMIT 1`,
	).Scan(&l.ID, &l.Key, &l.Tier, &l.ActivatedAt, &l.ExpiresAt, &l.MaxServers, &l.Features, &createdAtStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get active license: %w", err)
	}
	l.CreatedAt, err = parseTimestamp(createdAtStr)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	return &l, nil
}

// Activate inserts or updates a license as active. If the key already
// exists it is updated; otherwise a new row is inserted. All other
// licenses are deactivated first.
func (s *SQLiteLicenseStore) Activate(ctx context.Context, key, tier string, maxServers int) (*License, error) {
	// Try update first
	res, err := s.db.ExecContext(ctx,
		`UPDATE licenses SET tier = ?, max_servers = ?, activated_at = datetime('now')
		 WHERE key = ?`, tier, maxServers, key)
	if err != nil {
		return nil, fmt.Errorf("activate license update: %w", err)
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		// Deactivate others
		if _, err := s.db.ExecContext(ctx,
			`UPDATE licenses SET activated_at = NULL WHERE key != ?`, key); err != nil {
			return nil, fmt.Errorf("deactivate other licenses: %w", err)
		}
		return s.getByKey(ctx, key)
	}

	// Deactivate any currently active license
	if _, err := s.db.ExecContext(ctx,
		`UPDATE licenses SET activated_at = NULL WHERE activated_at IS NOT NULL`); err != nil {
		return nil, fmt.Errorf("deactivate existing: %w", err)
	}

	// Insert new
	ins, err := s.db.ExecContext(ctx,
		`INSERT INTO licenses (key, tier, max_servers, activated_at, created_at)
		 VALUES (?, ?, ?, datetime('now'), datetime('now'))`, key, tier, maxServers)
	if err != nil {
		return nil, fmt.Errorf("insert license: %w", err)
	}
	id, err := ins.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("last insert id: %w", err)
	}

	var l License
	var createdAtStr string
	err = s.db.QueryRowContext(ctx,
		`SELECT id, key, tier, activated_at, expires_at, max_servers, features, created_at
		 FROM licenses WHERE id = ?`, id,
	).Scan(&l.ID, &l.Key, &l.Tier, &l.ActivatedAt, &l.ExpiresAt, &l.MaxServers, &l.Features, &createdAtStr)
	if err != nil {
		return nil, fmt.Errorf("fetch activated license: %w", err)
	}
	l.CreatedAt, err = parseTimestamp(createdAtStr)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	return &l, nil
}

// Deactivate clears activated_at on all licenses (revert to free).
func (s *SQLiteLicenseStore) Deactivate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE licenses SET activated_at = NULL WHERE activated_at IS NOT NULL`)
	if err != nil {
		return fmt.Errorf("deactivate licenses: %w", err)
	}
	return nil
}

// GetByKeyHash retrieves a license by key. Reserved for future key validation.
func (s *SQLiteLicenseStore) GetByKeyHash(ctx context.Context, keyHash string) (*License, error) {
	return s.getByKey(ctx, keyHash)
}

func (s *SQLiteLicenseStore) getByKey(ctx context.Context, key string) (*License, error) {
	var l License
	var createdAtStr string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, key, tier, activated_at, expires_at, max_servers, features, created_at
		 FROM licenses WHERE key = ? LIMIT 1`, key,
	).Scan(&l.ID, &l.Key, &l.Tier, &l.ActivatedAt, &l.ExpiresAt, &l.MaxServers, &l.Features, &createdAtStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get license by key: %w", err)
	}
	l.CreatedAt, err = parseTimestamp(createdAtStr)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	return &l, nil
}
