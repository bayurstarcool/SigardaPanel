package migrations

// SSLSchema adds SSL certificate columns to the sites table.
var SSLSchema = Migration{
	Version: 5,
	SQL: `
ALTER TABLE sites ADD COLUMN ssl_status TEXT;
ALTER TABLE sites ADD COLUMN ssl_issuer TEXT;
ALTER TABLE sites ADD COLUMN ssl_expires_at DATETIME;
ALTER TABLE sites ADD COLUMN ssl_last_check DATETIME;
`,
}
