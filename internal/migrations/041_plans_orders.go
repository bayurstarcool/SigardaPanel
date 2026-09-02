package migrations

// PlansLicenseOrdersSchema creates the plans and license_orders tables for the
// billing system, and adds plan_id to users for plan-feature assignment.
var PlansLicenseOrdersSchema = Migration{
	Version: 41,
	SQL: `
CREATE TABLE IF NOT EXISTS plans (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE,
	slug TEXT NOT NULL UNIQUE,
	price_idr INTEGER DEFAULT 0,
	billing_cycle TEXT DEFAULT 'monthly',
	features TEXT DEFAULT '{}',
	is_active BOOLEAN DEFAULT 1,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS license_orders (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER REFERENCES users(id),
	plan_id INTEGER REFERENCES plans(id),
	status TEXT DEFAULT 'pending',
	payment_method TEXT,
	payment_proof TEXT,
	amount_idr INTEGER DEFAULT 0,
	notes TEXT,
	admin_notes TEXT,
	approved_by INTEGER REFERENCES users(id),
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

ALTER TABLE users ADD COLUMN plan_id INTEGER REFERENCES plans(id);
`,
}
