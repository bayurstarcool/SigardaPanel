package migrations

// GitDeploySchema adds git deployment columns to the sites table.
var GitDeploySchema = Migration{
	Version: 6,
	SQL: `
ALTER TABLE sites ADD COLUMN git_repo TEXT;
ALTER TABLE sites ADD COLUMN git_branch TEXT DEFAULT 'main';
ALTER TABLE sites ADD COLUMN git_commit TEXT;
ALTER TABLE sites ADD COLUMN deploy_key TEXT;
ALTER TABLE sites ADD COLUMN webhook_secret TEXT;
`,
}
