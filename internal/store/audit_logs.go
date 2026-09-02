package store

import (
	"context"
	"database/sql"
	"fmt"

	"sigardapanel/internal/audit"
)

// NullifyUser sets user_id to NULL in audit logs for the given user.
func (s *SQLiteAuditLogStore) NullifyUser(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE audit_logs SET user_id = NULL WHERE user_id = ?`, userID)
	return err
}

// SQLiteAuditLogStore implements audit.AuditLogStore against a SQLite database.
type SQLiteAuditLogStore struct {
	db *sql.DB
}

// NewAuditLogStore returns a SQLite-backed audit log store.
func NewAuditLogStore(db *sql.DB) *SQLiteAuditLogStore {
	return &SQLiteAuditLogStore{db: db}
}

// List returns audit log entries filtered and paginated.
func (s *SQLiteAuditLogStore) List(ctx context.Context, filter audit.AuditLogFilter) ([]audit.Entry, error) {
	query := `SELECT id, user_id, action, resource, resource_id, details, created_at FROM audit_logs WHERE 1=1`
	args := []any{}

	if filter.UserID > 0 {
		query += ` AND user_id = ?`
		args = append(args, filter.UserID)
	}
	if filter.Resource != "" {
		query += ` AND resource = ?`
		args = append(args, filter.Resource)
	}
	if filter.Action != "" {
		query += ` AND action = ?`
		args = append(args, filter.Action)
	}

	query += ` ORDER BY id DESC LIMIT ? OFFSET ?`
	if filter.Limit <= 0 {
		filter.Limit = 100
	}
	args = append(args, filter.Limit, filter.Offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query audit logs: %w", err)
	}
	defer rows.Close()

	var entries []audit.Entry
	for rows.Next() {
		var e audit.Entry
		if err := rows.Scan(&e.ID, &e.UserID, &e.Action, &e.Resource, &e.ResourceID, &e.Details, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan audit log: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return entries, nil
}
