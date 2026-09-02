package migrations

// JobsSchema adds error, attempts, max_attempts, started_at, completed_at columns
// and a composite index on jobs(status, type).
var JobsSchema = Migration{
	Version: 3,
	SQL: `
ALTER TABLE jobs ADD COLUMN error TEXT;
ALTER TABLE jobs ADD COLUMN attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE jobs ADD COLUMN max_attempts INTEGER NOT NULL DEFAULT 3;
ALTER TABLE jobs ADD COLUMN started_at DATETIME;
ALTER TABLE jobs ADD COLUMN completed_at DATETIME;
CREATE INDEX IF NOT EXISTS idx_jobs_status_type ON jobs(status, type);
`,
}
