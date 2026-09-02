package migrations

// UserSitesSchema maps panel users to sites they can access and manage.
var UserSitesSchema = Migration{
	Version: 20,
	SQL: `
CREATE TABLE IF NOT EXISTS user_sites (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	site_id    INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
	permission TEXT    NOT NULL DEFAULT 'read' CHECK (permission IN ('read','write','admin')),
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(user_id, site_id)
);

-- Ensure existing panel users have a home_path
UPDATE users SET email = username WHERE email = '';
` + "UPDATE users SET email = COALESCE(NULLIF(email, ''), username) WHERE email IS NULL OR email = '';",
}
