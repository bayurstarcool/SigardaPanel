package migrations

// IPRestrictionSchema adds ip_restrictions table.
var IPRestrictionSchema = Migration{
	Version: 43,
	SQL: `
CREATE TABLE IF NOT EXISTS ip_restrictions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    site_id INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    ip TEXT NOT NULL,
    action TEXT NOT NULL DEFAULT 'deny',
    reason TEXT DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_ip_restrictions_site_id ON ip_restrictions(site_id);
CREATE INDEX IF NOT EXISTS idx_ip_restrictions_ip ON ip_restrictions(ip);
`,
}
