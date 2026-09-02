package migrations

// BackupSchedulesSchema creates the backup_schedules table, adds retention_days
// to backup_storage_providers, and adds backup_provider_id to sites.
var BackupSchedulesSchema = Migration{
	Version: 33,
	SQL: `
-- Scheduled backups table
CREATE TABLE IF NOT EXISTS backup_schedules (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    site_id         INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    provider_id     INTEGER,
    cron_expr       TEXT NOT NULL DEFAULT '',
    backup_type     TEXT NOT NULL DEFAULT 'files',
    enabled         INTEGER NOT NULL DEFAULT 1,
    last_run_at     DATETIME,
    next_run_at     DATETIME,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_backup_schedules_site  ON backup_schedules(site_id);
CREATE INDEX IF NOT EXISTS idx_backup_schedules_due   ON backup_schedules(enabled, next_run_at);

-- Retention days on providers
ALTER TABLE backup_storage_providers ADD COLUMN retention_days INTEGER NOT NULL DEFAULT 0;

-- Per-site backup provider
ALTER TABLE sites ADD COLUMN backup_provider_id INTEGER;

-- App-level settings table for global config like retention
CREATE TABLE IF NOT EXISTS app_settings (
    key         TEXT PRIMARY KEY,
    value       TEXT NOT NULL DEFAULT '',
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT OR IGNORE INTO app_settings (key, value) VALUES ('retention_days', '30');
`,
}
