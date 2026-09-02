package migrations

// CloudflareConfigPerUserSchema removes the legacy id = 1 constraint.
// Migration 031 introduced per-user Cloudflare configuration, but the table
// created by migration 019 still allowed only one row. Rebuild the SQLite
// table so each user can own one configuration while preserving existing data.
var CloudflareConfigPerUserSchema = Migration{
	Version: 49,
	SQL: `
CREATE TABLE cloudflare_config_new (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	api_token TEXT NOT NULL DEFAULT '',
	email TEXT DEFAULT '',
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	user_id INTEGER DEFAULT NULL
);

INSERT INTO cloudflare_config_new (id, api_token, email, created_at, updated_at, user_id)
SELECT id, api_token, email, created_at, updated_at, user_id
FROM cloudflare_config;

DROP TABLE cloudflare_config;
ALTER TABLE cloudflare_config_new RENAME TO cloudflare_config;

CREATE UNIQUE INDEX IF NOT EXISTS idx_cloudflare_config_user
	ON cloudflare_config(user_id) WHERE user_id IS NOT NULL;
`,
}
