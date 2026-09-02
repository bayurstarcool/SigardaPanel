package migrations

var EncryptedPasswordSchema = Migration{
	Version: 16,
	SQL: `
ALTER TABLE db_users ADD COLUMN encrypted_password TEXT NOT NULL DEFAULT '';
`,
}
