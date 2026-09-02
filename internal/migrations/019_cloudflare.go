package migrations

// CloudflareSchema stores Cloudflare API configuration.
var CloudflareSchema = Migration{
	Version: 19,
	SQL: `
CREATE TABLE IF NOT EXISTS cloudflare_config (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	api_token TEXT NOT NULL DEFAULT '',
	email TEXT DEFAULT '',
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
`,
}
