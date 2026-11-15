package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"sigardapanel/internal/secrets"
)

// GitConnection is a user's access token for GitHub, GitLab or Gitea.
type GitConnection struct {
	ID           int64     `json:"id"`
	UserID       int64     `json:"user_id"`
	Provider     string    `json:"provider"`
	BaseURL      string    `json:"base_url"`
	Username     string    `json:"username"`
	Token        string    `json:"-"` // decrypted
	AllowPrivate bool      `json:"allow_private"`
	CreatedAt    time.Time `json:"created_at"`
}

// SiteGit is the git setup of a site connected through a GitConnection.
type SiteGit struct {
	SiteID          int64  `json:"site_id"`
	ConnectionID    int64  `json:"connection_id"`
	Provider        string `json:"provider"`
	RepoFullName    string `json:"repo_full_name"`
	RepoHTMLURL     string `json:"repo_html_url"`
	DeployKeyID     string `json:"deploy_key_id"`
	DeployKeyPublic string `json:"deploy_key_public"`
	WebhookID       string `json:"webhook_id"`
	AutoDeploy      bool   `json:"auto_deploy"`
	BuildEnabled    bool   `json:"build_enabled"`
	DeployScript    string `json:"deploy_script"`
	KnownHosts      string `json:"-"`
}

// Deployment is one deploy of a site.
type Deployment struct {
	ID             int64      `json:"id"`
	SiteID         int64      `json:"site_id"`
	JobID          int64      `json:"job_id"`
	UserID         int64      `json:"user_id"`
	TriggeredBy    string     `json:"triggered_by"` // manual, push, rollback, redeploy, connect
	Branch         string     `json:"branch"`
	TargetCommit   string     `json:"target_commit"`
	CommitSHA      string     `json:"commit_sha"`
	CommitMessage  string     `json:"commit_message"`
	CommitAuthor   string     `json:"commit_author"`
	Status         string     `json:"status"` // queued, running, succeeded, failed
	Log            string     `json:"log,omitempty"`
	Error          string     `json:"error,omitempty"`
	ProviderHandle string     `json:"-"`
	CreatedAt      time.Time  `json:"created_at"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
}

// GitStore persists git connections, site git settings and deployments.
type GitStore struct{ db *sql.DB }

// ---- connections

func (s *GitStore) CreateConnection(ctx context.Context, c *GitConnection) error {
	tok, err := secrets.Encrypt(c.Token)
	if err != nil {
		return fmt.Errorf("encrypt token: %w", err)
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO git_connections (user_id, provider, base_url, username, token, allow_private) VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(user_id, provider, base_url, username) DO UPDATE SET token = excluded.token, allow_private = excluded.allow_private`,
		c.UserID, c.Provider, c.BaseURL, c.Username, tok, boolToInt(c.AllowPrivate))
	if err != nil {
		return fmt.Errorf("save git connection: %w", err)
	}
	if id, _ := res.LastInsertId(); id > 0 {
		c.ID = id
	}
	// On conflict the existing row's id is kept.
	_ = s.db.QueryRowContext(ctx, `SELECT id FROM git_connections WHERE user_id = ? AND provider = ? AND base_url = ? AND username = ?`,
		c.UserID, c.Provider, c.BaseURL, c.Username).Scan(&c.ID)
	return nil
}

func scanConnection(sc interface{ Scan(...any) error }) (*GitConnection, error) {
	var c GitConnection
	var tok string
	var allow int
	if err := sc.Scan(&c.ID, &c.UserID, &c.Provider, &c.BaseURL, &c.Username, &tok, &allow, &c.CreatedAt); err != nil {
		return nil, err
	}
	plain, err := secrets.Decrypt(tok)
	if err != nil {
		return nil, fmt.Errorf("decrypt git token: %w", err)
	}
	c.Token, c.AllowPrivate = plain, allow == 1
	return &c, nil
}

const connCols = `id, user_id, provider, base_url, username, token, allow_private, created_at`

func (s *GitStore) GetConnection(ctx context.Context, id int64) (*GitConnection, error) {
	c, err := scanConnection(s.db.QueryRowContext(ctx, `SELECT `+connCols+` FROM git_connections WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return c, err
}

func (s *GitStore) ListConnections(ctx context.Context, userID int64) ([]GitConnection, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+connCols+` FROM git_connections WHERE user_id = ? ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GitConnection
	for rows.Next() {
		c, err := scanConnection(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func (s *GitStore) DeleteConnection(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM git_connections WHERE id = ?`, id)
	return err
}

// SitesUsingConnection returns the ids of sites connected through it.
func (s *GitStore) SitesUsingConnection(ctx context.Context, id int64) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT site_id FROM site_git WHERE connection_id = ? ORDER BY site_id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ---- site git

func (s *GitStore) GetSiteGit(ctx context.Context, siteID int64) (*SiteGit, error) {
	var g SiteGit
	var conn sql.NullInt64
	var auto, build int
	err := s.db.QueryRowContext(ctx, `SELECT site_id, connection_id, provider, repo_full_name, repo_html_url, deploy_key_id, deploy_key_public,
		webhook_id, auto_deploy, build_enabled, deploy_script, known_hosts FROM site_git WHERE site_id = ?`, siteID).
		Scan(&g.SiteID, &conn, &g.Provider, &g.RepoFullName, &g.RepoHTMLURL, &g.DeployKeyID, &g.DeployKeyPublic, &g.WebhookID, &auto, &build, &g.DeployScript, &g.KnownHosts)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	g.ConnectionID, g.AutoDeploy, g.BuildEnabled = conn.Int64, auto == 1, build == 1
	return &g, nil
}

// SaveSiteGit inserts or replaces the site's git settings.
func (s *GitStore) SaveSiteGit(ctx context.Context, g *SiteGit) error {
	var conn any
	if g.ConnectionID > 0 {
		conn = g.ConnectionID
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO site_git (site_id, connection_id, provider, repo_full_name, repo_html_url, deploy_key_id, deploy_key_public,
		webhook_id, auto_deploy, build_enabled, deploy_script, known_hosts, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(site_id) DO UPDATE SET connection_id = excluded.connection_id, provider = excluded.provider, repo_full_name = excluded.repo_full_name,
		repo_html_url = excluded.repo_html_url, deploy_key_id = excluded.deploy_key_id, deploy_key_public = excluded.deploy_key_public,
		webhook_id = excluded.webhook_id, auto_deploy = excluded.auto_deploy, build_enabled = excluded.build_enabled,
		deploy_script = excluded.deploy_script, known_hosts = excluded.known_hosts, updated_at = CURRENT_TIMESTAMP`,
		g.SiteID, conn, g.Provider, g.RepoFullName, g.RepoHTMLURL, g.DeployKeyID, g.DeployKeyPublic, g.WebhookID,
		boolToInt(g.AutoDeploy), boolToInt(g.BuildEnabled), g.DeployScript, g.KnownHosts)
	return err
}

func (s *GitStore) DeleteSiteGit(ctx context.Context, siteID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM site_git WHERE site_id = ?`, siteID)
	return err
}

// ---- deployments

const depCols = `id, site_id, COALESCE(job_id, 0), COALESCE(user_id, 0), triggered_by, branch, target_commit, commit_sha, commit_message,
	commit_author, status, log, error, provider_handle, created_at, started_at, finished_at`

func scanDeployment(sc interface{ Scan(...any) error }) (*Deployment, error) {
	var d Deployment
	var started, finished sql.NullTime
	if err := sc.Scan(&d.ID, &d.SiteID, &d.JobID, &d.UserID, &d.TriggeredBy, &d.Branch, &d.TargetCommit, &d.CommitSHA, &d.CommitMessage,
		&d.CommitAuthor, &d.Status, &d.Log, &d.Error, &d.ProviderHandle, &d.CreatedAt, &started, &finished); err != nil {
		return nil, err
	}
	if started.Valid {
		d.StartedAt = &started.Time
	}
	if finished.Valid {
		d.FinishedAt = &finished.Time
	}
	return &d, nil
}

func (s *GitStore) CreateDeployment(ctx context.Context, d *Deployment) error {
	if d.Status == "" {
		d.Status = "queued"
	}
	var job, user any
	if d.JobID > 0 {
		job = d.JobID
	}
	if d.UserID > 0 {
		user = d.UserID
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO deployments (site_id, job_id, user_id, triggered_by, branch, target_commit, commit_sha, commit_message, commit_author, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, d.SiteID, job, user, d.TriggeredBy, d.Branch, d.TargetCommit, d.CommitSHA, d.CommitMessage, d.CommitAuthor, d.Status)
	if err != nil {
		return fmt.Errorf("create deployment: %w", err)
	}
	d.ID, _ = res.LastInsertId()
	return nil
}

func (s *GitStore) SetDeploymentJob(ctx context.Context, id, jobID int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE deployments SET job_id = ? WHERE id = ?`, jobID, id)
	return err
}

func (s *GitStore) GetDeployment(ctx context.Context, id int64) (*Deployment, error) {
	d, err := scanDeployment(s.db.QueryRowContext(ctx, `SELECT `+depCols+` FROM deployments WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return d, err
}

func (s *GitStore) DeploymentByJob(ctx context.Context, jobID int64) (*Deployment, error) {
	d, err := scanDeployment(s.db.QueryRowContext(ctx, `SELECT `+depCols+` FROM deployments WHERE job_id = ? ORDER BY id DESC LIMIT 1`, jobID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return d, err
}

// ListDeployments returns the site's latest deployments (without logs).
func (s *GitStore) ListDeployments(ctx context.Context, siteID int64, limit int) ([]Deployment, error) {
	if limit <= 0 || limit > 200 {
		limit = 30
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+depCols+` FROM deployments WHERE site_id = ? ORDER BY id DESC LIMIT ?`, siteID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Deployment
	for rows.Next() {
		d, err := scanDeployment(rows)
		if err != nil {
			return nil, err
		}
		d.Log = ""
		out = append(out, *d)
	}
	return out, rows.Err()
}

// StartDeployment marks it running and records the provider's handle.
func (s *GitStore) StartDeployment(ctx context.Context, id int64, handle string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE deployments SET status = 'running', started_at = CURRENT_TIMESTAMP,
		provider_handle = CASE WHEN ? != '' THEN ? ELSE provider_handle END WHERE id = ?`, handle, handle, id)
	return err
}

// FinishDeployment records the outcome.
func (s *GitStore) FinishDeployment(ctx context.Context, d *Deployment) error {
	if len(d.Log) > 256<<10 {
		d.Log = "…(truncated)\n" + d.Log[len(d.Log)-256<<10:]
	}
	_, err := s.db.ExecContext(ctx, `UPDATE deployments SET status = ?, commit_sha = CASE WHEN ? != '' THEN ? ELSE commit_sha END,
		commit_message = CASE WHEN ? != '' THEN ? ELSE commit_message END, commit_author = CASE WHEN ? != '' THEN ? ELSE commit_author END,
		log = ?, error = ?, finished_at = CURRENT_TIMESTAMP, started_at = COALESCE(started_at, CURRENT_TIMESTAMP) WHERE id = ?`,
		d.Status, d.CommitSHA, d.CommitSHA, d.CommitMessage, d.CommitMessage, d.CommitAuthor, d.CommitAuthor, d.Log, d.Error, d.ID)
	return err
}

// PruneDeployments keeps the latest keep deployments of a site.
func (s *GitStore) PruneDeployments(ctx context.Context, siteID int64, keep int) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM deployments WHERE site_id = ? AND id NOT IN (SELECT id FROM deployments WHERE site_id = ? ORDER BY id DESC LIMIT ?)`,
		siteID, siteID, keep)
	return err
}

// DeleteSiteData removes a deleted site's git settings and history.
func (s *GitStore) DeleteSiteData(ctx context.Context, siteID int64) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM deployments WHERE site_id = ?`, siteID); err != nil {
		return err
	}
	return s.DeleteSiteGit(ctx, siteID)
}
