package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Session represents an active authentication session.
type Session struct {
	ID             int64
	UserID         int64
	TokenHash      string
	ExpiresAt      time.Time
	CreatedAt      time.Time
	RevokedAt      sql.NullTime
	ImpersonatedBy sql.NullInt64
}

// SessionRepository defines session persistence operations.
type SessionRepository interface {
	Create(ctx context.Context, session *Session) error
	GetByTokenHash(ctx context.Context, hash string) (*Session, error)
	Revoke(ctx context.Context, id int64) error
	RevokeByTokenHash(ctx context.Context, hash string) error
	CleanupExpired(ctx context.Context) error
	RevokeOldest(ctx context.Context, userID int64, maxSessions int) error
	CountActive(ctx context.Context, userID int64) (int, error)
}

// SQLiteSessionStore implements SessionRepository against a SQLite database.
type SQLiteSessionStore struct {
	db *sql.DB
}

// NewSessionStore returns a SQLite-backed session store.
func NewSessionStore(db *sql.DB) *SQLiteSessionStore {
	return &SQLiteSessionStore{db: db}
}

// Create inserts a new session into the database.
func (s *SQLiteSessionStore) Create(ctx context.Context, session *Session) error {
	// Store expires_at as UTC ISO8601 so SQLite datetime() can compare it
	expiresUTC := session.ExpiresAt.UTC().Format("2006-01-02 15:04:05")
	var res sql.Result
	var err error
	if session.ImpersonatedBy.Valid {
		res, err = s.db.ExecContext(ctx,
			`INSERT INTO sessions (user_id, token_hash, expires_at, impersonated_by) VALUES (?, ?, ?, ?)`,
			session.UserID, session.TokenHash, expiresUTC, session.ImpersonatedBy.Int64,
		)
	} else {
		res, err = s.db.ExecContext(ctx,
			`INSERT INTO sessions (user_id, token_hash, expires_at) VALUES (?, ?, ?)`,
			session.UserID, session.TokenHash, expiresUTC,
		)
	}
	if err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("last insert id: %w", err)
	}
	session.ID = id
	return nil
}

// GetByTokenHash retrieves a session by its token hash.
// Returns (nil, nil) if not found.
func (s *SQLiteSessionStore) GetByTokenHash(ctx context.Context, hash string) (*Session, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, token_hash, expires_at, created_at, revoked_at, impersonated_by
		 FROM sessions WHERE token_hash = ? AND revoked_at IS NULL`, hash)
	return scanSession(row)
}

// Revoke marks a session as revoked.
func (s *SQLiteSessionStore) Revoke(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET revoked_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

// RevokeByTokenHash marks a session as revoked using its token hash.
func (s *SQLiteSessionStore) RevokeByTokenHash(ctx context.Context, hash string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET revoked_at = CURRENT_TIMESTAMP WHERE token_hash = ?`, hash)
	if err != nil {
		return fmt.Errorf("revoke session by token hash: %w", err)
	}
	return nil
}

// CleanupExpired removes all expired and revoked sessions.
func (s *SQLiteSessionStore) CleanupExpired(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM sessions WHERE
		 revoked_at IS NOT NULL
		 OR expires_at < datetime('now')`)
	if err != nil {
		return fmt.Errorf("cleanup expired sessions: %w", err)
	}
	return nil
}

// RevokeOldest keeps only the latest `maxSessions` active sessions per user.
// Older sessions are revoked to prevent session accumulation.
func (s *SQLiteSessionStore) RevokeOldest(ctx context.Context, userID int64, maxSessions int) error {
	// Revoke all but the latest N active sessions for this user
	_, err := s.db.ExecContext(ctx, `
		UPDATE sessions SET revoked_at = CURRENT_TIMESTAMP
		WHERE id IN (
			SELECT id FROM sessions
			WHERE user_id = ? AND revoked_at IS NULL AND expires_at > datetime('now')
			ORDER BY id DESC
			LIMIT -1 OFFSET ?
		)`, userID, maxSessions)
	if err != nil {
		return fmt.Errorf("revoke oldest sessions: %w", err)
	}
	return nil
}

// CountActive returns the number of active (non-revoked, non-expired) sessions for a user.
func (s *SQLiteSessionStore) CountActive(ctx context.Context, userID int64) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sessions WHERE user_id = ? AND revoked_at IS NULL AND expires_at > datetime('now')`,
		userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count active sessions: %w", err)
	}
	return count, nil
}

func scanSession(row *sql.Row) (*Session, error) {
	var sess Session
	var expiresAtStr, createdAtStr string
	var revokedAtStr sql.NullString
	err := row.Scan(&sess.ID, &sess.UserID, &sess.TokenHash, &expiresAtStr, &createdAtStr, &revokedAtStr, &sess.ImpersonatedBy)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan session: %w", err)
	}
	
	// Parse expires_at — handle both new UTC format and legacy Go default format
	sess.ExpiresAt, err = parseTimestamp(expiresAtStr)
	if err != nil {
		return nil, fmt.Errorf("parse expires_at: %w", err)
	}
	
	// Parse created_at — same dual-format handling
	sess.CreatedAt, err = parseTimestamp(createdAtStr)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	
	// Parse revoked_at (nullable)
	if revokedAtStr.Valid {
		revokedTime, err := parseTimestamp(revokedAtStr.String)
		if err != nil {
			return nil, fmt.Errorf("parse revoked_at: %w", err)
		}
		sess.RevokedAt = sql.NullTime{Time: revokedTime, Valid: true}
	}
	
	return &sess, nil
}

// parseTimestamp handles both new UTC format ("2006-01-02 15:04:05") and
// legacy Go time.Time default format ("2006-01-02 15:04:05.999999999 -0700 MST m=...").
func parseTimestamp(s string) (time.Time, error) {
	// Try new clean UTC format first
	if t, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
		return t.UTC(), nil
	}
	// Try Go's default layout (may contain fractional seconds, timezone, monotonic)
	// Strip monotonic part: everything after " m="
	cleaned := s
	if idx := strings.Index(cleaned, " m="); idx > 0 {
		cleaned = cleaned[:idx]
	}
	if t, err := time.Parse("2006-01-02 15:04:05.999999999 -0700 MST", cleaned); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse("2006-01-02 15:04:05 -0700 MST", cleaned); err == nil {
		return t.UTC(), nil
	}
	// Last resort: let Go try its best
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("cannot parse timestamp %q", s)
}