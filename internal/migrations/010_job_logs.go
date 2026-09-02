package migrations

var JobLogsSchema = Migration{
	Version: 10,
	SQL: `
CREATE TABLE IF NOT EXISTS job_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id INTEGER NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    line_number INTEGER NOT NULL,
    message TEXT NOT NULL,
    stream TEXT NOT NULL DEFAULT 'stdout',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_job_logs_job_id ON job_logs(job_id);
CREATE INDEX IF NOT EXISTS idx_job_logs_job_line ON job_logs(job_id, line_number);
`,
}
