package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"sigardapanel/internal/agentclient"
	"sigardapanel/internal/gitprovider"
	"sigardapanel/internal/jobmodel"
	"sigardapanel/internal/store"
)

// deployProvider returns the git provider a site is connected through and
// the repository, or nil.
func (w *Worker) deployProvider(ctx context.Context, siteID int64) (gitprovider.Provider, string) {
	g, _ := w.store.Git.GetSiteGit(ctx, siteID)
	if g == nil || g.ConnectionID == 0 || g.RepoFullName == "" {
		return nil, ""
	}
	conn, _ := w.store.Git.GetConnection(ctx, g.ConnectionID)
	if conn == nil {
		return nil, ""
	}
	p, err := gitprovider.Open(gitprovider.Config{Kind: gitprovider.Kind(conn.Provider), BaseURL: conn.BaseURL, Token: conn.Token, AllowPrivate: conn.AllowPrivate})
	if err != nil {
		return nil, ""
	}
	return p, g.RepoFullName
}

func siteURL(site *store.Site) string {
	if site == nil {
		return ""
	}
	if site.SSLStatus.Valid && site.SSLStatus.String == "active" {
		return "https://" + site.Domain
	}
	return "http://" + site.Domain
}

// deployStarting marks the deployment of a site.deploy job running and
// reports it to the git provider (on the commit being deployed).
func (w *Worker) deployStarting(ctx context.Context, job *jobmodel.Job) {
	d, _ := w.store.Git.DeploymentByJob(ctx, job.ID)
	if d == nil {
		return
	}
	handle := ""
	sha := d.TargetCommit
	if sha == "" {
		sha = d.CommitSHA
	}
	if p, repo := w.deployProvider(ctx, d.SiteID); p != nil && sha != "" {
		rctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		h, err := p.StartDeployment(rctx, repo, sha, d.Branch, "SigardaPanel deploy")
		if err == nil {
			_ = p.UpdateDeployment(rctx, repo, h, sha, gitprovider.StateInProgress, "", "Deploying")
			handle = h
		} else {
			slog.Warn("deploy: report start", "site_id", d.SiteID, "error", err)
		}
		cancel()
	}
	_ = w.store.Git.StartDeployment(ctx, d.ID, handle)
}

// deployFinished records the outcome of a site.deploy job: the deployment's
// commit, log and status, the site's deployed commit, the provider status.
func (w *Worker) deployFinished(ctx context.Context, job *jobmodel.Job, result *agentclient.TaskResult, execErr error) {
	site, _ := w.store.Sites.GetByID(ctx, job.SiteID.Int64)
	if execErr == nil && result != nil && result.Commit != "" && site != nil {
		_ = w.store.Sites.UpdateGitCommit(ctx, site.ID, result.Commit)
	}
	d, _ := w.store.Git.DeploymentByJob(ctx, job.ID)
	if d == nil {
		return
	}
	d.Status = "succeeded"
	if execErr != nil {
		d.Status = "failed"
		d.Error = agentErrPrefix.ReplaceAllString(execErr.Error(), "")
	}
	if result != nil {
		d.CommitSHA, d.CommitMessage, d.CommitAuthor, d.Log = result.Commit, result.CommitMessage, result.CommitAuthor, result.DeployLog
		if d.Log == "" && result.Output != "" {
			d.Log = result.Output
		}
	}
	if d.Log == "" && execErr != nil {
		d.Log = execErr.Error()
	}
	if err := w.store.Git.FinishDeployment(ctx, d); err != nil {
		slog.Error("deploy: record outcome", "deployment", d.ID, "error", err)
	}
	_ = w.store.Git.PruneDeployments(ctx, d.SiteID, 50)

	sha := d.CommitSHA
	if sha == "" {
		sha = d.TargetCommit
	}
	if p, repo := w.deployProvider(ctx, d.SiteID); p != nil && sha != "" && site != nil {
		rctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		handle := d.ProviderHandle
		if fresh, _ := w.store.Git.GetDeployment(ctx, d.ID); fresh != nil && fresh.ProviderHandle != "" {
			handle = fresh.ProviderHandle
		}
		if handle == "" {
			handle, _ = p.StartDeployment(rctx, repo, sha, d.Branch, "SigardaPanel deploy")
		}
		state, desc := gitprovider.StateSuccess, fmt.Sprintf("Deployed to %s", site.Domain)
		if execErr != nil {
			state, desc = gitprovider.StateFailure, "Deploy failed: "+firstLine(d.Error)
		}
		if handle != "" {
			if err := p.UpdateDeployment(rctx, repo, handle, sha, state, siteURL(site), desc); err != nil {
				slog.Warn("deploy: report outcome", "site_id", d.SiteID, "error", err)
			}
		}
	}
	if execErr != nil && site != nil {
		w.store.NotifySiteUsers(ctx, site.ID, "deploy_failed", "Deploy failed: "+site.Domain, firstLine(d.Error), "error", "site", site.ID)
	}
}

var agentErrPrefix = regexp.MustCompile(`^agent (error|returned) \d+: `)

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// deployResultJSON keeps logs out of the job's result column (they live in
// the deployment), so the jobs list stays small.
func deployResultJSON(result *agentclient.TaskResult) string {
	if result == nil {
		return ""
	}
	r := *result
	r.DeployLog = ""
	b, _ := json.Marshal(r)
	return string(b)
}
