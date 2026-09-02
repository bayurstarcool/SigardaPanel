package migrations

// UserDiskLimitSchema adds disk_limit column to users table.
// NULL = unlimited, INTEGER = max disk usage in GB.
var UserDiskLimitSchema = Migration{
	Version: 23,
	SQL:     `ALTER TABLE users ADD COLUMN disk_limit INTEGER;`,
}
