package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"sigardapanel/internal/jobmodel"
	"sigardapanel/internal/secrets"
)

// JobRepository defines job persistence operations.
type JobRepository interface {
	Create(ctx context.Context, j *jobmodel.Job) error
	GetByID(ctx context.Context, id int64) (*jobmodel.Job, error)
	List(ctx context.Context, filter JobListFilter) ([]jobmodel.Job, error)
	UpdateStatus(ctx context.Context, id int64, status string) error
	ClaimNext(ctx context.Context, jobType string) (*jobmodel.Job, error)
	ScheduleRetry(ctx context.Context, id int64, retryAt time.Time) error
	SetError(ctx context.Context, id int64, errMsg string) error
	SetResult(ctx context.Context, id int64, result string) error
	MarkStarted(ctx context.Context, id int64) error
	MarkCompleted(ctx context.Context, id int64, success bool, result, errMsg string) error
	Cancel(ctx context.Context, id int64) error
	HasActiveJob(ctx context.Context, jobType string, siteID int64) (bool, error)
	HasRecentFailedJob(ctx context.Context, jobType string, siteID int64, within time.Duration) (bool, error)
	CountRunningByServer(ctx context.Context, serverID int64) (int, error)
	CleanupStuckRunning(ctx context.Context, staleAfter time.Duration) (int64, error)
	FailOrphanedRunning(ctx context.Context, reason string) (int64, error)
	ScrubTerminalPayloads(ctx context.Context, olderThan time.Duration) (int64, error)
}

// JobListFilter provides filtering and pagination for List.
type JobListFilter struct {
	ServerID int64
	SiteID   int64
	Status   string
	Type     string
	Limit    int
	Offset   int
}

// SQLiteJobStore implements JobRepository against SQLite.
type SQLiteJobStore struct {
	db *sql.DB
}

// NewJobStore returns a SQLite-backed job store.
func NewJobStore(db *sql.DB) *SQLiteJobStore {
	return &SQLiteJobStore{db: db}
}

var jobColumns = `id, server_id, site_id, type, status, COALESCE(payload, ''), COALESCE(result, ''), COALESCE(error, ''), COALESCE(description, ''), attempts, max_attempts, created_at, updated_at, started_at, completed_at, next_retry_at`

// Create inserts a new job with default status queued and max_attempts 3.
func (s *SQLiteJobStore) Create(ctx context.Context, j *jobmodel.Job) error {
	if j.Status == "" {
		j.Status = jobmodel.StatusQueued
	}
	if j.MaxAttempts == 0 {
		j.MaxAttempts = 3
	}
	// Encrypt credential fields in the payload so they are never stored in
	// plaintext at rest. The worker decrypts before dispatching to the agent.
	storedPayload := j.Payload
	if enc, err := secrets.EncryptPayload(j.Payload); err == nil {
		storedPayload = enc
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO jobs (server_id, site_id, type, status, payload, max_attempts, description) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		j.ServerID, j.SiteID, j.Type, j.Status, storedPayload, j.MaxAttempts, j.Description,
	)
	if err != nil {
		return fmt.Errorf("insert job: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("last insert id: %w", err)
	}
	j.ID = id
	return nil
}

// GetByID retrieves a job by ID. Returns (nil, nil) if not found.
func (s *SQLiteJobStore) GetByID(ctx context.Context, id int64) (*jobmodel.Job, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+jobColumns+` FROM jobs WHERE id = ?`, id)
	return scanJob(row)
}

// List returns jobs ordered by id DESC with optional filters.
func (s *SQLiteJobStore) List(ctx context.Context, filter JobListFilter) ([]jobmodel.Job, error) {
	if filter.Limit <= 0 {
		filter.Limit = 100
	}

	query := `SELECT ` + jobColumns + ` FROM jobs WHERE 1=1`
	var args []any

	if filter.ServerID > 0 {
		query += ` AND server_id = ?`
		args = append(args, filter.ServerID)
	}
	if filter.SiteID > 0 {
		query += ` AND site_id = ?`
		args = append(args, filter.SiteID)
	}
	if filter.Status != "" {
		query += ` AND status = ?`
		args = append(args, filter.Status)
	}
	if filter.Type != "" {
		query += ` AND type = ?`
		args = append(args, filter.Type)
	}
	query += ` ORDER BY id DESC LIMIT ? OFFSET ?`
	args = append(args, filter.Limit, filter.Offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query jobs: %w", err)
	}
	defer rows.Close()

	var out []jobmodel.Job
	for rows.Next() {
		var j jobmodel.Job
		if err := rows.Scan(
			&j.ID, &j.ServerID, &j.SiteID, &j.Type, &j.Status, &j.Payload, &j.Result, &j.Error,
			&j.Description, &j.Attempts, &j.MaxAttempts, &j.CreatedAt, &j.UpdatedAt, &j.StartedAt, &j.CompletedAt, &j.NextRetryAt,
		); err != nil {
			return nil, fmt.Errorf("scan job: %w", err)
		}
		out = append(out, j)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return out, nil
}

// claimableJobsCond selects queued jobs (alias j) that may start now:
//   - due (no pending retry back-off),
//   - their site has no job running (jobs for one site never run concurrently),
//   - for backup jobs, their server has no backup running (max 1 per server).
const claimableJobsCond = `j.status = 'queued'
	AND (j.next_retry_at IS NULL OR j.next_retry_at <= CURRENT_TIMESTAMP)
	AND (j.site_id IS NULL OR NOT EXISTS (
		SELECT 1 FROM jobs r WHERE r.status = 'running' AND r.site_id = j.site_id))
	AND (j.type NOT IN ('backup.create', 'db_backup.create') OR j.server_id IS NULL OR NOT EXISTS (
		SELECT 1 FROM jobs r WHERE r.status = 'running' AND r.server_id = j.server_id
		AND r.type IN ('backup.create', 'db_backup.create')))`

// ClaimNext atomically picks the oldest claimable queued job, marks it
// running, increments attempts, and sets started_at.
//
// The pick and the claim are one UPDATE statement, so several workers (or
// processes) calling ClaimNext concurrently can never claim the same job, nor
// two jobs of the same site: SQLite serialises writers and the eligibility
// sub-queries are evaluated inside that write.
func (s *SQLiteJobStore) ClaimNext(ctx context.Context, jobType string) (*jobmodel.Job, error) {
	query := `UPDATE jobs SET status = 'running', started_at = CURRENT_TIMESTAMP,
		attempts = attempts + 1, updated_at = CURRENT_TIMESTAMP
		WHERE status = 'queued' AND id = (
			SELECT j.id FROM jobs j WHERE ` + claimableJobsCond
	var args []any
	if jobType != "" {
		query += ` AND j.type = ?`
		args = append(args, jobType)
	}
	query += ` ORDER BY j.id ASC LIMIT 1)
		RETURNING id`

	var id int64
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claim next job: %w", err)
	}

	job, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("load claimed job: %w", err)
	}
	if job == nil {
		return nil, fmt.Errorf("claimed job %d vanished", id)
	}
	return job, nil
}

// FailOrphanedRunning marks every job still in 'running' as failed. It is
// meant for worker start-up: the worker runs in-process, so a 'running' row
// left over from a previous process can never finish, and with per-site
// serialisation it would otherwise block that site's queue until the 30-minute
// stuck-job cleanup.
func (s *SQLiteJobStore) FailOrphanedRunning(ctx context.Context, reason string) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE jobs SET status = 'failed', error = ?, completed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		 WHERE status = 'running'`, reason)
	if err != nil {
		return 0, fmt.Errorf("fail orphaned jobs: %w", err)
	}
	return res.RowsAffected()
}

// UpdateStatus updates a job's status.
func (s *SQLiteJobStore) UpdateStatus(ctx context.Context, id int64, status string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE jobs SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		status, id,
	)
	if err != nil {
		return fmt.Errorf("update job status: %w", err)
	}
	return nil
}

// Cancel sets a job's status to canceled only if it is currently queued.
func (s *SQLiteJobStore) Cancel(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE jobs SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND status = ?`,
		jobmodel.StatusCanceled, id, jobmodel.StatusQueued,
	)
	if err != nil {
		return fmt.Errorf("cancel job: %w", err)
	}
	return nil
}

// SetError updates the error column for a job.
func (s *SQLiteJobStore) SetError(ctx context.Context, id int64, errMsg string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE jobs SET error = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		sql.NullString{String: errMsg, Valid: errMsg != ""}, id,
	)
	if err != nil {
		return fmt.Errorf("set job error: %w", err)
	}
	return nil
}

// SetResult updates the result column for a job.
func (s *SQLiteJobStore) SetResult(ctx context.Context, id int64, result string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE jobs SET result = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		result, id,
	)
	if err != nil {
		return fmt.Errorf("set job result: %w", err)
	}
	return nil
}

// MarkStarted records the start time of a job.
func (s *SQLiteJobStore) MarkStarted(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE jobs SET started_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		id,
	)
	if err != nil {
		return fmt.Errorf("mark job started: %w", err)
	}
	return nil
}

// ScheduleRetry sets status=queued and next_retry_at for a failed job.
func (s *SQLiteJobStore) ScheduleRetry(ctx context.Context, id int64, retryAt time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE jobs SET status = ?, next_retry_at = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		jobmodel.StatusQueued, retryAt.UTC().Format("2006-01-02 15:04:05"), id,
	)
	if err != nil {
		return fmt.Errorf("schedule retry: %w", err)
	}
	return nil
}

// MarkCompleted updates status, completed_at, result, and error in one call.
func (s *SQLiteJobStore) MarkCompleted(ctx context.Context, id int64, success bool, result, errMsg string) error {
	var status string
	if success {
		status = jobmodel.StatusSucceeded
	} else {
		status = jobmodel.StatusFailed
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE jobs SET status = ?, completed_at = CURRENT_TIMESTAMP, result = ?, error = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		status, result, sql.NullString{String: errMsg, Valid: errMsg != ""}, id,
	)
	if err != nil {
		return fmt.Errorf("mark job completed: %w", err)
	}
	return nil
}

func scanJob(row *sql.Row) (*jobmodel.Job, error) {
	var j jobmodel.Job
	err := row.Scan(
		&j.ID, &j.ServerID, &j.SiteID, &j.Type, &j.Status, &j.Payload, &j.Result, &j.Error,
		&j.Description, &j.Attempts, &j.MaxAttempts, &j.CreatedAt, &j.UpdatedAt, &j.StartedAt, &j.CompletedAt, &j.NextRetryAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan job: %w", err)
	}
	return &j, nil
}

// HasRecentFailedJob returns true if a job of the given type for the site
// failed within the given window.
func (s *SQLiteJobStore) HasRecentFailedJob(ctx context.Context, jobType string, siteID int64, within time.Duration) (bool, error) {
	cutoff := time.Now().Add(-within).UTC().Format("2006-01-02 15:04:05")
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM jobs WHERE type = ? AND site_id = ? AND status = 'failed' AND updated_at >= ?`,
		jobType, siteID, cutoff).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("has recent failed job: %w", err)
	}
	return count > 0, nil
}

// HasActiveJob returns true if there is a queued or running job of the given type for the site.
func (s *SQLiteJobStore) HasActiveJob(ctx context.Context, jobType string, siteID int64) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM jobs WHERE type = ? AND site_id = ? AND status IN ('queued', 'running')`,
		jobType, siteID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check active job: %w", err)
	}
	return count > 0, nil
}

// CountRunningByServer returns the number of running backup jobs for a server.
func (s *SQLiteJobStore) CountRunningByServer(ctx context.Context, serverID int64) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM jobs WHERE server_id = ? AND status = 'running'
		 AND type IN ('backup.create', 'db_backup.create')`, serverID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count running backups: %w", err)
	}
	return count, nil
}

// CleanupStuckRunning marks jobs stuck in 'running' for longer than staleAfter as failed.
func (s *SQLiteJobStore) CleanupStuckRunning(ctx context.Context, staleAfter time.Duration) (int64, error) {
	cutoff := time.Now().Add(-staleAfter)
	res, err := s.db.ExecContext(ctx,
		`UPDATE jobs SET status = 'failed', error = 'cleanup: stuck in running state', updated_at = CURRENT_TIMESTAMP
		 WHERE status = 'running' AND updated_at < ?`, cutoff.UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		return 0, fmt.Errorf("cleanup stuck jobs: %w", err)
	}
	return res.RowsAffected()
}

// ScrubTerminalPayloads clears the payload of jobs that reached a terminal
// state longer than olderThan ago. Job payloads may contain S3 credentials and
// database passwords needed only while the job runs; scrubbing them limits how
// long secrets linger in the database. Job history (type, status, timing) is
// preserved.
func (s *SQLiteJobStore) ScrubTerminalPayloads(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan).UTC().Format("2006-01-02 15:04:05")
	res, err := s.db.ExecContext(ctx,
		`UPDATE jobs SET payload = '' WHERE payload IS NOT NULL AND payload != ''
		   AND status IN (?, ?, ?)
		   AND COALESCE(completed_at, updated_at) < ?`,
		jobmodel.StatusSucceeded, jobmodel.StatusFailed, jobmodel.StatusCanceled, cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("scrub terminal payloads: %w", err)
	}
	return res.RowsAffected()
}
