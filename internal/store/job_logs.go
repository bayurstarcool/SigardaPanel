package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// JobLog represents a single line of job output.
type JobLog struct {
	ID         int64
	JobID      int64
	LineNumber int
	Message    string
	Stream     string
	CreatedAt  time.Time
}

// JobLogRepository defines job log persistence operations.
type JobLogRepository interface {
	Append(ctx context.Context, log *JobLog) error
	GetSince(ctx context.Context, jobID int64, sinceLine int, limit int) ([]JobLog, error)
	GetAll(ctx context.Context, jobID int64) ([]JobLog, error)
	Count(ctx context.Context, jobID int64) (int, error)
}

// SQLiteJobLogStore implements JobLogRepository against SQLite.
type SQLiteJobLogStore struct {
	db *sql.DB
}

// NewJobLogStore returns a SQLite-backed job log store.
func NewJobLogStore(db *sql.DB) *SQLiteJobLogStore {
	return &SQLiteJobLogStore{db: db}
}

// Append inserts a new log line.
func (s *SQLiteJobLogStore) Append(ctx context.Context, log *JobLog) error {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO job_logs (job_id, line_number, message, stream) VALUES (?, ?, ?, ?)`,
		log.JobID, log.LineNumber, log.Message, log.Stream,
	)
	if err != nil {
		return fmt.Errorf("insert job log: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	log.ID = id
	return nil
}

// GetSince returns log lines after sinceLine, ordered by line_number ascending.
func (s *SQLiteJobLogStore) GetSince(ctx context.Context, jobID int64, sinceLine int, limit int) ([]JobLog, error) {
	if limit <= 0 || limit > 10000 {
		limit = 1000
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, job_id, line_number, message, stream, created_at FROM job_logs WHERE job_id = ? AND line_number > ? ORDER BY line_number ASC LIMIT ?`,
		jobID, sinceLine, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query logs: %w", err)
	}
	defer rows.Close()

	var logs []JobLog
	for rows.Next() {
		var l JobLog
		if err := rows.Scan(&l.ID, &l.JobID, &l.LineNumber, &l.Message, &l.Stream, &l.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

// GetAll returns all log lines for a job (up to 10000).
func (s *SQLiteJobLogStore) GetAll(ctx context.Context, jobID int64) ([]JobLog, error) {
	return s.GetSince(ctx, jobID, 0, 10000)
}

// Count returns the total number of log lines for a job.
func (s *SQLiteJobLogStore) Count(ctx context.Context, jobID int64) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM job_logs WHERE job_id = ?`, jobID).Scan(&count)
	return count, err
}
