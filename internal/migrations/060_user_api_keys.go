package migrations

// UserAPIKeysSchema adds personal API keys: long-lived bearer tokens that act
// as the user who created them. Only the SHA-256 hash of the key is stored;
// the raw key is shown once at creation. prefix keeps the first characters
// so the UI can tell keys apart without revealing them. Version 60 leaves
// room for unrelated migrations being added in parallel.
var UserAPIKeysSchema = Migration{
	Version: 60,
	SQL: `
CREATE TABLE IF NOT EXISTS user_api_keys (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	name         TEXT NOT NULL,
	key_hash     TEXT NOT NULL UNIQUE,
	prefix       TEXT NOT NULL,
	created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	expires_at   DATETIME,
	last_used_at DATETIME
);
CREATE INDEX IF NOT EXISTS idx_user_api_keys_user_id ON user_api_keys(user_id);
`,
}
