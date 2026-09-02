package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"sigardapanel/internal/auth"
)

// User represents a panel user.
type User struct {
	ID                int64
	Username          string
	Email             string
	PasswordHash      string
	Role              string
	CreatedBy         sql.NullInt64
	CreatedAt         time.Time
	DiskLimit         sql.NullInt64 // NULL = unlimited, value = max GB
	MaxSites          sql.NullInt64 // NULL = unlimited, value = max sites
	PlanID            sql.NullInt64 // NULL = no plan assigned
	TOTPSecret        string        // TOTP secret for 2FA
	TOTPEnabled       bool          // Whether 2FA is enabled
	PasswordChangedAt sql.NullTime  // NULL = never changed (force change)
}

// UserRepository defines user persistence operations.
type UserRepository interface {
	Create(ctx context.Context, user *User) error
	GetByID(ctx context.Context, id int64) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	GetByUsername(ctx context.Context, username string) (*User, error)
	UpdatePassword(ctx context.Context, userID int64, newHash string) error
	List(ctx context.Context, filter UserListFilter) ([]User, error)
	UpdateRole(ctx context.Context, userID int64, role string) error
	UpdateEmail(ctx context.Context, userID int64, email string) error
	Delete(ctx context.Context, userID int64) error
	NullifyCreatedBy(ctx context.Context, userID int64) error
	UpdateDiskLimit(ctx context.Context, userID int64, diskLimit sql.NullInt64) error
	UpdateMaxSites(ctx context.Context, userID int64, maxSites sql.NullInt64) error
	UpdatePlan(ctx context.Context, userID int64, planID sql.NullInt64) error
}

// UserListFilter controls which users are returned by List.
type UserListFilter struct {
	CreatedBy int64 // 0 means no filter
}

// SQLiteUserStore implements UserRepository against a SQLite database.
type SQLiteUserStore struct {
	db *sql.DB
}

// NewUserStore returns a SQLite-backed user store.
func NewUserStore(db *sql.DB) *SQLiteUserStore {
	return &SQLiteUserStore{db: db}
}

// Create inserts a new user into the database.
func (s *SQLiteUserStore) Create(ctx context.Context, user *User) error {
	if !auth.IsValidRole(user.Role) {
		return fmt.Errorf("invalid role %q: must be super_admin, admin or user", user.Role)
	}

	hash, err := auth.HashPassword(user.PasswordHash)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	user.PasswordHash = hash

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO users (username, email, password_hash, role, created_by, disk_limit, max_sites, plan_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		user.Username, user.Email, user.PasswordHash, user.Role, user.CreatedBy, user.DiskLimit, user.MaxSites, user.PlanID,
	)
	if err != nil {
		return fmt.Errorf("insert user: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("last insert id: %w", err)
	}
	user.ID = id
	return nil
}

// GetByID retrieves a user by ID.
func (s *SQLiteUserStore) GetByID(ctx context.Context, id int64) (*User, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, username, email, password_hash, role, created_by, created_at, disk_limit, max_sites, plan_id, COALESCE(totp_secret, ''), COALESCE(totp_enabled, 0), password_changed_at FROM users WHERE id = ?`, id)
	return scanUser(row)
}

// GetByEmail retrieves a user by email.
func (s *SQLiteUserStore) GetByEmail(ctx context.Context, email string) (*User, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, username, email, password_hash, role, created_by, created_at, disk_limit, max_sites, plan_id, COALESCE(totp_secret, ''), COALESCE(totp_enabled, 0), password_changed_at FROM users WHERE email = ?`, email)
	return scanUser(row)
}

// GetByUsername retrieves a user by username.
func (s *SQLiteUserStore) GetByUsername(ctx context.Context, username string) (*User, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, username, email, password_hash, role, created_by, created_at, disk_limit, max_sites, plan_id, COALESCE(totp_secret, ''), COALESCE(totp_enabled, 0), password_changed_at FROM users WHERE username = ?`, username)
	return scanUser(row)
}

// UpdatePassword updates the password hash for a user and marks password as changed.
func (s *SQLiteUserStore) UpdatePassword(ctx context.Context, userID int64, newHash string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET password_hash = ?, password_changed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		newHash, userID,
	)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	return nil
}

// UpdateEmail changes a user's email.
func (s *SQLiteUserStore) UpdateEmail(ctx context.Context, userID int64, email string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET email = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, email, userID)
	if err != nil {
		return fmt.Errorf("update email: %w", err)
	}
	return nil
}

// UpdateDiskLimit changes a user's disk limit (NULL = unlimited).
func (s *SQLiteUserStore) UpdateDiskLimit(ctx context.Context, userID int64, diskLimit sql.NullInt64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET disk_limit = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, diskLimit, userID)
	if err != nil {
		return fmt.Errorf("update disk limit: %w", err)
	}
	return nil
}

// UpdateMaxSites changes a user's max sites (NULL = unlimited).
func (s *SQLiteUserStore) UpdateMaxSites(ctx context.Context, userID int64, maxSites sql.NullInt64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET max_sites = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, maxSites, userID)
	if err != nil {
		return fmt.Errorf("update max sites: %w", err)
	}
	return nil
}

// Delete removes a user by ID.
func (s *SQLiteUserStore) Delete(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, userID)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return nil
}

// NullifyCreatedBy sets created_by to NULL for all users created by the given user.
func (s *SQLiteUserStore) NullifyCreatedBy(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET created_by = NULL WHERE created_by = ?`, userID)
	return err
}

func scanUser(row *sql.Row) (*User, error) {
	var u User
	var totpSecret sql.NullString
	var totpEnabled sql.NullInt64
	err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Role, &u.CreatedBy, &u.CreatedAt, &u.DiskLimit, &u.MaxSites, &u.PlanID, &totpSecret, &totpEnabled, &u.PasswordChangedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan user: %w", err)
	}
	u.TOTPSecret = totpSecret.String
	u.TOTPEnabled = totpEnabled.Int64 == 1
	return &u, nil
}

// List retrieves users, optionally filtered by creator.
func (s *SQLiteUserStore) List(ctx context.Context, filter UserListFilter) ([]User, error) {
	query := `SELECT id, username, email, password_hash, role, created_by, created_at, disk_limit, max_sites, plan_id, COALESCE(totp_secret, ''), COALESCE(totp_enabled, 0), password_changed_at FROM users`
	var args []any

	if filter.CreatedBy > 0 {
		query += ` WHERE created_by = ?`
		args = append(args, filter.CreatedBy)
	}

	query += ` ORDER BY id`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		var u User
		var totpSecret sql.NullString
		var totpEnabled sql.NullInt64
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Role, &u.CreatedBy, &u.CreatedAt, &u.DiskLimit, &u.MaxSites, &u.PlanID, &totpSecret, &totpEnabled, &u.PasswordChangedAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		u.TOTPSecret = totpSecret.String
		u.TOTPEnabled = totpEnabled.Int64 == 1
		users = append(users, u)
	}
	return users, rows.Err()
}

// UpdateRole changes a user's role.
func (s *SQLiteUserStore) UpdateRole(ctx context.Context, userID int64, role string) error {
	if !auth.IsValidRole(role) {
		return fmt.Errorf("invalid role %q: must be super_admin, admin or user", role)
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET role = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, role, userID)
	if err != nil {
		return fmt.Errorf("update role: %w", err)
	}
	return nil
}

// UpdatePlan assigns or removes a plan for a user.
func (s *SQLiteUserStore) UpdatePlan(ctx context.Context, userID int64, planID sql.NullInt64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET plan_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, planID, userID)
	if err != nil {
		return fmt.Errorf("update plan: %w", err)
	}
	return nil
}
