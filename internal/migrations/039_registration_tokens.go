package migrations

// RegistrationTokensSchema creates the registration_tokens table used for
// one-time agent auto-registration. Each token is created by an admin,
// used once by an install script, and expires after 1 hour.
var RegistrationTokensSchema = Migration{
	Version: 39,
	SQL: `
CREATE TABLE IF NOT EXISTS registration_tokens (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	token_hash TEXT NOT NULL,
	created_by INTEGER,
	expires_at DATETIME NOT NULL,
	used_at DATETIME,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_registration_tokens_hash ON registration_tokens(token_hash);
`,
}
