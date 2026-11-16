package jobs

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"sigardapanel/internal/agentclient"
	"sigardapanel/internal/buildinfo"
	"sigardapanel/internal/jobmodel"
	"sigardapanel/internal/store"
)

// Jobs that would change an external site on the server never reach the
// agent; other jobs for it (and every job for a panel-managed site) do.
func TestExternalSiteJobsNeverReachTheAgent(t *testing.T) {
	s := regenWorkerDB(t)
	ctx := t.Context()
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		json.NewEncoder(w).Encode(agentclient.TaskResult{OK: true})
	}))
	defer ts.Close()
	w := NewWorker(s, agentclient.New(ts.URL, "token"))

	srv := &store.Server{Name: "s", Hostname: "s.example.com", Status: "active", AgentToken: "t"}
	s.Servers.Create(ctx, srv)
	s.Servers.UpdateAgentVersion(ctx, srv.ID, buildinfo.Version) // agent knows PHP-FPM pools
	ext := &store.Site{ServerID: srv.ID, Name: "cms", Domain: "cms.example.com", Runtime: "php",
		RootPath: "/home/u/htdocs/cms.example.com", ManagedBy: sql.NullString{String: "external", Valid: true}}
	if err := s.Sites.Create(ctx, ext); err != nil {
		t.Fatal(err)
	}
	s.Sites.UpdateStatus(ctx, ext.ID, "active")

	for _, typ := range []string{jobmodel.TypeSiteCreate, jobmodel.TypeSiteDelete, jobmodel.TypeSiteSuspend,
		jobmodel.TypeSiteResume, jobmodel.TypeSSLIssue, jobmodel.TypeSSLRenew, jobmodel.TypeSiteDeploy} {
		job := &jobmodel.Job{Type: typ, SiteID: sql.NullInt64{Int64: ext.ID, Valid: true},
			ServerID: sql.NullInt64{Int64: srv.ID, Valid: true}, Payload: `{"site_id":1,"domain":"cms.example.com"}`}
		s.Jobs.Create(ctx, job)
		w.processOne(ctx)
		got, _ := s.Jobs.GetByID(ctx, job.ID)
		if got.Status != jobmodel.StatusFailed || got.Error.String != ExternalSiteMessage {
			t.Fatalf("%s: status %s error %q", typ, got.Status, got.Error.String)
		}
	}
	if n := calls.Load(); n != 0 {
		t.Fatalf("agent called %d times for an external site", n)
	}

	// The scheduler moves a panel-managed PHP site to its own pool but
	// leaves the external one alone.
	own := &store.Site{ServerID: srv.ID, Name: "own", Domain: "own.example.com", Runtime: "php",
		RootPath: "/home/u/htdocs/own.example.com"}
	if err := s.Sites.Create(ctx, own); err != nil {
		t.Fatal(err)
	}
	s.Sites.UpdateStatus(ctx, own.ID, "active")
	migratePHPPools(ctx, s, time.Now())
	jobs, _ := s.Jobs.List(ctx, store.JobListFilter{Status: jobmodel.StatusQueued})
	if len(jobs) != 1 || !jobs[0].SiteID.Valid || jobs[0].SiteID.Int64 != own.ID {
		t.Fatalf("pool migration queued %d jobs (want 1, for the panel-managed site)", len(jobs))
	}
}
