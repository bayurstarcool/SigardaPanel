package migrations

// SiteDNSRecordsSchema records the DNS records the panel created (or found
// already pointing at the server) for a site, so only those are ever changed
// or removed later. user_id is the account whose Cloudflare token was used.
var SiteDNSRecordsSchema = Migration{
	Version: 59,
	SQL: `
CREATE TABLE IF NOT EXISTS site_dns_records (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    site_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    zone_id TEXT NOT NULL,
    zone_name TEXT NOT NULL,
    record_id TEXT NOT NULL,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    content TEXT NOT NULL,
    managed INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_site_dns_records_site ON site_dns_records(site_id);
`,
}
