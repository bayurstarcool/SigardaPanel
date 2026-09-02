package migrations

// UserCreatedBySchema adds created_by to users table for role-based user management.
var UserCreatedBySchema = Migration{
	Version: 14,
	SQL: `
ALTER TABLE users ADD COLUMN created_by INTEGER REFERENCES users(id);
CREATE INDEX IF NOT EXISTS idx_users_created_by ON users(created_by);
`,
}
