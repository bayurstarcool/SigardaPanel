package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"

	"sigardapanel/internal/agenttls"
)

// Server represents a registered VPS target server.
type Server struct {
	ID              int64
	Name            string
	Hostname        string
	AgentURL        sql.NullString
	IPAddress       sql.NullString
	IPv6            sql.NullString
	SSHPort         int
	Description     string
	Status          string
	Provider        sql.NullString
	AgentToken      string
	LastHealthCheck sql.NullTime
	HealthStatus    string
	HealthLatencyMs int64
	AgentVersion    sql.NullString // reported by the agent's /health
	// AgentTLSFingerprint pins the remote agent's certificate (hex SHA-256
	// of the DER certificate); https agent URLs are refused without it.
	AgentTLSFingerprint string
	// Rebound is set by agent registration when an existing server (same
	// agent address) was re-bound instead of a new one created.
	Rebound bool
	// PreviousAgentToken (registration only, not stored) proves the caller
	// controls the server already registered at the same agent address.
	PreviousAgentToken string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// ServerRepository defines server persistence operations.
type ServerRepository interface {
	Create(ctx context.Context, s *Server) error
	GetByID(ctx context.Context, id int64) (*Server, error)
	List(ctx context.Context) ([]Server, error)
	Update(ctx context.Context, s *Server) error
	UpdateStatus(ctx context.Context, id int64, status string) error
	UpdateHealth(ctx context.Context, id int64, status string, latencyMs int64) error
	UpdateAgentToken(ctx context.Context, id int64, token string) error
	UpdateAgentVersion(ctx context.Context, id int64, version string) error
	// SetAgentTLS stores the agent's https URL and certificate pin.
	SetAgentTLS(ctx context.Context, id int64, agentURL, fingerprint string) error
	// TLSPinForAddr returns the pin of the https agent dialed at addr.
	TLSPinForAddr(ctx context.Context, addr string) (string, error)
	Delete(ctx context.Context, id int64) error
}

// SQLiteServerStore implements ServerRepository against a SQLite database.
type SQLiteServerStore struct {
	db *sql.DB
}

// NewServerStore returns a SQLite-backed server store.
func NewServerStore(db *sql.DB) *SQLiteServerStore {
	return &SQLiteServerStore{db: db}
}

// Create inserts a new server into the database.
// Sets Status to "pending" if empty.
func (s *SQLiteServerStore) Create(ctx context.Context, srv *Server) error {
	if srv.Status == "" {
		srv.Status = "pending"
	}
	if srv.HealthStatus == "" {
		srv.HealthStatus = "unknown"
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO servers (name, hostname, agent_url, ip_address, ipv6, ssh_port, description, status, provider, agent_token, health_status, agent_tls_fingerprint) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		srv.Name, srv.Hostname, srv.AgentURL, srv.IPAddress, srv.IPv6, srv.SSHPort, srv.Description, srv.Status, srv.Provider, srv.AgentToken, srv.HealthStatus, srv.AgentTLSFingerprint,
	)
	if err != nil {
		return fmt.Errorf("insert server: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("last insert id: %w", err)
	}
	srv.ID = id
	return nil
}

// GetByID retrieves a server by ID.
// Returns (nil, nil) if not found.
func (s *SQLiteServerStore) GetByID(ctx context.Context, id int64) (*Server, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, hostname, agent_url, ip_address, ipv6, ssh_port, description, status, provider, agent_token,
		 last_health_check, health_status, health_latency_ms, agent_version, agent_tls_fingerprint, created_at, updated_at
		 FROM servers WHERE id = ?`, id)
	return scanServer(row)
}

// List returns all servers ordered by id ASC.
func (s *SQLiteServerStore) List(ctx context.Context) ([]Server, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, hostname, agent_url, ip_address, ipv6, ssh_port, description, status, provider, agent_token,
		 last_health_check, health_status, health_latency_ms, agent_version, agent_tls_fingerprint, created_at, updated_at
		 FROM servers ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("query servers: %w", err)
	}
	defer rows.Close()

	var servers []Server
	for rows.Next() {
		var srv Server
		if err := rows.Scan(&srv.ID, &srv.Name, &srv.Hostname, &srv.AgentURL, &srv.IPAddress, &srv.IPv6, &srv.SSHPort, &srv.Description, &srv.Status, &srv.Provider, &srv.AgentToken, &srv.LastHealthCheck, &srv.HealthStatus, &srv.HealthLatencyMs, &srv.AgentVersion, &srv.AgentTLSFingerprint, &srv.CreatedAt, &srv.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan server: %w", err)
		}
		servers = append(servers, srv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return servers, nil
}

// Update modifies a server's name, hostname, ip_address, and status.
func (s *SQLiteServerStore) Update(ctx context.Context, srv *Server) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE servers SET name = ?, hostname = ?, agent_url = ?, ip_address = ?, ipv6 = ?, ssh_port = ?, description = ?, status = ?, provider = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		srv.Name, srv.Hostname, srv.AgentURL, srv.IPAddress, srv.IPv6, srv.SSHPort, srv.Description, srv.Status, srv.Provider, srv.ID,
	)
	if err != nil {
		return fmt.Errorf("update server: %w", err)
	}
	return nil
}

// UpdateHealth updates the health check fields of a server.
func (s *SQLiteServerStore) UpdateHealth(ctx context.Context, id int64, status string, latencyMs int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE servers SET health_status = ?, health_latency_ms = ?, last_health_check = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		status, latencyMs, id)
	if err != nil {
		return fmt.Errorf("update server health: %w", err)
	}
	return nil
}

// UpdateAgentToken stores a new agent token (Update deliberately leaves the
// token alone so ordinary edits cannot clear it).
func (s *SQLiteServerStore) UpdateAgentToken(ctx context.Context, id int64, token string) error {
	if token == "" {
		return fmt.Errorf("update agent token: empty token")
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE servers SET agent_token = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, token, id)
	if err != nil {
		return fmt.Errorf("update agent token: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("update agent token: server %d not found", id)
	}
	return nil
}

// UpdateAgentVersion records the version the agent reported.
func (s *SQLiteServerStore) UpdateAgentVersion(ctx context.Context, id int64, version string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE servers SET agent_version = ? WHERE id = ?`, sql.NullString{String: version, Valid: version != ""}, id)
	if err != nil {
		return fmt.Errorf("update agent version: %w", err)
	}
	return nil
}

// UpdateStatus updates only the status field of a server.
func (s *SQLiteServerStore) UpdateStatus(ctx context.Context, id int64, status string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE servers SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		status, id)
	if err != nil {
		return fmt.Errorf("update server status: %w", err)
	}
	return nil
}

// Delete removes a server by ID.
func (s *SQLiteServerStore) Delete(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM servers WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete server: %w", err)
	}
	return nil
}

func scanServer(row *sql.Row) (*Server, error) {
	var srv Server
	err := row.Scan(&srv.ID, &srv.Name, &srv.Hostname, &srv.AgentURL, &srv.IPAddress, &srv.IPv6, &srv.SSHPort, &srv.Description, &srv.Status, &srv.Provider, &srv.AgentToken, &srv.LastHealthCheck, &srv.HealthStatus, &srv.HealthLatencyMs, &srv.AgentVersion, &srv.AgentTLSFingerprint, &srv.CreatedAt, &srv.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan server: %w", err)
	}
	return &srv, nil
}

// SetAgentTLS stores the https agent URL and its certificate pin together.
func (s *SQLiteServerStore) SetAgentTLS(ctx context.Context, id int64, agentURL, fingerprint string) error {
	fp := agenttls.Normalize(fingerprint)
	u, err := url.Parse(agentURL)
	if fp == "" || err != nil || u.Scheme != "https" {
		return fmt.Errorf("set agent TLS: an https URL and a SHA-256 fingerprint are required")
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE servers SET agent_url = ?, agent_tls_fingerprint = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		agentURL, fp, id); err != nil {
		return fmt.Errorf("set agent TLS: %w", err)
	}
	return nil
}

// TLSPinForAddr returns the stored pin of the https agent whose URL dials
// addr ("host:port"), or "" when none is stored.
func (s *SQLiteServerStore) TLSPinForAddr(ctx context.Context, addr string) (string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT agent_url, agent_tls_fingerprint FROM servers WHERE agent_tls_fingerprint != '' AND agent_url LIKE 'https://%'`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	want := strings.ToLower(addr)
	for rows.Next() {
		var u sql.NullString
		var fp string
		if err := rows.Scan(&u, &fp); err != nil {
			return "", err
		}
		if u.Valid && agenttls.AddrOf(u.String) == want {
			return fp, nil
		}
	}
	return "", rows.Err()
}
