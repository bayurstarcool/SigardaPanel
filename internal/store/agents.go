package store

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"strings"
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
		// Canonical UTC text: "expires_at > CURRENT_TIMESTAMP" is a text
		// comparison, which time.String() output breaks on non-UTC hosts.
		rt.TokenHash, rt.CreatedBy, sqliteUTC(rt.ExpiresAt),
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

// ErrServerAlreadyRegistered: a server with the same agent address exists
// and the registration did not prove it holds that server's agent token.
var ErrServerAlreadyRegistered = errors.New("a server is already registered at this address")

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
	// Re-running the installer on a server the panel already knows (same
	// agent address) re-binds that server: its sites, jobs and settings stay
	// attached and the new agent token replaces the old one, instead of a
	// duplicate row while the sites keep pointing at a dead token. Only the
	// holder of the current agent token may do that (the installer reads it
	// from agent.env): anyone else with a registration token could otherwise
	// take over another server's record by naming its address.
	if srv.AgentURL.Valid && srv.AgentURL.String != "" {
		var existingID int64
		var existingToken string
		// The same address may have been registered over http before TLS.
		httpURL, httpsURL := agentURLVariants(srv.AgentURL.String)
		err := tx.QueryRowContext(ctx, `SELECT id, agent_token FROM servers WHERE agent_url IN (?, ?) ORDER BY id LIMIT 1`, httpURL, httpsURL).Scan(&existingID, &existingToken)
		if err == nil && existingID > 0 {
			if srv.PreviousAgentToken == "" || subtle.ConstantTimeCompare([]byte(srv.PreviousAgentToken), []byte(existingToken)) != 1 {
				return ErrServerAlreadyRegistered
			}
			if _, err := tx.ExecContext(ctx,
				`UPDATE servers SET hostname = ?, ip_address = ?, agent_token = ?, status = ?, health_status = ?, agent_url = ?, agent_tls_fingerprint = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
				srv.Hostname, srv.IPAddress, srv.AgentToken, srv.Status, srv.HealthStatus, srv.AgentURL.String, srv.AgentTLSFingerprint, existingID); err != nil {
				return fmt.Errorf("re-bind registered server: %w", err)
			}
			srv.ID = existingID
			srv.Rebound = true
			return tx.Commit()
		}
	}
	res, err = tx.ExecContext(ctx,
		`INSERT INTO servers (name, hostname, agent_url, ip_address, ipv6, ssh_port, description, status, provider, agent_token, health_status, agent_tls_fingerprint) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		srv.Name, srv.Hostname, srv.AgentURL, srv.IPAddress, srv.IPv6, srv.SSHPort, srv.Description, srv.Status, srv.Provider, srv.AgentToken, srv.HealthStatus, srv.AgentTLSFingerprint)
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

// agentURLVariants returns the http and https spellings of an agent URL.
func agentURLVariants(u string) (string, string) {
	rest := strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
	return "http://" + rest, "https://" + rest
}
