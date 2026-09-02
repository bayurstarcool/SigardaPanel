package migrations

// UserMaxSitesSchema adds max_sites column to users table.
// NULL = unlimited, INTEGER = max number of sites.
var UserMaxSitesSchema = Migration{
	Version: 24,
	SQL:     `ALTER TABLE users ADD COLUMN max_sites INTEGER;`,
}
