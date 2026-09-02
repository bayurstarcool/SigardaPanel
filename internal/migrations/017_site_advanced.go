package migrations

// SiteAdvancedSchema adds tables for site advanced features: vhost, varnish, security, ssh/ftp, cron.
var SiteAdvancedSchema = Migration{
	Version: 17,
	SQL: `
CREATE TABLE IF NOT EXISTS site_vhost (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    site_id INTEGER NOT NULL UNIQUE REFERENCES sites(id) ON DELETE CASCADE,
    config TEXT NOT NULL DEFAULT '',
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS site_varnish (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    site_id INTEGER NOT NULL UNIQUE REFERENCES sites(id) ON DELETE CASCADE,
    enabled INTEGER NOT NULL DEFAULT 0,
    cache_time INTEGER NOT NULL DEFAULT 3600,
    hit_rate TEXT NOT NULL DEFAULT '0%',
    backend_host TEXT NOT NULL DEFAULT '127.0.0.1',
    backend_port INTEGER NOT NULL DEFAULT 80,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS site_security (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    site_id INTEGER NOT NULL UNIQUE REFERENCES sites(id) ON DELETE CASCADE,
    basic_auth_enabled INTEGER NOT NULL DEFAULT 0,
    basic_auth_username TEXT NOT NULL DEFAULT '',
    basic_auth_password TEXT NOT NULL DEFAULT '',
    blocked_ips TEXT NOT NULL DEFAULT '[]',
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS site_ssh_ftp (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    site_id INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    username TEXT NOT NULL,
    password TEXT NOT NULL DEFAULT '',
    ssh_keys TEXT NOT NULL DEFAULT '',
    home_dir TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(site_id, username)
);

CREATE TABLE IF NOT EXISTS site_cron_jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    site_id INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    schedule TEXT NOT NULL DEFAULT '* * * * *',
    command TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_site_vhost_site_id ON site_vhost(site_id);
CREATE INDEX IF NOT EXISTS idx_site_varnish_site_id ON site_varnish(site_id);
CREATE INDEX IF NOT EXISTS idx_site_security_site_id ON site_security(site_id);
CREATE INDEX IF NOT EXISTS idx_site_ssh_ftp_site_id ON site_ssh_ftp(site_id);
CREATE INDEX IF NOT EXISTS idx_site_ssh_ftp_username ON site_ssh_ftp(username);
CREATE INDEX IF NOT EXISTS idx_site_cron_jobs_site_id ON site_cron_jobs(site_id);
`,
}
