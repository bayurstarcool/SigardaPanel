package migrations

// BackupProvidersSchema creates the backup_storage_providers table for S3-compatible storage configs.
var BackupProvidersSchema = Migration{
	Version: 25,
	SQL: `
CREATE TABLE IF NOT EXISTS backup_storage_providers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    provider_type TEXT NOT NULL DEFAULT 's3' CHECK(provider_type IN ('s3','b2','r2','wasabi','digitalocean','generic')),
    bucket TEXT NOT NULL,
    endpoint TEXT NOT NULL,
    region TEXT NOT NULL DEFAULT 'auto',
    access_key TEXT NOT NULL,
    secret_key TEXT NOT NULL,
    is_default INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_backup_providers_default ON backup_storage_providers(is_default);
`,
}
