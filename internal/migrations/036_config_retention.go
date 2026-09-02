package migrations

// BackupConfigRetentionSchema adds retention_days to backup_configs for automatic cleanup.
var BackupConfigRetentionSchema = Migration{
	Version: 36,
	SQL:     `ALTER TABLE backup_configs ADD COLUMN retention_days INTEGER NOT NULL DEFAULT 30`,
}
