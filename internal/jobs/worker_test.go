package jobs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"sigardapanel/internal/agentclient"
	"sigardapanel/internal/db"
	"sigardapanel/internal/jobmodel"
	"sigardapanel/internal/migrations"
	"sigardapanel/internal/store"
)

func testWorkerDB(t *testing.T) (*store.Store, context.Context) {
	t.Helper()
	f, _ := os.CreateTemp("", "worker_test_*.db")
	f.Close()
	d, err := db.Open(f.Name())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		d.Close()
		os.Remove(f.Name())
	})
	if err := migrations.Up(d, []migrations.Migration{migrations.InitialSchema, migrations.AuthSchema, migrations.JobsSchema, migrations.SitesOwnerSchema, migrations.SSLSchema, migrations.GitDeploySchema, migrations.BackupsSchema, migrations.AlertsSchema, migrations.JobLogsSchema, migrations.JobRetrySchema, migrations.DatabasesSchema, migrations.ServerAgentURLSchema, migrations.SiteOriginSchema, migrations.JobDescriptionSchema}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	s := store.New(d)
	return s, context.Background()
}

func TestProcessOne_Success(t *testing.T) {
	s, ctx := testWorkerDB(t)

	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewEncoder(w).Encode(agentclient.TaskResult{OK: true, Output: "site created"})
	}))
	defer ts.Close()

	agent := agentclient.New(ts.URL, "token")
	w := NewWorker(s, agent)

	j := &jobmodel.Job{Type: jobmodel.TypeSiteCreate, Payload: `{}`}
	if err := s.Jobs.Create(ctx, j); err != nil {
		t.Fatalf("create job: %v", err)
	}

	w.processOne(ctx)

	got, err := s.Jobs.GetByID(ctx, j.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if got.Status != jobmodel.StatusSucceeded {
		t.Fatalf("expected status succeeded, got %s", got.Status)
	}
	if gotPath != "/tasks/site/create" {
		t.Fatalf("unexpected path: %s", gotPath)
	}
}

func TestProcessOne_RetryableError(t *testing.T) {
	s, ctx := testWorkerDB(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(agentclient.TaskResult{OK: false, Error: "boom"})
	}))
	defer ts.Close()

	agent := agentclient.New(ts.URL, "token")
	w := NewWorker(s, agent)

	j := &jobmodel.Job{Type: jobmodel.TypeSiteDelete, MaxAttempts: 3}
	if err := s.Jobs.Create(ctx, j); err != nil {
		t.Fatalf("create job: %v", err)
	}

	// First attempt fails → scheduled retry
	w.processOne(ctx)
	got, _ := s.Jobs.GetByID(ctx, j.ID)
	if got.Status != jobmodel.StatusQueued {
		t.Fatalf("expected re-queued, got %s", got.Status)
	}
	if got.Attempts != 1 {
		t.Fatalf("expected attempts 1, got %d", got.Attempts)
	}
	if !got.Error.Valid || got.Error.String != "agent returned 500: boom" {
		t.Fatalf("expected error set, got %v", got.Error)
	}
	if !got.NextRetryAt.Valid {
		t.Fatalf("expected next_retry_at set")
	}

	// Clear next_retry_at so second attempt can proceed
	if _, err := s.DB().ExecContext(ctx, `UPDATE jobs SET next_retry_at = NULL WHERE id = ?`, j.ID); err != nil {
		t.Fatalf("clear next_retry_at: %v", err)
	}

	// Second attempt fails → scheduled retry
	w.processOne(ctx)
	got, _ = s.Jobs.GetByID(ctx, j.ID)
	if got.Status != jobmodel.StatusQueued {
		t.Fatalf("expected re-queued, got %s", got.Status)
	}
	if got.Attempts != 2 {
		t.Fatalf("expected attempts 2, got %d", got.Attempts)
	}
}

func TestProcessOne_MaxAttemptsExceeded(t *testing.T) {
	s, ctx := testWorkerDB(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(agentclient.TaskResult{OK: false, Error: "boom"})
	}))
	defer ts.Close()

	agent := agentclient.New(ts.URL, "token")
	w := NewWorker(s, agent)

	j := &jobmodel.Job{Type: jobmodel.TypeSSLIssue, MaxAttempts: 2}
	if err := s.Jobs.Create(ctx, j); err != nil {
		t.Fatalf("create job: %v", err)
	}

	w.processOne(ctx)

	// Clear next_retry_at so second claim can proceed
	if _, err := s.DB().ExecContext(ctx, `UPDATE jobs SET next_retry_at = NULL WHERE id = ?`, j.ID); err != nil {
		t.Fatalf("clear next_retry_at: %v", err)
	}

	w.processOne(ctx)

	got, err := s.Jobs.GetByID(ctx, j.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if got.Status != jobmodel.StatusFailed {
		t.Fatalf("expected status failed, got %s", got.Status)
	}
	if got.Attempts != 2 {
		t.Fatalf("expected attempts 2, got %d", got.Attempts)
	}
}

func TestProcessOne_NoJobs(t *testing.T) {
	s, ctx := testWorkerDB(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("agent should not be called when no jobs")
	}))
	defer ts.Close()

	agent := agentclient.New(ts.URL, "token")
	w := NewWorker(s, agent)

	w.processOne(ctx)
}

func TestBackoff_DelayCalculation(t *testing.T) {
	b := backoffConfig{baseDelay: 1 * time.Second, maxDelay: 5 * time.Minute, jitter: 500 * time.Millisecond}

	cases := []struct {
		attempts int
		minDelay time.Duration
		maxDelay time.Duration
	}{
		{1, 1 * time.Second, 1*time.Second + 500*time.Millisecond},
		{2, 2 * time.Second, 2*time.Second + 500*time.Millisecond},
		{3, 4 * time.Second, 4*time.Second + 500*time.Millisecond},
		{10, 5 * time.Minute, 5*time.Minute + 500*time.Millisecond},
	}

	for _, c := range cases {
		delay := b.delay(c.attempts)
		if delay < c.minDelay {
			t.Errorf("delay(%d) = %v, want >= %v", c.attempts, delay, c.minDelay)
		}
		if delay > c.maxDelay {
			t.Errorf("delay(%d) = %v, want <= %v", c.attempts, delay, c.maxDelay)
		}
	}
}

func TestProcessOne_RetryWithBackoff(t *testing.T) {
	s, ctx := testWorkerDB(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(agentclient.TaskResult{OK: false, Error: "boom"})
	}))
	defer ts.Close()

	agent := agentclient.New(ts.URL, "token")
	w := NewWorker(s, agent)

	j := &jobmodel.Job{Type: jobmodel.TypeSiteCreate, MaxAttempts: 3}
	if err := s.Jobs.Create(ctx, j); err != nil {
		t.Fatalf("create job: %v", err)
	}

	w.processOne(ctx)

	got, err := s.Jobs.GetByID(ctx, j.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if got.Status != jobmodel.StatusQueued {
		t.Fatalf("expected queued, got %s", got.Status)
	}
	if !got.NextRetryAt.Valid {
		t.Fatalf("expected next_retry_at to be set")
	}
	if got.NextRetryAt.Time.Before(time.Now()) {
		t.Fatalf("expected next_retry_at in the future, got %v", got.NextRetryAt.Time)
	}
	if got.Attempts != 1 {
		t.Fatalf("expected attempts 1, got %d", got.Attempts)
	}
}

func TestProcessOne_RetryRespectsMaxAttempts(t *testing.T) {
	s, ctx := testWorkerDB(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(agentclient.TaskResult{OK: false, Error: "boom"})
	}))
	defer ts.Close()

	agent := agentclient.New(ts.URL, "token")
	w := NewWorker(s, agent)

	j := &jobmodel.Job{Type: jobmodel.TypeSSLIssue, MaxAttempts: 1}
	if err := s.Jobs.Create(ctx, j); err != nil {
		t.Fatalf("create job: %v", err)
	}

	w.processOne(ctx)

	got, err := s.Jobs.GetByID(ctx, j.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if got.Status != jobmodel.StatusFailed {
		t.Fatalf("expected status failed, got %s", got.Status)
	}
	if got.Attempts != 1 {
		t.Fatalf("expected attempts 1, got %d", got.Attempts)
	}
	if got.NextRetryAt.Valid {
		t.Fatalf("expected next_retry_at to be NULL after max attempts, got %v", got.NextRetryAt.Time)
	}
}
