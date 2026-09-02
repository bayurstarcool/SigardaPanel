package migrations

// LicensingSchema creates the licenses and feature_flags tables for the
// Open Core licensing system. Licenses control tier-based access (free,
// pro, enterprise) and feature_flags define which capabilities each tier
// unlocks.
var LicensingSchema = Migration{
	Version: 40,
	SQL: `
CREATE TABLE IF NOT EXISTS licenses (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	key TEXT NOT NULL UNIQUE,
	tier TEXT NOT NULL DEFAULT 'free',
	activated_at DATETIME,
	expires_at DATETIME,
	max_servers INTEGER DEFAULT 1,
	features TEXT DEFAULT '{}',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS feature_flags (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	key TEXT NOT NULL UNIQUE,
	name TEXT NOT NULL,
	tier TEXT NOT NULL DEFAULT 'pro',
	enabled INTEGER DEFAULT 1,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
`,
}
