package member

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)

// Member represents a registered user in the member system.
type Member struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Company      string    `json:"company"`
	Plan         string    `json:"plan"`
	APIToken     string    `json:"api_token,omitempty"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Session represents a login session.
type Session struct {
	ID        int64     `json:"id"`
	MemberID  int64     `json:"member_id"`
	TokenHash string    `json:"-"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Store handles member database operations.
type Store struct {
	db *sql.DB
}

// NewStore creates a new member store.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// ConnectPostgreSQL connects to PostgreSQL and creates the member database.
func ConnectPostgreSQL(host, port, user, password, dbname string) (*sql.DB, error) {
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	return db, nil
}

// Migrate creates the member tables.
func (s *Store) Migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS members (
			id SERIAL PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			email VARCHAR(255) UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			company VARCHAR(255) DEFAULT '',
			plan VARCHAR(50) DEFAULT 'community',
			api_token VARCHAR(255) UNIQUE,
			status VARCHAR(50) DEFAULT 'active',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS member_sessions (
			id SERIAL PRIMARY KEY,
			member_id INTEGER NOT NULL REFERENCES members(id) ON DELETE CASCADE,
			token_hash VARCHAR(255) NOT NULL,
			expires_at TIMESTAMP NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS member_api_keys (
			id SERIAL PRIMARY KEY,
			member_id INTEGER NOT NULL REFERENCES members(id) ON DELETE CASCADE,
			name VARCHAR(255) NOT NULL,
			token_hash VARCHAR(255) NOT NULL,
			active BOOLEAN DEFAULT true,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			last_used_at TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_members_email ON members(email)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_token ON member_sessions(token_hash)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_expiry ON member_sessions(expires_at)`,
	}

	for _, q := range queries {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("migration: %w", err)
		}
	}
	return nil
}

// GetByEmail finds a member by email.
func (s *Store) GetByEmail(email string) (*Member, error) {
	m := &Member{}
	err := s.db.QueryRow(
		"SELECT id, name, email, password_hash, company, plan, status, created_at, updated_at FROM members WHERE email = $1",
		email,
	).Scan(&m.ID, &m.Name, &m.Email, &m.PasswordHash, &m.Company, &m.Plan, &m.Status, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return m, nil
}

// GetByID finds a member by ID.
func (s *Store) GetByID(id int64) (*Member, error) {
	m := &Member{}
	err := s.db.QueryRow(
		"SELECT id, name, email, company, plan, status, created_at, updated_at FROM members WHERE id = $1",
		id,
	).Scan(&m.ID, &m.Name, &m.Email, &m.Company, &m.Plan, &m.Status, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return m, nil
}

// Create inserts a new member.
func (s *Store) Create(m *Member) error {
	err := s.db.QueryRow(
		"INSERT INTO members (name, email, password_hash, company, plan) VALUES ($1, $2, $3, $4, $5) RETURNING id",
		m.Name, m.Email, m.PasswordHash, m.Company, m.Plan,
	).Scan(&m.ID)
	return err
}

// UpdatePlan updates a member's plan.
func (s *Store) UpdatePlan(id int64, plan string) error {
	_, err := s.db.Exec("UPDATE members SET plan = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2", plan, id)
	return err
}

// CreateSession creates a new session.
func (s *Store) CreateSession(sess *Session) error {
	err := s.db.QueryRow(
		"INSERT INTO member_sessions (member_id, token_hash, expires_at) VALUES ($1, $2, $3) RETURNING id",
		sess.MemberID, sess.TokenHash, sess.ExpiresAt,
	).Scan(&sess.ID)
	return err
}

// GetSession finds a session by token hash.
func (s *Store) GetSession(tokenHash string) (*Session, error) {
	sess := &Session{}
	err := s.db.QueryRow(
		"SELECT id, member_id, token_hash, expires_at FROM member_sessions WHERE token_hash = $1 AND expires_at > NOW()",
		tokenHash,
	).Scan(&sess.ID, &sess.MemberID, &sess.TokenHash, &sess.ExpiresAt)
	if err != nil {
		return nil, err
	}
	return sess, nil
}

// DeleteSession removes a session.
func (s *Store) DeleteSession(id int64) error {
	_, err := s.db.Exec("DELETE FROM member_sessions WHERE id = $1", id)
	return err
}

// CleanupExpired removes expired sessions.
func (s *Store) CleanupExpired() (int64, error) {
	res, err := s.db.Exec("DELETE FROM member_sessions WHERE expires_at < NOW()")
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
