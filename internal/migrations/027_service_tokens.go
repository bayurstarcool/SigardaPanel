package migrations

// ServiceTokensSchema creates the service_tokens table for API key / service account auth.
var ServiceTokensSchema = Migration{
	Version: 27,
	SQL: `
CREATE TABLE IF NOT EXISTS service_tokens (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    key_hash TEXT NOT NULL UNIQUE,
    scopes TEXT NOT NULL DEFAULT '*',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME,
    last_used_at DATETIME
);
CREATE INDEX IF NOT EXISTS idx_service_tokens_key_hash ON service_tokens(key_hash);
`,
}
