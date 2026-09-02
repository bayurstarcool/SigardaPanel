package migrations

// PasswordChangedSchema adds password_changed_at column to users table.
var PasswordChangedSchema = Migration{
	Version: 44,
	SQL: `
ALTER TABLE users ADD COLUMN password_changed_at DATETIME;
UPDATE users SET password_changed_at = created_at;
`,
}
