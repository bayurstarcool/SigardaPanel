package migrations

// BackupCredentialsSchema adds a dedicated encrypted credential for database backups.
var BackupCredentialsSchema = Migration{
	Version: 48,
	SQL: `
ALTER TABLE database_servers ADD COLUMN backup_username TEXT NOT NULL DEFAULT '';
ALTER TABLE database_servers ADD COLUMN backup_password_encrypted TEXT NOT NULL DEFAULT '';
`,
}
