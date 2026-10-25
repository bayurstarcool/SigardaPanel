package store

import (
	"context"
	"database/sql"
	"time"
)

// SecurityBaseline is one known-good snapshot row of a critical file.
type SecurityBaseline struct {
	ID        int64
	ServerID  int64
	Path      string
	SHA256    string
	Mode      string
	UID       int
	Size      int64
	UpdatedAt time.Time
}

// SecurityBaselineRepository persists integrity baselines per server.
type SecurityBaselineRepository interface {
	ReplaceForServer(ctx context.Context, serverID int64, entries []SecurityBaseline) error
	ListByServer(ctx context.Context, serverID int64) ([]SecurityBaseline, error)
}

type SQLiteSecurityBaselineStore struct{ db *sql.DB }

func NewSecurityBaselineStore(db *sql.DB) *SQLiteSecurityBaselineStore {
	return &SQLiteSecurityBaselineStore{db: db}
}

// ReplaceForServer swaps the whole baseline for a server in one transaction
// so a snapshot is never half-written.
func (s *SQLiteSecurityBaselineStore) ReplaceForServer(ctx context.Context, serverID int64, entries []SecurityBaseline) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM security_baselines WHERE server_id = ?`, serverID); err != nil {
		return err
	}
	for _, e := range entries {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO security_baselines (server_id, path, sha256, mode, uid, size, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
			serverID, e.Path, e.SHA256, e.Mode, e.UID, e.Size); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteSecurityBaselineStore) ListByServer(ctx context.Context, serverID int64) ([]SecurityBaseline, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, server_id, path, sha256, mode, uid, size, updated_at FROM security_baselines WHERE server_id = ? ORDER BY path`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SecurityBaseline
	for rows.Next() {
		var b SecurityBaseline
		if err := rows.Scan(&b.ID, &b.ServerID, &b.Path, &b.SHA256, &b.Mode, &b.UID, &b.Size, &b.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
