package apihttp

// Git provider integration (GitHub, GitLab, Gitea): per-user connections,
// connecting a site to a repository (read-only deploy key + push webhook),
// deploy settings, and the deploy history with redeploy / rollback.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
	"sigardapanel/internal/gitprovider"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/jobmodel"
	"sigardapanel/internal/store"
)

func okJSON(c *echo.Context, code int, data any) error {
	return c.JSON(code, httpjson.Success{Data: data, RequestID: c.Response().Header().Get(echo.HeaderXRequestID)})
}

func providerFor(conn *store.GitConnection) (gitprovider.Provider, error) {
	return gitprovider.Open(gitprovider.Config{Kind: gitprovider.Kind(conn.Provider), BaseURL: conn.BaseURL, Token: conn.Token, AllowPrivate: conn.AllowPrivate})
}

type connectionView struct {
	ID        int64     `json:"id"`
	Provider  string    `json:"provider"`
	BaseURL   string    `json:"base_url"`
	Username  string    `json:"username"`
	Sites     int       `json:"sites"`
	CreatedAt time.Time `json:"created_at"`
}

// ownConnection loads a connection of the caller (404 otherwise).
func (h *siteHandler) ownConnection(c *echo.Context, id int64) (*store.GitConnection, error) {
	sess := sessionFromEcho(c)
	if sess == nil {
		return nil, echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	conn, err := h.store.Git.GetConnection(c.Request().Context(), id)
	if err != nil || conn == nil || conn.UserID != sess.UserID {
		return nil, echo.NewHTTPError(http.StatusNotFound, "connection not found")
	}
	return conn, nil
}

// GET /api/v1/git/connections
func (h *siteHandler) listGitConnections(c *echo.Context) error {
	sess := sessionFromEcho(c)
	if sess == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	conns, err := h.store.Git.ListConnections(c.Request().Context(), sess.UserID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	out := []connectionView{}
	for _, cn := range conns {
		ids, _ := h.store.Git.SitesUsingConnection(c.Request().Context(), cn.ID)
		out = append(out, connectionView{ID: cn.ID, Provider: cn.Provider, BaseURL: cn.BaseURL, Username: cn.Username, Sites: len(ids), CreatedAt: cn.CreatedAt})
	}
	return okJSON(c, http.StatusOK, map[string]any{"connections": out})
}

// POST /api/v1/git/connections {provider, base_url, token, allow_private}
func (h *siteHandler) createGitConnection(c *echo.Context) error {
	sess := sessionFromEcho(c)
	if sess == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	ctx := c.Request().Context()
	var body struct {
		Provider     string `json:"provider"`
		BaseURL      string `json:"base_url"`
		Token        string `json:"token"`
		AllowPrivate bool   `json:"allow_private"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	kind := gitprovider.Kind(strings.ToLower(strings.TrimSpace(body.Provider)))
	if kind != gitprovider.GitHub && kind != gitprovider.GitLab && kind != gitprovider.Gitea {
		return echo.NewHTTPError(http.StatusBadRequest, "provider must be github, gitlab or gitea")
	}
	if body.AllowPrivate && !isSessionSuperAdmin(ctx, h.store, sess) {
		return echo.NewHTTPError(http.StatusForbidden, "only a super admin can connect a server on a private network")
	}
	if kind == gitprovider.Gitea && strings.TrimSpace(body.BaseURL) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "the Gitea server URL is required")
	}
	if len(body.Token) > 1024 {
		return echo.NewHTTPError(http.StatusBadRequest, "token too long")
	}
	p, err := gitprovider.Open(gitprovider.Config{Kind: kind, BaseURL: body.BaseURL, Token: body.Token, AllowPrivate: body.AllowPrivate})
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	u, err := p.CurrentUser(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "the token could not be verified: "+err.Error())
	}
	conn := &store.GitConnection{UserID: sess.UserID, Provider: string(kind), BaseURL: p.BaseURL(), Username: u.Login, Token: strings.TrimSpace(body.Token), AllowPrivate: body.AllowPrivate}
	if err := h.store.Git.CreateConnection(ctx, conn); err != nil {
		slog.Error("git: save connection", "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	h.audit.RecordAction(ctx, sess.UserID, "git_connect", "git_connection", conn.ID, map[string]any{"provider": conn.Provider, "base_url": conn.BaseURL, "username": conn.Username})
	return okJSON(c, http.StatusCreated, connectionView{ID: conn.ID, Provider: conn.Provider, BaseURL: conn.BaseURL, Username: conn.Username, CreatedAt: time.Now()})
}

// DELETE /api/v1/git/connections/:id (refused while sites use it)
func (h *siteHandler) deleteGitConnection(c *echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	conn, err := h.ownConnection(c, id)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	ids, _ := h.store.Git.SitesUsingConnection(ctx, conn.ID)
	if len(ids) > 0 {
		var names []string
		for _, sid := range ids {
			if s, _ := h.store.Sites.GetByID(ctx, sid); s != nil {
				names = append(names, s.Domain)
			}
		}
		return echo.NewHTTPError(http.StatusConflict, "disconnect these sites from git first: "+strings.Join(names, ", "))
	}
	if err := h.store.Git.DeleteConnection(ctx, conn.ID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	h.audit.RecordAction(ctx, conn.UserID, "git_disconnect", "git_connection", conn.ID, map[string]any{"provider": conn.Provider, "username": conn.Username})
	return okJSON(c, http.StatusOK, map[string]any{"deleted": true})
}

// GET /api/v1/git/connections/:id/repos?q=
func (h *siteHandler) listGitRepos(c *echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	conn, err := h.ownConnection(c, id)
	if err != nil {
		return err
	}
	p, err := providerFor(conn)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	repos, err := p.ListRepos(c.Request().Context(), c.QueryParam("q"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, err.Error())
	}
	if repos == nil {
		repos = []gitprovider.Repo{}
	}
	return okJSON(c, http.StatusOK, map[string]any{"repos": repos})
}

// GET /api/v1/git/connections/:id/branches?repo=
func (h *siteHandler) listGitBranches(c *echo.Context) error {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	conn, err := h.ownConnection(c, id)
	if err != nil {
		return err
	}
	p, err := providerFor(conn)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	branches, err := p.ListBranches(c.Request().Context(), c.QueryParam("repo"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, err.Error())
	}
	if branches == nil {
		branches = []gitprovider.Branch{}
	}
	return okJSON(c, http.StatusOK, map[string]any{"branches": branches})
}

// webhookTarget returns the URL providers deliver pushes to, or "" when the
// panel has no public https URL (SIGARDAPANEL_PUBLIC_URL).
func webhookTarget(siteID int64) (string, bool) {
	base, err := installerPanelURL()
	if err != nil {
		return "", false
	}
	u, _ := url.Parse(base)
	host := u.Hostname()
	if host == "localhost" {
		return "", false
	}
	verify := true
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() {
			return "", false
		}
		verify = false // an IP address has no publicly trusted certificate; the HMAC still authenticates
	}
	return fmt.Sprintf("%s/api/v1/webhooks/deploy/%d", base, siteID), verify
}

type siteGitView struct {
	Repo          string             `json:"repo"`
	Branch        string             `json:"branch"`
	Commit        string             `json:"commit"`
	Git           *store.SiteGit     `json:"git"`
	Connection    *connectionView    `json:"connection,omitempty"`
	ConnectionOK  bool               `json:"connection_owned"` // the caller owns the connection (can change it)
	WebhookURL    string             `json:"webhook_url"`
	PublicURLSet  bool               `json:"public_url_set"`
	CanManage     bool               `json:"can_manage"` // connect / disconnect (owner or super admin)
	CanConfigure  bool               `json:"can_configure"`
	CanDeploy     bool               `json:"can_deploy"`
	Deployments   []store.Deployment `json:"deployments"`
	ActiveDeploys int                `json:"active_deploys"`
}

// GET /api/v1/sites/:id/git
func (h *siteHandler) getSiteGit(c *echo.Context) error {
	site, err := h.getSiteForUserPerm(c, sitePermRead)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	sess := sessionFromEcho(c)
	g, _ := h.store.Git.GetSiteGit(ctx, site.ID)
	v := siteGitView{
		Repo: site.GitRepo.String, Branch: site.GitBranch.String, Commit: site.GitCommit.String, Git: g,
		CanManage:    h.isSiteOwnerOrSuperAdmin(ctx, sess, site),
		CanConfigure: hasSitePermission(ctx, h.store, sess, site, sitePermAdmin),
		CanDeploy:    hasSitePermission(ctx, h.store, sess, site, sitePermWrite),
	}
	if v.Branch == "" {
		v.Branch = "main"
	}
	if hook, _ := webhookTarget(site.ID); hook != "" {
		v.WebhookURL, v.PublicURLSet = hook, true
	} else {
		v.WebhookURL = fmt.Sprintf("/api/v1/webhooks/deploy/%d", site.ID)
	}
	if g != nil && g.ConnectionID > 0 {
		if conn, _ := h.store.Git.GetConnection(ctx, g.ConnectionID); conn != nil {
			v.Connection = &connectionView{ID: conn.ID, Provider: conn.Provider, BaseURL: conn.BaseURL, Username: conn.Username}
			v.ConnectionOK = sess != nil && conn.UserID == sess.UserID
		}
	}
	v.Deployments, _ = h.store.Git.ListDeployments(ctx, site.ID, 30)
	if v.Deployments == nil {
		v.Deployments = []store.Deployment{}
	}
	for _, d := range v.Deployments {
		if d.Status == "queued" || d.Status == "running" {
			v.ActiveDeploys++
		}
	}
	return okJSON(c, http.StatusOK, v)
}

// deployRequest describes one deploy to queue.
type deployRequest struct {
	Trigger       string // manual, push, rollback, redeploy, connect
	UserID        int64
	TargetCommit  string // deploy exactly this commit (rollback/redeploy)
	CommitSHA     string // expected head (push), for the history until the deploy reports
	CommitMessage string
	CommitAuthor  string
}

// queueSiteDeploy records a deployment and queues its site.deploy job (one
// attempt: a failed build would fail again).
func queueSiteDeploy(ctx context.Context, s *store.Store, site *store.Site, req deployRequest) (*store.Deployment, *jobmodel.Job, error) {
	branch := site.GitBranch.String
	if branch == "" {
		branch = "main"
	}
	d := &store.Deployment{SiteID: site.ID, UserID: req.UserID, TriggeredBy: req.Trigger, Branch: branch, TargetCommit: req.TargetCommit,
		CommitSHA: req.CommitSHA, CommitMessage: req.CommitMessage, CommitAuthor: req.CommitAuthor}
	if err := s.Git.CreateDeployment(ctx, d); err != nil {
		return nil, nil, err
	}
	g, _ := s.Git.GetSiteGit(ctx, site.ID)
	desc := fmt.Sprintf("Deploy site %s", site.Domain)
	switch req.Trigger {
	case "push":
		desc += " (push)"
	case "rollback":
		desc += fmt.Sprintf(" (rollback to %.7s)", req.TargetCommit)
	case "redeploy":
		desc += fmt.Sprintf(" (redeploy %.7s)", req.TargetCommit)
	}
	job := &jobmodel.Job{
		ServerID:    sql.NullInt64{Int64: site.ServerID, Valid: true},
		SiteID:      sql.NullInt64{Int64: site.ID, Valid: true},
		Type:        jobmodel.TypeSiteDeploy,
		Description: desc,
		Payload:     deployPayloadFull(site, branch, g, req.TargetCommit, d.ID),
		MaxAttempts: 1,
	}
	if err := s.Jobs.Create(ctx, job); err != nil {
		return nil, nil, err
	}
	_ = s.Git.SetDeploymentJob(ctx, d.ID, job.ID)
	d.JobID = job.ID
	return d, job, nil
}

// POST /api/v1/sites/:id/git/connect
func (h *siteHandler) connectSiteGit(c *echo.Context) error {
	site, err := h.getSiteForUserPerm(c, sitePermRead)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	sess := sessionFromEcho(c)
	if !h.isSiteOwnerOrSuperAdmin(ctx, sess, site) {
		return echo.NewHTTPError(http.StatusForbidden, "only the site owner can connect it to a repository")
	}
	var body struct {
		ConnectionID int64  `json:"connection_id"`
		Repo         string `json:"repo"`
		Branch       string `json:"branch"`
		AutoDeploy   *bool  `json:"auto_deploy"`
		BuildEnabled *bool  `json:"build_enabled"`
		DeployScript string `json:"deploy_script"`
		DeployNow    *bool  `json:"deploy_now"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if err := validateDeploySettings(body.Branch, body.DeployScript); err != nil {
		return err
	}
	if site.Runtime == "proxy" || site.Runtime == "docker" {
		return echo.NewHTTPError(http.StatusBadRequest, "git deploys are not available for "+site.Runtime+" sites")
	}
	if !site.SystemUser.Valid || site.SystemUser.String == "" {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "the site has no Linux user to deploy as")
	}
	conn, err := h.ownConnection(c, body.ConnectionID)
	if err != nil {
		return err
	}
	p, err := providerFor(conn)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	repo, err := p.GetRepo(ctx, strings.TrimSpace(body.Repo))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, err.Error())
	}
	if !repo.CanAdmin {
		return echo.NewHTTPError(http.StatusForbidden, "your account needs admin (GitHub/Gitea) or maintainer (GitLab) access to "+repo.FullName+" to add a deploy key and a webhook")
	}
	sshURL := repo.SSHURL
	if sshURL == "" {
		return echo.NewHTTPError(http.StatusBadGateway, "the provider did not return an SSH clone URL for "+repo.FullName)
	}
	branch := strings.TrimSpace(body.Branch)
	if branch == "" {
		branch = repo.DefaultBranch
	}
	if branch == "" {
		branch = "main"
	}

	// A previous connection's key and hook go first.
	if prev, _ := h.store.Git.GetSiteGit(ctx, site.ID); prev != nil && prev.ConnectionID > 0 {
		h.removeRemoteGit(ctx, prev)
	}

	// 1. The site's deploy key, made on its server by its Linux user.
	code, out, err := h.siteAgentCall(ctx, site, http.MethodPost, "/tasks/site/deploy-key",
		map[string]any{"site_id": site.ID, "system_user": site.SystemUser.String, "action": "create"}, 30*time.Second)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "agent unreachable: "+err.Error())
	}
	if code >= 400 {
		if code == http.StatusNotFound {
			return echo.NewHTTPError(http.StatusBadGateway, "the agent on this server is too old for git integration; update it (Settings → Panel update → Agents)")
		}
		return agentHTTPError(code, out)
	}
	pub, _ := out["public_key"].(string)
	if !strings.HasPrefix(pub, "ssh-ed25519 ") {
		return echo.NewHTTPError(http.StatusBadGateway, "the agent returned no deploy key")
	}
	g := &store.SiteGit{SiteID: site.ID, ConnectionID: conn.ID, Provider: conn.Provider, RepoFullName: repo.FullName, RepoHTMLURL: repo.HTMLURL,
		DeployKeyPublic: pub, AutoDeploy: body.AutoDeploy == nil || *body.AutoDeploy, BuildEnabled: body.BuildEnabled == nil || *body.BuildEnabled,
		DeployScript: strings.TrimSpace(body.DeployScript)}

	// 2. Read-only deploy key on the repository.
	keyID, err := p.AddDeployKey(ctx, repo.FullName, "SigardaPanel "+site.Domain, pub)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "adding the deploy key failed: "+err.Error())
	}
	g.DeployKeyID = keyID
	g.KnownHosts = strings.Join(p.KnownHosts(ctx), "\n")

	// 3. Push webhook (needs a public panel URL).
	warnings := []string{}
	if hook, verify := webhookTarget(site.ID); hook != "" {
		secret := site.WebhookSecret.String
		if secret == "" {
			secret, err = h.store.Sites.GenerateWebhookSecret(ctx, site.ID)
		}
		if err == nil {
			if id, err := p.CreateWebhook(ctx, repo.FullName, hook, secret, verify); err != nil {
				warnings = append(warnings, "webhook not created ("+err.Error()+"); deploy manually or add it yourself")
			} else {
				g.WebhookID = id
			}
		}
	} else {
		warnings = append(warnings, "no webhook: the panel has no public https URL (SIGARDAPANEL_PUBLIC_URL), so pushes cannot reach it; deploy from the panel")
	}

	if err := h.store.Sites.UpdateGitConfig(ctx, site.ID, sshURL, branch); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if err := h.store.Git.SaveSiteGit(ctx, g); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	h.audit.RecordAction(ctx, sess.UserID, "git_site_connect", "site", site.ID, map[string]any{
		"domain": site.Domain, "provider": conn.Provider, "repo": repo.FullName, "branch": branch, "webhook": g.WebhookID != ""})

	resp := map[string]any{"repo": repo.FullName, "branch": branch, "webhook": g.WebhookID != "", "warnings": warnings}
	if body.DeployNow == nil || *body.DeployNow {
		site, _ = h.store.Sites.GetByID(ctx, site.ID)
		if d, job, err := queueSiteDeploy(ctx, h.store, site, deployRequest{Trigger: "connect", UserID: sess.UserID}); err == nil {
			resp["deployment_id"], resp["job_id"] = d.ID, job.ID
		}
	}
	return okJSON(c, http.StatusOK, resp)
}

func validateDeploySettings(branch, script string) error {
	if branch != "" && (strings.HasPrefix(branch, "-") || strings.Contains(branch, "..") || strings.ContainsAny(branch, " ~^:?*[\\\x00") || len(branch) > 200) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid branch name")
	}
	if len(script) > 16<<10 {
		return echo.NewHTTPError(http.StatusBadRequest, "the deploy script is too long (16 KB)")
	}
	if strings.ContainsRune(script, 0) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid deploy script")
	}
	return nil
}

// PATCH /api/v1/sites/:id/git {branch, auto_deploy, build_enabled, deploy_script, repo_url}
// repo_url sets a repository without a provider connection (public https or
// any URL the server can reach).
func (h *siteHandler) updateSiteGit(c *echo.Context) error {
	site, err := h.getSiteForUserPerm(c, sitePermAdmin)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	var body struct {
		Branch       *string `json:"branch"`
		AutoDeploy   *bool   `json:"auto_deploy"`
		BuildEnabled *bool   `json:"build_enabled"`
		DeployScript *string `json:"deploy_script"`
		RepoURL      *string `json:"repo_url"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	branch := site.GitBranch.String
	if body.Branch != nil {
		branch = strings.TrimSpace(*body.Branch)
	}
	script := ""
	if body.DeployScript != nil {
		script = *body.DeployScript
	}
	if err := validateDeploySettings(branch, script); err != nil {
		return err
	}
	g, _ := h.store.Git.GetSiteGit(ctx, site.ID)
	if g == nil {
		g = &store.SiteGit{SiteID: site.ID, AutoDeploy: true, BuildEnabled: true}
	}
	repo := site.GitRepo.String
	if body.RepoURL != nil {
		if g.ConnectionID > 0 {
			return echo.NewHTTPError(http.StatusConflict, "the site is connected to "+g.RepoFullName+"; disconnect it to use another repository")
		}
		repo = strings.TrimSpace(*body.RepoURL)
		if repo != "" && !validRepoURL(repo) {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid repository URL (https://…, git@host:owner/repo.git or ssh://git@host/…)")
		}
	}
	if body.AutoDeploy != nil {
		g.AutoDeploy = *body.AutoDeploy
	}
	if body.BuildEnabled != nil {
		g.BuildEnabled = *body.BuildEnabled
	}
	if body.DeployScript != nil {
		g.DeployScript = strings.TrimSpace(*body.DeployScript)
	}
	if branch == "" {
		branch = "main"
	}
	if err := h.store.Sites.UpdateGitConfig(ctx, site.ID, repo, branch); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if err := h.store.Git.SaveSiteGit(ctx, g); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	var uid int64
	if sess := sessionFromEcho(c); sess != nil {
		uid = sess.UserID
	}
	h.audit.RecordAction(ctx, uid, "git_site_settings", "site", site.ID, map[string]any{
		"domain": site.Domain, "branch": branch, "repo": repo, "auto_deploy": g.AutoDeploy, "build": g.BuildEnabled, "deploy_script_changed": body.DeployScript != nil})
	return okJSON(c, http.StatusOK, map[string]any{"updated": true})
}

func validRepoURL(u string) bool {
	if len(u) > 500 || strings.ContainsAny(u, " \t\n'\";|&$`\\") || strings.HasPrefix(u, "-") {
		return false
	}
	for _, p := range []string{"https://", "http://", "ssh://git@", "git@"} {
		if strings.HasPrefix(u, p) && len(u) > len(p)+3 {
			return true
		}
	}
	return false
}

// removeRemoteGit deletes the site's webhook and deploy key at the provider
// (best effort; the connection may be gone).
func (h *siteHandler) removeRemoteGit(ctx context.Context, g *store.SiteGit) []string {
	var notes []string
	conn, _ := h.store.Git.GetConnection(ctx, g.ConnectionID)
	if conn == nil {
		return []string{"the git connection no longer exists; remove the deploy key and webhook in the repository settings"}
	}
	p, err := providerFor(conn)
	if err != nil {
		return []string{err.Error()}
	}
	if g.WebhookID != "" {
		if err := p.DeleteWebhook(ctx, g.RepoFullName, g.WebhookID); err != nil {
			notes = append(notes, "webhook: "+err.Error())
		}
	}
	if g.DeployKeyID != "" {
		if err := p.DeleteDeployKey(ctx, g.RepoFullName, g.DeployKeyID); err != nil {
			notes = append(notes, "deploy key: "+err.Error())
		}
	}
	return notes
}

// DELETE /api/v1/sites/:id/git: stop deploying from git. Files stay.
func (h *siteHandler) disconnectSiteGit(c *echo.Context) error {
	site, err := h.getSiteForUserPerm(c, sitePermRead)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	sess := sessionFromEcho(c)
	if !h.isSiteOwnerOrSuperAdmin(ctx, sess, site) {
		return echo.NewHTTPError(http.StatusForbidden, "only the site owner can disconnect it")
	}
	notes := []string{}
	if g, _ := h.store.Git.GetSiteGit(ctx, site.ID); g != nil {
		if g.ConnectionID > 0 {
			notes = append(notes, h.removeRemoteGit(ctx, g)...)
			_, _, _ = h.siteAgentCall(ctx, site, http.MethodPost, "/tasks/site/deploy-key",
				map[string]any{"site_id": site.ID, "system_user": site.SystemUser.String, "action": "delete"}, 30*time.Second)
		}
		_ = h.store.Git.DeleteSiteGit(ctx, site.ID)
	}
	_, _ = h.store.DB().ExecContext(ctx, `UPDATE sites SET git_repo = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, site.ID)
	h.audit.RecordAction(ctx, sess.UserID, "git_site_disconnect", "site", site.ID, map[string]any{"domain": site.Domain, "notes": notes})
	return okJSON(c, http.StatusOK, map[string]any{"disconnected": true, "notes": notes})
}

// GET /api/v1/sites/:id/deployments/:did (with the log)
func (h *siteHandler) getDeployment(c *echo.Context) error {
	site, err := h.getSiteForUserPerm(c, sitePermRead)
	if err != nil {
		return err
	}
	did, _ := strconv.ParseInt(c.Param("did"), 10, 64)
	d, err := h.store.Git.GetDeployment(c.Request().Context(), did)
	if err != nil || d == nil || d.SiteID != site.ID {
		return echo.NewHTTPError(http.StatusNotFound, "deployment not found")
	}
	return okJSON(c, http.StatusOK, d)
}

// POST /api/v1/sites/:id/deployments/:did/{redeploy,rollback}: deploy that
// deployment's commit again.
func (h *siteHandler) redeploy(c *echo.Context) error {
	site, err := h.getSiteForUserPerm(c, sitePermWrite)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	did, _ := strconv.ParseInt(c.Param("did"), 10, 64)
	d, err := h.store.Git.GetDeployment(ctx, did)
	if err != nil || d == nil || d.SiteID != site.ID {
		return echo.NewHTTPError(http.StatusNotFound, "deployment not found")
	}
	if d.CommitSHA == "" {
		return echo.NewHTTPError(http.StatusConflict, "this deployment did not record a commit")
	}
	if !site.GitRepo.Valid || site.GitRepo.String == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "no git repository configured")
	}
	if err := checkSiteDiskQuota(ctx, h.store, site, 0); err != nil {
		return err
	}
	trigger := "redeploy"
	if strings.HasSuffix(c.Path(), "/rollback") {
		trigger = "rollback"
	}
	var uid int64
	if sess := sessionFromEcho(c); sess != nil {
		uid = sess.UserID
	}
	nd, job, err := queueSiteDeploy(ctx, h.store, site, deployRequest{Trigger: trigger, UserID: uid, TargetCommit: d.CommitSHA,
		CommitSHA: d.CommitSHA, CommitMessage: d.CommitMessage, CommitAuthor: d.CommitAuthor})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	h.audit.RecordAction(ctx, uid, "site_"+trigger, "site", site.ID, map[string]any{"domain": site.Domain, "commit": d.CommitSHA})
	return okJSON(c, http.StatusAccepted, map[string]any{"deployment_id": nd.ID, "job_id": job.ID})
}

// pushInfo extracts the pushed branch and head commit from a GitHub, Gitea
// or GitLab push payload.
type pushInfo struct {
	Ref, After, Message, Author string
	Deleted                     bool
}

func parsePush(body []byte) pushInfo {
	var p struct {
		Ref         string `json:"ref"`
		After       string `json:"after"`
		CheckoutSHA string `json:"checkout_sha"`
		Deleted     bool   `json:"deleted"`
		HeadCommit  *struct {
			ID      string `json:"id"`
			Message string `json:"message"`
			Author  struct {
				Name string `json:"name"`
			} `json:"author"`
		} `json:"head_commit"`
		Commits []struct {
			ID      string `json:"id"`
			Message string `json:"message"`
			Author  struct {
				Name string `json:"name"`
			} `json:"author"`
		} `json:"commits"`
	}
	_ = json.Unmarshal(body, &p)
	info := pushInfo{Ref: p.Ref, After: p.After, Deleted: p.Deleted}
	if p.CheckoutSHA != "" {
		info.After = p.CheckoutSHA
	}
	if strings.Trim(info.After, "0") == "" && info.After != "" {
		info.Deleted = true
	}
	if p.HeadCommit != nil {
		info.Message, info.Author = p.HeadCommit.Message, p.HeadCommit.Author.Name
	} else {
		for _, c := range p.Commits {
			if c.ID == info.After {
				info.Message, info.Author = c.Message, c.Author.Name
			}
		}
	}
	if i := strings.IndexByte(info.Message, '\n'); i >= 0 {
		info.Message = info.Message[:i]
	}
	return info
}
