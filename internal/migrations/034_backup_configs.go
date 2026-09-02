package migrations

// BackupConfigsSchema creates the backup_configs table — a flexible,
// multi-configuration backup system that supersedes backup_schedules.
var BackupConfigsSchema = Migration{
	Version: 34,
	SQL: `
CREATE TABLE IF NOT EXISTS backup_configs (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    name              TEXT NOT NULL DEFAULT '',
    scope_type        TEXT NOT NULL DEFAULT 'files',
    site_ids          TEXT NOT NULL DEFAULT '[]',
    provider_id       INTEGER,
    cron_expr         TEXT NOT NULL DEFAULT '',
    enabled           INTEGER NOT NULL DEFAULT 1,
    exclude_patterns  TEXT NOT NULL DEFAULT '[]',
    database_ids      TEXT NOT NULL DEFAULT '[]',
    last_run_at       DATETIME,
    next_run_at       DATETIME,
    created_at        DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at        DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_backup_configs_due ON backup_configs(enabled, next_run_at);
`,
}
