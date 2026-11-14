package migrations

// RBACRolesSchema adds custom roles. users.role keeps the role name; the
// built-in roles are rows too (their permissions come from internal/rbac, so
// role_permissions holds rows only for custom roles).
var RBACRolesSchema = Migration{
	Version: 58,
	SQL: `
CREATE TABLE IF NOT EXISTS roles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    built_in INTEGER NOT NULL DEFAULT 0,
    created_by INTEGER,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS role_permissions (
    role_id INTEGER NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission TEXT NOT NULL,
    PRIMARY KEY (role_id, permission)
);
INSERT OR IGNORE INTO roles (name, description, built_in) VALUES
    ('super_admin', 'Full access to every site, user, server and panel setting', 1),
    ('admin', 'Reseller: manages own sites and the users it created', 1),
    ('user', 'Customer: manages own and assigned sites', 1);
`,
}
