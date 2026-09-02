package migrations

// CloudflarePerUserSchema adds user_id to cloudflare_config for per-user settings.
var CloudflarePerUserSchema = Migration{
	Version: 31,
	SQL: `
-- Add user_id column (nullable first for backward compat)
ALTER TABLE cloudflare_config ADD COLUMN user_id INTEGER DEFAULT NULL;

-- Migrate existing global config: assign to the first admin user
UPDATE cloudflare_config SET user_id = (
	SELECT id FROM users WHERE role = 'super_admin' LIMIT 1
) WHERE user_id IS NULL AND api_token != '';

-- Add unique constraint per user
CREATE UNIQUE INDEX IF NOT EXISTS idx_cloudflare_config_user
	ON cloudflare_config(user_id) WHERE user_id IS NOT NULL;
`,
}
