package migrations

var DatabaseServersSchema = Migration{
	Version: 15,
	SQL: `
CREATE TABLE IF NOT EXISTS database_servers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL DEFAULT 'Local MySQL',
    engine TEXT NOT NULL DEFAULT 'MariaDB',
    host TEXT NOT NULL DEFAULT '127.0.0.1',
    port INTEGER NOT NULL DEFAULT 3306,
    username TEXT NOT NULL DEFAULT 'root',
    password_encrypted TEXT NOT NULL,
    is_active INTEGER NOT NULL DEFAULT 1,
    is_default INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`,
}
