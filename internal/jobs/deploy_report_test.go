package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sigardapanel/internal/agentclient"
	"sigardapanel/internal/gitprovider"
	"sigardapanel/internal/jobmodel"
	"sigardapanel/internal/store"
)

type statusRecorder struct {
	gitprovider.Provider
	events []string
}

func (s *statusRecorder) StartDeployment(_ context.Context, _, sha, _, _ string) (string, error) {
	s.events = append(s.events, "start "+sha)
	return "h1", nil
}
func (s *statusRecorder) UpdateDeployment(_ context.Context, _, h, sha string, st gitprovider.State, url, _ string) error {
	s.events = append(s.events, string(st)+" "+h+" "+url)
	return nil
}

func TestDeployJobRecordsHistoryAndReports(t *testing.T) {
	s := regenWorkerDB(t)
	ctx := t.Context()
	rec := &statusRecorder{}
	prev := gitprovider.Open
	gitprovider.Open = func(gitprovider.Config) (gitprovider.Provider, error) { return rec, nil }
	defer func() { gitprovider.Open = prev }()

	fail := false
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			w.WriteHeader(http.StatusUnprocessableEntity)
			json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "npm run build failed", "deploy_log": "▸ npm run build\nboom", "commit": "c2", "commit_message": "broken"})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "commit": "c1", "commit_message": "good", "commit_author": "Ana", "deploy_log": "▸ Fetch main\nok"})
	}))
	defer agent.Close()
	w := NewWorker(s, agentclient.New(agent.URL, "token"))

	owner := &store.User{Username: "ana", Email: "a@example.com", PasswordHash: "x", Role: "user"}
	s.Users.Create(ctx, owner)
	srv := &store.Server{Name: "s", Hostname: "s", Status: "active", AgentToken: "token", AgentURL: sql.NullString{String: agent.URL, Valid: true}}
	s.Servers.Create(ctx, srv)
	site := &store.Site{ServerID: srv.ID, Name: "n", Domain: "shop.example.com", Runtime: "php", RootPath: "/home/ana/htdocs/shop.example.com",
		Status: "active", OwnerID: sql.NullInt64{Int64: owner.ID, Valid: true}}
	s.Sites.Create(ctx, site)
	conn := &store.GitConnection{UserID: owner.ID, Provider: "github", BaseURL: "https://github.com", Username: "ana", Token: "t"}
	s.Git.CreateConnection(ctx, conn)
	s.Git.SaveSiteGit(ctx, &store.SiteGit{SiteID: site.ID, ConnectionID: conn.ID, Provider: "github", RepoFullName: "ana/shop", DeployKeyPublic: "k", AutoDeploy: true, BuildEnabled: true})

	queue := func(target string) *store.Deployment {
		d := &store.Deployment{SiteID: site.ID, TriggeredBy: "push", Branch: "main", CommitSHA: "c1", TargetCommit: target}
		s.Git.CreateDeployment(ctx, d)
		j := &jobmodel.Job{Type: jobmodel.TypeSiteDeploy, ServerID: sql.NullInt64{Int64: srv.ID, Valid: true}, SiteID: sql.NullInt64{Int64: site.ID, Valid: true}, Payload: `{"site_id":1}`, MaxAttempts: 1}
		s.Jobs.Create(ctx, j)
		s.Git.SetDeploymentJob(ctx, d.ID, j.ID)
		return d
	}

	d := queue("")
	w.processOne(ctx)
	got, _ := s.Git.GetDeployment(ctx, d.ID)
	if got.Status != "succeeded" || got.CommitMessage != "good" || got.CommitAuthor != "Ana" || got.Log == "" || got.StartedAt == nil || got.FinishedAt == nil {
		t.Fatalf("deployment = %+v", got)
	}
	if st, _ := s.Sites.GetByID(ctx, site.ID); st.GitCommit.String != "c1" {
		t.Fatalf("site commit = %q", st.GitCommit.String)
	}
	if len(rec.events) < 3 || rec.events[0] != "start c1" || rec.events[len(rec.events)-1] != "success h1 http://shop.example.com" {
		t.Fatalf("provider events = %v", rec.events)
	}
	// The job result does not carry the log (it lives in the deployment).
	j, _ := s.Jobs.GetByID(ctx, got.JobID)
	if j == nil || j.Result == "" || strings.Contains(j.Result, "Fetch main") {
		t.Fatalf("job result = %+v", j)
	}

	fail = true
	rec.events = nil
	d2 := queue("c2")
	w.processOne(ctx)
	got, _ = s.Git.GetDeployment(ctx, d2.ID)
	if got.Status != "failed" || got.Log != "▸ npm run build\nboom" || got.Error == "" {
		t.Fatalf("failed deployment = %+v", got)
	}
	if st, _ := s.Sites.GetByID(ctx, site.ID); st.GitCommit.String != "c1" {
		t.Fatal("a failed deploy changed the site's deployed commit")
	}
	if rec.events[len(rec.events)-1][:7] != "failure" {
		t.Fatalf("provider events = %v", rec.events)
	}
}
