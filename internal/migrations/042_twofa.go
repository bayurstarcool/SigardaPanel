package migrations

// TwoFactorSchema adds TOTP 2FA columns to the users table.
var TwoFactorSchema = Migration{
	Version: 42,
	SQL: `
ALTER TABLE users ADD COLUMN totp_secret TEXT DEFAULT '';
ALTER TABLE users ADD COLUMN totp_enabled INTEGER NOT NULL DEFAULT 0;
`,
}
