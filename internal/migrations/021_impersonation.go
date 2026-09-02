package migrations

// ImpersonationSchema adds impersonation support to the sessions table.
var ImpersonationSchema = Migration{
	Version: 21,
	SQL: `
ALTER TABLE sessions ADD COLUMN impersonated_by INTEGER REFERENCES users(id);
`,
}
