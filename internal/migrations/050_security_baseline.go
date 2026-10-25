package migrations

// SecurityBaselineSchema stores the known-good integrity hashes of critical
// files per server so the threat monitor can report drift (tampered
// sshd_config, new authorized_keys entry, edited sudoers, ...).
var SecurityBaselineSchema = Migration{
	Version: 50,
	SQL: `
CREATE TABLE IF NOT EXISTS security_baselines (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	server_id INTEGER NOT NULL,
	path TEXT NOT NULL,
	sha256 TEXT NOT NULL,
	mode TEXT NOT NULL DEFAULT '',
	uid INTEGER NOT NULL DEFAULT 0,
	size INTEGER NOT NULL DEFAULT 0,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(server_id, path)
);
`,
}
