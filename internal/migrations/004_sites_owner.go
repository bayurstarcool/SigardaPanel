package migrations

// SitesOwnerSchema adds owner_id to sites table for ownership tracking.
var SitesOwnerSchema = Migration{
	Version: 4,
	SQL: `
ALTER TABLE sites ADD COLUMN owner_id INTEGER REFERENCES users(id);
CREATE INDEX IF NOT EXISTS idx_sites_owner_id ON sites(owner_id);
`,
}
