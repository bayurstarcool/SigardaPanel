package store

import (
	"database/sql"
	"fmt"
	"time"
)

type Member struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Company      string    `json:"company"`
	Plan         string    `json:"plan"`
	LicenseKey   string    `json:"license_key,omitempty"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Session struct {
	ID        int64     `json:"id"`
	MemberID  int64     `json:"member_id"`
	Token     string    `json:"-"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS members (
			id SERIAL PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			email VARCHAR(255) UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			company VARCHAR(255) DEFAULT '',
			plan VARCHAR(50) DEFAULT 'community',
			license_key VARCHAR(255) UNIQUE,
			status VARCHAR(50) DEFAULT 'active',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id SERIAL PRIMARY KEY,
			member_id INTEGER NOT NULL REFERENCES members(id) ON DELETE CASCADE,
			token VARCHAR(255) NOT NULL,
			expires_at TIMESTAMP NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_members_email ON members(email)`,
		`CREATE INDEX IF NOT EXISTS idx_members_license ON members(license_key)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_token ON sessions(token)`,
	}

	for _, q := range queries {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("migration: %w", err)
		}
	}
	return nil
}

func (s *Store) GetByEmail(email string) (*Member, error) {
	m := &Member{}
	err := s.db.QueryRow(
		"SELECT id, name, email, password_hash, company, plan, COALESCE(license_key,''), status, created_at, updated_at FROM members WHERE email = $1",
		email,
	).Scan(&m.ID, &m.Name, &m.Email, &m.PasswordHash, &m.Company, &m.Plan, &m.LicenseKey, &m.Status, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (s *Store) GetByID(id int64) (*Member, error) {
	m := &Member{}
	err := s.db.QueryRow(
		"SELECT id, name, email, company, plan, COALESCE(license_key,''), status, created_at, updated_at FROM members WHERE id = $1",
		id,
	).Scan(&m.ID, &m.Name, &m.Email, &m.Company, &m.Plan, &m.LicenseKey, &m.Status, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (s *Store) GetByLicenseKey(key string) (*Member, error) {
	m := &Member{}
	err := s.db.QueryRow(
		"SELECT id, name, email, company, plan, license_key, status FROM members WHERE license_key = $1",
		key,
	).Scan(&m.ID, &m.Name, &m.Email, &m.Company, &m.Plan, &m.LicenseKey, &m.Status)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (s *Store) Create(m *Member) error {
	err := s.db.QueryRow(
		"INSERT INTO members (name, email, password_hash, company, plan) VALUES ($1, $2, $3, $4, $5) RETURNING id",
		m.Name, m.Email, m.PasswordHash, m.Company, m.Plan,
	).Scan(&m.ID)
	return err
}

func (s *Store) UpdateProfile(id int64, name, company string) error {
	_, err := s.db.Exec("UPDATE members SET name=$1, company=$2, updated_at=NOW() WHERE id=$3", name, company, id)
	return err
}

func (s *Store) UpdatePlan(id int64, plan string) error {
	_, err := s.db.Exec("UPDATE members SET plan=$1, updated_at=NOW() WHERE id=$2", plan, id)
	return err
}

func (s *Store) SetLicenseKey(id int64, key string) error {
	_, err := s.db.Exec("UPDATE members SET license_key=$1, updated_at=NOW() WHERE id=$2", key, id)
	return err
}

func (s *Store) CreateSession(sess *Session) error {
	err := s.db.QueryRow(
		"INSERT INTO sessions (member_id, token, expires_at) VALUES ($1, $2, $3) RETURNING id",
		sess.MemberID, sess.Token, sess.ExpiresAt,
	).Scan(&sess.ID)
	return err
}

func (s *Store) GetSession(token string) (*Session, error) {
	sess := &Session{}
	err := s.db.QueryRow(
		"SELECT id, member_id, token, expires_at FROM sessions WHERE token = $1 AND expires_at > NOW()",
		token,
	).Scan(&sess.ID, &sess.MemberID, &sess.Token, &sess.ExpiresAt)
	if err != nil {
		return nil, err
	}
	return sess, nil
}

func (s *Store) DeleteSession(id int64) error {
	_, err := s.db.Exec("DELETE FROM sessions WHERE id = $1", id)
	return err
}
