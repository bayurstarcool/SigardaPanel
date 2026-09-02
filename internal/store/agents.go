package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// RegistrationToken represents a one-time registration token for agent install.
type RegistrationToken struct {
	ID        int64
	TokenHash string
	CreatedBy sql.NullInt64
	ExpiresAt time.Time
	UsedAt    sql.NullTime
	CreatedAt time.Time
}

// RegistrationTokenRepository defines persistence for registration tokens.
type RegistrationTokenRepository interface {
	Create(ctx context.Context, rt *RegistrationToken) error
	GetByHash(ctx context.Context, tokenHash string) (*RegistrationToken, error)
	MarkUsed(ctx context.Context, id int64) error
}

// SQLiteRegistrationTokenStore implements RegistrationTokenRepository.
type SQLiteRegistrationTokenStore struct {
	db *sql.DB
}

// NewRegistrationTokenStore returns a new store backed by db.
func NewRegistrationTokenStore(db *sql.DB) *SQLiteRegistrationTokenStore {
	return &SQLiteRegistrationTokenStore{db: db}
}

// Create inserts a new registration token.
func (s *SQLiteRegistrationTokenStore) Create(ctx context.Context, rt *RegistrationToken) error {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO registration_tokens (token_hash, created_by, expires_at) VALUES (?, ?, ?)`,
		rt.TokenHash, rt.CreatedBy, rt.ExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("insert registration token: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("last insert id: %w", err)
	}
	rt.ID = id
	return nil
}

// GetByHash retrieves a token by its SHA-256 hash.
func (s *SQLiteRegistrationTokenStore) GetByHash(ctx context.Context, tokenHash string) (*RegistrationToken, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, token_hash, created_by, expires_at, used_at, created_at
		 FROM registration_tokens WHERE token_hash = ?`, tokenHash)
	var rt RegistrationToken
	err := row.Scan(&rt.ID, &rt.TokenHash, &rt.CreatedBy, &rt.ExpiresAt, &rt.UsedAt, &rt.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan registration token: %w", err)
	}
	return &rt, nil
}

// MarkUsed atomically claims an unused token.
func (s *SQLiteRegistrationTokenStore) MarkUsed(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE registration_tokens SET used_at = CURRENT_TIMESTAMP WHERE id = ? AND used_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("mark registration token used: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("registration token rows affected: %w", err)
	}
	if rows != 1 {
		return fmt.Errorf("registration token already used or missing")
	}
	return nil
}

// ConsumeRegistrationTokenAndCreateServer claims a one-time token and creates
// its server record in one transaction. Any insert failure rolls back token use.
func (s *Store) ConsumeRegistrationTokenAndCreateServer(ctx context.Context, tokenID int64, srv *Server) error {
	if srv == nil || srv.Name == "" || srv.Hostname == "" || srv.AgentToken == "" {
		return fmt.Errorf("invalid server registration")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin agent registration: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`UPDATE registration_tokens SET used_at = CURRENT_TIMESTAMP WHERE id = ? AND used_at IS NULL AND expires_at > CURRENT_TIMESTAMP`, tokenID)
	if err != nil {
		return fmt.Errorf("claim registration token: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("registration token rows affected: %w", err)
	}
	if rows != 1 {
		return fmt.Errorf("registration token already used, expired, or missing")
	}

	if srv.Status == "" {
		srv.Status = "pending"
	}
	if srv.HealthStatus == "" {
		srv.HealthStatus = "unknown"
	}
	res, err = tx.ExecContext(ctx,
		`INSERT INTO servers (name, hostname, agent_url, ip_address, ipv6, ssh_port, description, status, provider, agent_token, health_status) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		srv.Name, srv.Hostname, srv.AgentURL, srv.IPAddress, srv.IPv6, srv.SSHPort, srv.Description, srv.Status, srv.Provider, srv.AgentToken, srv.HealthStatus)
	if err != nil {
		return fmt.Errorf("insert registered server: %w", err)
	}
	srv.ID, err = res.LastInsertId()
	if err != nil {
		return fmt.Errorf("registered server id: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit agent registration: %w", err)
	}
	return nil
}
