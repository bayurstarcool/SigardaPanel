package migrations

var DatabasesSchema = Migration{
	Version: 12,
	SQL: `
CREATE TABLE IF NOT EXISTS databases (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    site_id INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    charset TEXT NOT NULL DEFAULT 'utf8mb4',
    collation TEXT NOT NULL DEFAULT 'utf8mb4_unicode_ci',
    size_bytes INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_databases_site_id ON databases(site_id);
CREATE INDEX IF NOT EXISTS idx_databases_name ON databases(name);

CREATE TABLE IF NOT EXISTS db_users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    database_id INTEGER NOT NULL REFERENCES databases(id) ON DELETE CASCADE,
    username TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    host_pattern TEXT NOT NULL DEFAULT 'localhost',
    privileges TEXT NOT NULL DEFAULT 'ALL',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_db_users_database_id ON db_users(database_id);
CREATE INDEX IF NOT EXISTS idx_db_users_username ON db_users(username);
`,
}
