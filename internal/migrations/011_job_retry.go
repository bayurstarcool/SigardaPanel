package migrations

var JobRetrySchema = Migration{
	Version: 11,
	SQL: `
ALTER TABLE jobs ADD COLUMN next_retry_at DATETIME;
CREATE INDEX IF NOT EXISTS idx_jobs_next_retry_at ON jobs(next_retry_at) WHERE status = 'queued';
`,
}
