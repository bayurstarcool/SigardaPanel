package migrations

// SiteOriginSchema stores where a site came from and how SigardaPanel should treat it.
var SiteOriginSchema = Migration{
	Version: 18,
	SQL: `
ALTER TABLE sites ADD COLUMN managed_by TEXT NOT NULL DEFAULT 'sigardapanel';
ALTER TABLE sites ADD COLUMN system_user TEXT;
ALTER TABLE sites ADD COLUMN conf_file TEXT;
ALTER TABLE sites ADD COLUMN proxy_pass_url TEXT;
CREATE INDEX IF NOT EXISTS idx_sites_managed_by ON sites(managed_by);
CREATE INDEX IF NOT EXISTS idx_sites_system_user ON sites(system_user);
`,
}
