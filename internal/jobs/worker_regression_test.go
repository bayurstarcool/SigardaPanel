package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"sigardapanel/internal/agentclient"
	"sigardapanel/internal/db"
	"sigardapanel/internal/jobmodel"
	"sigardapanel/internal/migrations"
	"sigardapanel/internal/store"
)

func fullWorkerDB(t *testing.T) (*store.Store, context.Context) {
	t.Helper()
	f, _ := os.CreateTemp("", "worker_full_*.db")
	f.Close()
	d, err := db.Open(f.Name())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close(); os.Remove(f.Name()) })
	if err := migrations.Up(d, migrations.AllMigrations); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return store.New(d), context.Background()
}

// agentServer stands in for a server's agent and records how often it is hit.
func agentServer(t *testing.T, s *store.Store, ctx context.Context, res agentclient.TaskResult) (int64, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		json.NewEncoder(w).Encode(res)
	}))
	t.Cleanup(ts.Close)
	srv := &store.Server{Name: "srv", Hostname: "srv.test", Status: "active", AgentToken: "tok",
		AgentURL: sql.NullString{String: ts.URL, Valid: true}}
	if err := s.Servers.Create(ctx, srv); err != nil {
		t.Fatalf("create server: %v", err)
	}
	return srv.ID, &calls
}

// The concurrency guard used to count the backup job it had just claimed, so
// every backup was re-queued forever and the agent was never called.
func TestBackupJob_NotBlockedByItself(t *testing.T) {
	s, ctx := fullWorkerDB(t)
	srvID, calls := agentServer(t, s, ctx, agentclient.TaskResult{OK: true, Path: "/var/backups/x.tar.gz"})

	j := &jobmodel.Job{Type: jobmodel.TypeBackupCreate, Payload: `{}`,
		ServerID: sql.NullInt64{Int64: srvID, Valid: true}}
	if err := s.Jobs.Create(ctx, j); err != nil {
		t.Fatalf("create job: %v", err)
	}

	NewWorker(s, nil).processOne(ctx)

	if calls.Load() != 1 {
		t.Fatalf("expected the agent to be called once, got %d", calls.Load())
	}
	got, _ := s.Jobs.GetByID(ctx, j.ID)
	if got.Status != jobmodel.StatusSucceeded {
		t.Fatalf("expected succeeded, got %s", got.Status)
	}
}

// A successful renewal must record the certificate's new end date.
func TestSSLRenew_UpdatesExpiry(t *testing.T) {
	s, ctx := fullWorkerDB(t)
	srvID, _ := agentServer(t, s, ctx, agentclient.TaskResult{OK: true, Expires: "Jan  2 15:04:05 2027 GMT"})

	site := &store.Site{ServerID: srvID, Name: "s", Domain: "s.example.com", Runtime: "static"}
	if err := s.Sites.Create(ctx, site); err != nil {
		t.Fatalf("create site: %v", err)
	}
	old := time.Now().Add(5 * 24 * time.Hour)
	if err := s.Sites.UpdateSSL(ctx, site.ID, "active", "Let's Encrypt", old); err != nil {
		t.Fatal(err)
	}

	j := &jobmodel.Job{Type: jobmodel.TypeSSLRenew, Payload: `{"site_id":1,"domain":"s.example.com"}`,
		ServerID: sql.NullInt64{Int64: srvID, Valid: true}, SiteID: sql.NullInt64{Int64: site.ID, Valid: true}}
	if err := s.Jobs.Create(ctx, j); err != nil {
		t.Fatalf("create job: %v", err)
	}
	NewWorker(s, nil).processOne(ctx)

	got, _ := s.Sites.GetByID(ctx, site.ID)
	want := time.Date(2027, 1, 2, 15, 4, 5, 0, time.UTC)
	if !got.SSLExpiresAt.Valid || !got.SSLExpiresAt.Time.UTC().Equal(want) {
		t.Fatalf("expected expiry %v, got %v (valid=%v)", want, got.SSLExpiresAt.Time, got.SSLExpiresAt.Valid)
	}
}

func TestParseCertEndDate(t *testing.T) {
	for in, ok := range map[string]bool{
		"Jan  2 15:04:05 2027 GMT": true,
		"Dec 31 23:59:59 2026 GMT": true,
		"":                         false,
		"not a date":               false,
	} {
		if _, got := parseCertEndDate(in); got != ok {
			t.Errorf("parseCertEndDate(%q) ok=%v, want %v", in, got, ok)
		}
	}
}

// A failed auto-renewal must not be re-queued (and re-alerted) every minute.
func TestSSLAutoRenew_BacksOffAfterFailure(t *testing.T) {
	s, ctx := fullWorkerDB(t)
	srvID, _ := agentServer(t, s, ctx, agentclient.TaskResult{OK: true})
	site := &store.Site{ServerID: srvID, Name: "s", Domain: "s.example.com", Runtime: "static", Status: "active"}
	if err := s.Sites.Create(ctx, site); err != nil {
		t.Fatal(err)
	}
	if err := s.Sites.UpdateSSL(ctx, site.ID, "active", "Let's Encrypt", time.Now().Add(5*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	queuedRenewals := func() int {
		jobs, _ := s.Jobs.List(ctx, store.JobListFilter{Type: jobmodel.TypeSSLRenew, SiteID: site.ID, Status: jobmodel.StatusQueued})
		return len(jobs)
	}

	failed := &jobmodel.Job{Type: jobmodel.TypeSSLRenew, Payload: `{}`, SiteID: sql.NullInt64{Int64: site.ID, Valid: true}}
	if err := s.Jobs.Create(ctx, failed); err != nil {
		t.Fatal(err)
	}
	if err := s.Jobs.MarkCompleted(ctx, failed.ID, false, "", "certbot failed"); err != nil {
		t.Fatal(err)
	}
	runSSLAutoRenew(ctx, s)
	if n := queuedRenewals(); n != 0 {
		t.Fatalf("renewal re-queued right after a failure (%d queued)", n)
	}

	// Without a recent failure the renewal is queued as before.
	if _, err := s.DB().ExecContext(ctx, `UPDATE jobs SET updated_at = datetime('now','-7 hours') WHERE id = ?`, failed.ID); err != nil {
		t.Fatal(err)
	}
	runSSLAutoRenew(ctx, s)
	if n := queuedRenewals(); n != 1 {
		t.Fatalf("expected one queued renewal after the back-off, got %d", n)
	}
}
