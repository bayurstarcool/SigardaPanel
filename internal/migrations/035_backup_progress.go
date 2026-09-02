package migrations

// BackupProgressSchema adds S3/progress/type columns to backups for real-time tracking.
var BackupProgressSchema = Migration{
	Version: 35,
	SQL: `
ALTER TABLE backups ADD COLUMN s3_path TEXT;
ALTER TABLE backups ADD COLUMN progress TEXT DEFAULT '';
ALTER TABLE backups ADD COLUMN backup_type TEXT DEFAULT 'files';
`,
}
