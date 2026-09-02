package migrations

// AppMemoryLimitSchema adds custom per-site memory limit in MB.
var AppMemoryLimitSchema = Migration{
	Version: 29,
	SQL:     `ALTER TABLE sites ADD COLUMN app_memory_max INTEGER DEFAULT 512;`,
}
