package migrations

// RecoveryCodesSchema adds the two_factor_recovery_codes table.
var RecoveryCodesSchema = Migration{
	Version: 47,
	SQL: `
CREATE TABLE IF NOT EXISTS two_factor_recovery_codes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash TEXT NOT NULL,
    used INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_recovery_codes_user_id ON two_factor_recovery_codes(user_id);
`,
}
