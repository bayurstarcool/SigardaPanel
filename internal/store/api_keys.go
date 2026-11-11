package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// UserAPIKey is a personal API key that authenticates as the user who owns it.
// Only KeyHash is persisted; the raw key is returned once at creation.
type UserAPIKey struct {
	ID         int64
	UserID     int64
	Name       string
	KeyHash    string
	Prefix     string
	CreatedAt  time.Time
	ExpiresAt  sql.NullTime
	LastUsedAt sql.NullTime
}

// MaxAPIKeysPerUser caps how many keys one user may hold.
const MaxAPIKeysPerUser = 20

// SQLiteUserAPIKeyStore persists personal API keys.
type SQLiteUserAPIKeyStore struct {
	db *sql.DB
}

// NewUserAPIKeyStore returns a SQLite-backed API key store.
func NewUserAPIKeyStore(db *sql.DB) *SQLiteUserAPIKeyStore {
	return &SQLiteUserAPIKeyStore{db: db}
}

const apiKeyColumns = `id, user_id, name, key_hash, prefix, created_at, expires_at, last_used_at`

// Create inserts a key and sets its ID and CreatedAt.
func (s *SQLiteUserAPIKeyStore) Create(ctx context.Context, k *UserAPIKey) error {
	var expiresAt any
	if k.ExpiresAt.Valid {
		expiresAt = k.ExpiresAt.Time.UTC().Format("2006-01-02 15:04:05")
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO user_api_keys (user_id, name, key_hash, prefix, expires_at) VALUES (?, ?, ?, ?, ?)`,
		k.UserID, k.Name, k.KeyHash, k.Prefix, expiresAt)
	if err != nil {
		return fmt.Errorf("insert api key: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("last insert id: %w", err)
	}
	k.ID = id
	k.CreatedAt = time.Now().UTC()
	return nil
}

// GetByKeyHash returns the key with the given hash, or (nil, nil).
func (s *SQLiteUserAPIKeyStore) GetByKeyHash(ctx context.Context, hash string) (*UserAPIKey, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+apiKeyColumns+` FROM user_api_keys WHERE key_hash = ?`, hash)
	k, err := scanAPIKey(row.Scan)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return k, err
}

// ListByUser returns a user's keys, newest first.
func (s *SQLiteUserAPIKeyStore) ListByUser(ctx context.Context, userID int64) ([]UserAPIKey, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+apiKeyColumns+` FROM user_api_keys WHERE user_id = ? ORDER BY id DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("query api keys: %w", err)
	}
	defer rows.Close()
	out := []UserAPIKey{}
	for rows.Next() {
		k, err := scanAPIKey(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, *k)
	}
	return out, rows.Err()
}

// CountByUser returns how many keys a user holds.
func (s *SQLiteUserAPIKeyStore) CountByUser(ctx context.Context, userID int64) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_api_keys WHERE user_id = ?`, userID).Scan(&n)
	return n, err
}

// DeleteForUser removes a key only if it belongs to userID. It reports
// whether a row was deleted, so callers can answer 404 for foreign keys.
func (s *SQLiteUserAPIKeyStore) DeleteForUser(ctx context.Context, id, userID int64) (bool, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM user_api_keys WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return false, fmt.Errorf("delete api key: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// UpdateLastUsed stamps last_used_at with the current time.
func (s *SQLiteUserAPIKeyStore) UpdateLastUsed(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE user_api_keys SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
	return err
}

func scanAPIKey(scan func(dest ...any) error) (*UserAPIKey, error) {
	var k UserAPIKey
	var created string
	var expires, lastUsed sql.NullString
	if err := scan(&k.ID, &k.UserID, &k.Name, &k.KeyHash, &k.Prefix, &created, &expires, &lastUsed); err != nil {
		if err == sql.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("scan api key: %w", err)
	}
	var err error
	if k.CreatedAt, err = parseTimestamp(created); err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	if expires.Valid && expires.String != "" {
		t, err := parseTimestamp(expires.String)
		if err != nil {
			return nil, fmt.Errorf("parse expires_at: %w", err)
		}
		k.ExpiresAt = sql.NullTime{Time: t, Valid: true}
	}
	if lastUsed.Valid && lastUsed.String != "" {
		t, err := parseTimestamp(lastUsed.String)
		if err != nil {
			return nil, fmt.Errorf("parse last_used_at: %w", err)
		}
		k.LastUsedAt = sql.NullTime{Time: t, Valid: true}
	}
	return &k, nil
}
