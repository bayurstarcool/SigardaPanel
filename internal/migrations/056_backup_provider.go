package migrations

// BackupProviderSchema records which storage provider holds each backup's
// S3 copy, so deleting, restoring or downloading it uses that provider even
// after the site's (or the default) provider changed.
var BackupProviderSchema = Migration{
	Version: 56,
	SQL:     `ALTER TABLE backups ADD COLUMN provider_id INTEGER;`,
}
