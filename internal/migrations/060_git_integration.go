package migrations

// GitIntegrationSchema: per-user connections to GitHub / GitLab / Gitea
// (token encrypted), the git settings of a site connected through one, and
// the deploy history of every site.
var GitIntegrationSchema = Migration{
	Version: 60,
	SQL: `
CREATE TABLE IF NOT EXISTS git_connections (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    provider TEXT NOT NULL,
    base_url TEXT NOT NULL,
    username TEXT NOT NULL DEFAULT '',
    token TEXT NOT NULL,
    allow_private INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, provider, base_url, username)
);
CREATE TABLE IF NOT EXISTS site_git (
    site_id INTEGER PRIMARY KEY,
    connection_id INTEGER,
    provider TEXT NOT NULL DEFAULT '',
    repo_full_name TEXT NOT NULL DEFAULT '',
    repo_html_url TEXT NOT NULL DEFAULT '',
    deploy_key_id TEXT NOT NULL DEFAULT '',
    deploy_key_public TEXT NOT NULL DEFAULT '',
    webhook_id TEXT NOT NULL DEFAULT '',
    auto_deploy INTEGER NOT NULL DEFAULT 1,
    build_enabled INTEGER NOT NULL DEFAULT 1,
    deploy_script TEXT NOT NULL DEFAULT '',
    known_hosts TEXT NOT NULL DEFAULT '',
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS deployments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    site_id INTEGER NOT NULL,
    job_id INTEGER,
    user_id INTEGER,
    triggered_by TEXT NOT NULL DEFAULT 'manual',
    branch TEXT NOT NULL DEFAULT '',
    target_commit TEXT NOT NULL DEFAULT '',
    commit_sha TEXT NOT NULL DEFAULT '',
    commit_message TEXT NOT NULL DEFAULT '',
    commit_author TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'queued',
    log TEXT NOT NULL DEFAULT '',
    error TEXT NOT NULL DEFAULT '',
    provider_handle TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at DATETIME,
    finished_at DATETIME
);
CREATE INDEX IF NOT EXISTS idx_deployments_site ON deployments(site_id, id);
CREATE INDEX IF NOT EXISTS idx_deployments_job ON deployments(job_id);
`,
}
