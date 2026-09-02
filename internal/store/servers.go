package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Server represents a registered VPS target server.
type Server struct {
	ID               int64
	Name             string
	Hostname         string
	AgentURL         sql.NullString
	IPAddress        sql.NullString
	IPv6             sql.NullString
	SSHPort          int
	Description      string
	Status           string
	Provider         sql.NullString
	AgentToken       string
	LastHealthCheck  sql.NullTime
	HealthStatus     string
	HealthLatencyMs  int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// ServerRepository defines server persistence operations.
type ServerRepository interface {
	Create(ctx context.Context, s *Server) error
	GetByID(ctx context.Context, id int64) (*Server, error)
	List(ctx context.Context) ([]Server, error)
	Update(ctx context.Context, s *Server) error
	UpdateStatus(ctx context.Context, id int64, status string) error
	UpdateHealth(ctx context.Context, id int64, status string, latencyMs int64) error
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
		`INSERT INTO servers (name, hostname, agent_url, ip_address, ipv6, ssh_port, description, status, provider, agent_token, health_status) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		srv.Name, srv.Hostname, srv.AgentURL, srv.IPAddress, srv.IPv6, srv.SSHPort, srv.Description, srv.Status, srv.Provider, srv.AgentToken, srv.HealthStatus,
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
		 last_health_check, health_status, health_latency_ms, created_at, updated_at
		 FROM servers WHERE id = ?`, id)
	return scanServer(row)
}

// List returns all servers ordered by id ASC.
func (s *SQLiteServerStore) List(ctx context.Context) ([]Server, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, hostname, agent_url, ip_address, ipv6, ssh_port, description, status, provider, agent_token,
		 last_health_check, health_status, health_latency_ms, created_at, updated_at
		 FROM servers ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("query servers: %w", err)
	}
	defer rows.Close()

	var servers []Server
	for rows.Next() {
		var srv Server
		if err := rows.Scan(&srv.ID, &srv.Name, &srv.Hostname, &srv.AgentURL, &srv.IPAddress, &srv.IPv6, &srv.SSHPort, &srv.Description, &srv.Status, &srv.Provider, &srv.AgentToken, &srv.LastHealthCheck, &srv.HealthStatus, &srv.HealthLatencyMs, &srv.CreatedAt, &srv.UpdatedAt); err != nil {
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
	err := row.Scan(&srv.ID, &srv.Name, &srv.Hostname, &srv.AgentURL, &srv.IPAddress, &srv.IPv6, &srv.SSHPort, &srv.Description, &srv.Status, &srv.Provider, &srv.AgentToken, &srv.LastHealthCheck, &srv.HealthStatus, &srv.HealthLatencyMs, &srv.CreatedAt, &srv.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan server: %w", err)
	}
	return &srv, nil
}
