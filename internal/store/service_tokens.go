package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// ServiceToken represents a service account API key.
type ServiceToken struct {
	ID         int64
	Name       string
	KeyHash    string
	Scopes     string
	CreatedAt  time.Time
	ExpiresAt  sql.NullTime
	LastUsedAt sql.NullTime
}

// ServiceTokenRepository defines service token persistence operations.
type ServiceTokenRepository interface {
	Create(ctx context.Context, token *ServiceToken) error
	GetByKeyHash(ctx context.Context, keyHash string) (*ServiceToken, error)
	List(ctx context.Context) ([]ServiceToken, error)
	Delete(ctx context.Context, id int64) error
	UpdateLastUsed(ctx context.Context, id int64) error
}

// SQLiteServiceTokenStore implements ServiceTokenRepository.
type SQLiteServiceTokenStore struct {
	db *sql.DB
}

// NewServiceTokenStore returns a new store.
func NewServiceTokenStore(db *sql.DB) *SQLiteServiceTokenStore {
	return &SQLiteServiceTokenStore{db: db}
}

// Create inserts a new service token into the database.
func (s *SQLiteServiceTokenStore) Create(ctx context.Context, token *ServiceToken) error {
	var expiresAt interface{}
	if token.ExpiresAt.Valid {
		expiresAt = token.ExpiresAt.Time.UTC().Format("2006-01-02 15:04:05")
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO service_tokens (name, key_hash, scopes, expires_at) VALUES (?, ?, ?, ?)`,
		token.Name, token.KeyHash, token.Scopes, expiresAt,
	)
	if err != nil {
		return fmt.Errorf("insert service token: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("last insert id: %w", err)
	}
	token.ID = id
	return nil
}

// GetByKeyHash retrieves a service token by its key hash.
// Returns (nil, nil) if not found.
func (s *SQLiteServiceTokenStore) GetByKeyHash(ctx context.Context, keyHash string) (*ServiceToken, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, key_hash, scopes, created_at, expires_at, last_used_at
		 FROM service_tokens WHERE key_hash = ?`, keyHash)
	return scanServiceToken(row)
}

// List returns all service tokens ordered by created_at DESC.
func (s *SQLiteServiceTokenStore) List(ctx context.Context) ([]ServiceToken, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, key_hash, scopes, created_at, expires_at, last_used_at
		 FROM service_tokens ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("query service tokens: %w", err)
	}
	defer rows.Close()

	var out []ServiceToken
	for rows.Next() {
		var t ServiceToken
		if err := scanServiceTokenRows(rows, &t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Delete removes a service token by ID.
func (s *SQLiteServiceTokenStore) Delete(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM service_tokens WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete service token: %w", err)
	}
	return nil
}

// UpdateLastUsed sets last_used_at to the current timestamp.
func (s *SQLiteServiceTokenStore) UpdateLastUsed(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE service_tokens SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("update service token last_used_at: %w", err)
	}
	return nil
}

// ─── helpers ────────────────────────────────────────────────

func scanServiceToken(row *sql.Row) (*ServiceToken, error) {
	var t ServiceToken
	var createdAtStr string
	var expiresAtStr sql.NullString
	var lastUsedAtStr sql.NullString
	err := row.Scan(&t.ID, &t.Name, &t.KeyHash, &t.Scopes, &createdAtStr, &expiresAtStr, &lastUsedAtStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan service token: %w", err)
	}

	t.CreatedAt, err = parseTimestamp(createdAtStr)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}

	if expiresAtStr.Valid {
		et, err := parseTimestamp(expiresAtStr.String)
		if err != nil {
			return nil, fmt.Errorf("parse expires_at: %w", err)
		}
		t.ExpiresAt = sql.NullTime{Time: et, Valid: true}
	}

	if lastUsedAtStr.Valid {
		lu, err := parseTimestamp(lastUsedAtStr.String)
		if err != nil {
			return nil, fmt.Errorf("parse last_used_at: %w", err)
		}
		t.LastUsedAt = sql.NullTime{Time: lu, Valid: true}
	}

	return &t, nil
}

func scanServiceTokenRows(rows *sql.Rows, t *ServiceToken) error {
	var createdAtStr string
	var expiresAtStr sql.NullString
	var lastUsedAtStr sql.NullString
	if err := rows.Scan(&t.ID, &t.Name, &t.KeyHash, &t.Scopes, &createdAtStr, &expiresAtStr, &lastUsedAtStr); err != nil {
		return fmt.Errorf("scan service token row: %w", err)
	}

	var err error
	t.CreatedAt, err = parseTimestamp(createdAtStr)
	if err != nil {
		return fmt.Errorf("parse created_at: %w", err)
	}

	if expiresAtStr.Valid {
		et, err := parseTimestamp(expiresAtStr.String)
		if err != nil {
			return fmt.Errorf("parse expires_at: %w", err)
		}
		t.ExpiresAt = sql.NullTime{Time: et, Valid: true}
	}

	if lastUsedAtStr.Valid {
		lu, err := parseTimestamp(lastUsedAtStr.String)
		if err != nil {
			return fmt.Errorf("parse last_used_at: %w", err)
		}
		t.LastUsedAt = sql.NullTime{Time: lu, Valid: true}
	}

	return nil
}
