package store

import (
	"testing"
	"time"

	"sigardapanel/internal/jobmodel"
)

func TestJobCreate_Basic(t *testing.T) {
	db := testDB(t)
	s := NewJobStore(db)

	j := &jobmodel.Job{
		Type:    jobmodel.TypeSiteCreate,
		Payload: `{"domain":"example.com"}`,
	}
	if err := s.Create(ctx, j); err != nil {
		t.Fatalf("create job: %v", err)
	}
	if j.ID == 0 {
		t.Fatal("job ID not set after create")
	}
	if j.Status != jobmodel.StatusQueued {
		t.Fatalf("expected status %s, got %s", jobmodel.StatusQueued, j.Status)
	}
	if j.MaxAttempts != 3 {
		t.Fatalf("expected max_attempts 3, got %d", j.MaxAttempts)
	}
}

func TestJobGetByID_Found(t *testing.T) {
	db := testDB(t)
	s := NewJobStore(db)

	j := &jobmodel.Job{Type: jobmodel.TypeSSLIssue, Payload: `{}`}
	if err := s.Create(ctx, j); err != nil {
		t.Fatalf("create job: %v", err)
	}

	got, err := s.GetByID(ctx, j.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if got == nil {
		t.Fatal("job not found")
	}
	if got.Type != jobmodel.TypeSSLIssue {
		t.Fatalf("expected type %s, got %s", jobmodel.TypeSSLIssue, got.Type)
	}
}

func TestJobGetByID_NotFound(t *testing.T) {
	db := testDB(t)
	s := NewJobStore(db)

	got, err := s.GetByID(ctx, 99999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil for nonexistent job")
	}
}

func TestJobList_FilteredByStatus(t *testing.T) {
	db := testDB(t)
	s := NewJobStore(db)

	for i := 0; i < 3; i++ {
		j := &jobmodel.Job{Type: jobmodel.TypeSiteCreate}
		if err := s.Create(ctx, j); err != nil {
			t.Fatalf("create job %d: %v", i, err)
		}
	}

	list, err := s.List(ctx, JobListFilter{Limit: 10})
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 jobs, got %d", len(list))
	}
	if err := s.UpdateStatus(ctx, list[0].ID, jobmodel.StatusRunning); err != nil {
		t.Fatalf("update status: %v", err)
	}

	queued, err := s.List(ctx, JobListFilter{Status: jobmodel.StatusQueued, Limit: 10})
	if err != nil {
		t.Fatalf("list queued: %v", err)
	}
	if len(queued) != 2 {
		t.Fatalf("expected 2 queued jobs, got %d", len(queued))
	}

	running, err := s.List(ctx, JobListFilter{Status: jobmodel.StatusRunning, Limit: 10})
	if err != nil {
		t.Fatalf("list running: %v", err)
	}
	if len(running) != 1 {
		t.Fatalf("expected 1 running job, got %d", len(running))
	}
}

func TestJobClaimNext_Atomic(t *testing.T) {
	db := testDB(t)
	s := NewJobStore(db)

	j := &jobmodel.Job{Type: jobmodel.TypeSiteDelete}
	if err := s.Create(ctx, j); err != nil {
		t.Fatalf("create job: %v", err)
	}

	claimed, err := s.ClaimNext(ctx, "")
	if err != nil {
		t.Fatalf("claim next: %v", err)
	}
	if claimed == nil {
		t.Fatal("expected a claimed job")
	}
	if claimed.Status != jobmodel.StatusRunning {
		t.Fatalf("expected status running, got %s", claimed.Status)
	}
	if claimed.Attempts != 1 {
		t.Fatalf("expected attempts 1, got %d", claimed.Attempts)
	}

	got, err := s.GetByID(ctx, j.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if got.Status != jobmodel.StatusRunning {
		t.Fatalf("expected stored status running, got %s", got.Status)
	}
	if got.Attempts != 1 {
		t.Fatalf("expected stored attempts 1, got %d", got.Attempts)
	}
}

func TestJobClaimNext_Empty(t *testing.T) {
	db := testDB(t)
	s := NewJobStore(db)

	claimed, err := s.ClaimNext(ctx, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if claimed != nil {
		t.Fatal("expected nil when no queued jobs")
	}
}

func TestJobCancel_FromQueued(t *testing.T) {
	db := testDB(t)
	s := NewJobStore(db)

	j := &jobmodel.Job{Type: jobmodel.TypeBackupCreate}
	if err := s.Create(ctx, j); err != nil {
		t.Fatalf("create job: %v", err)
	}

	if err := s.Cancel(ctx, j.ID); err != nil {
		t.Fatalf("cancel job: %v", err)
	}

	got, err := s.GetByID(ctx, j.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if got.Status != jobmodel.StatusCanceled {
		t.Fatalf("expected status canceled, got %s", got.Status)
	}
}

func TestJobCancel_RunningNotCanceled(t *testing.T) {
	db := testDB(t)
	s := NewJobStore(db)

	j := &jobmodel.Job{Type: jobmodel.TypeSiteDeploy}
	if err := s.Create(ctx, j); err != nil {
		t.Fatalf("create job: %v", err)
	}
	claimed, err := s.ClaimNext(ctx, "")
	if err != nil {
		t.Fatalf("claim next: %v", err)
	}
	if claimed == nil {
		t.Fatal("expected claimed job")
	}

	if err := s.Cancel(ctx, j.ID); err != nil {
		t.Fatalf("cancel job: %v", err)
	}

	got, err := s.GetByID(ctx, j.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if got.Status != jobmodel.StatusRunning {
		t.Fatalf("expected status still running, got %s", got.Status)
	}
}

func TestJobMarkCompleted(t *testing.T) {
	db := testDB(t)
	s := NewJobStore(db)

	j := &jobmodel.Job{Type: jobmodel.TypeSSLRenew}
	if err := s.Create(ctx, j); err != nil {
		t.Fatalf("create job: %v", err)
	}
	if err := s.MarkCompleted(ctx, j.ID, true, `{"ok":true}`, ""); err != nil {
		t.Fatalf("mark completed: %v", err)
	}

	got, err := s.GetByID(ctx, j.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if got.Status != jobmodel.StatusSucceeded {
		t.Fatalf("expected status succeeded, got %s", got.Status)
	}
	if got.Result != `{"ok":true}` {
		t.Fatalf("unexpected result: %s", got.Result)
	}
	if !got.CompletedAt.Valid {
		t.Fatal("expected completed_at to be set")
	}
}

func TestScheduleRetry(t *testing.T) {
	db := testDB(t)
	s := NewJobStore(db)

	j := &jobmodel.Job{Type: jobmodel.TypeSiteCreate}
	if err := s.Create(ctx, j); err != nil {
		t.Fatalf("create job: %v", err)
	}
	if err := s.UpdateStatus(ctx, j.ID, jobmodel.StatusRunning); err != nil {
		t.Fatalf("update status: %v", err)
	}

	retryAt := time.Now().Add(5 * time.Second).UTC().Truncate(time.Second)
	if err := s.ScheduleRetry(ctx, j.ID, retryAt); err != nil {
		t.Fatalf("schedule retry: %v", err)
	}
	if err := s.SetError(ctx, j.ID, "temporary error"); err != nil {
		t.Fatalf("set error: %v", err)
	}

	got, err := s.GetByID(ctx, j.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if got.Status != jobmodel.StatusQueued {
		t.Fatalf("expected status queued, got %s", got.Status)
	}
	if !got.NextRetryAt.Valid {
		t.Fatalf("expected next_retry_at to be set")
	}
	if got.NextRetryAt.Time.UTC().Truncate(time.Second) != retryAt.UTC().Truncate(time.Second) {
		t.Fatalf("expected next_retry_at %v, got %v", retryAt, got.NextRetryAt.Time)
	}
	if !got.Error.Valid || got.Error.String != "temporary error" {
		t.Fatalf("expected error set, got %v", got.Error)
	}
}

func TestClaimNext_RespectsNextRetryAt_Future(t *testing.T) {
	db := testDB(t)
	s := NewJobStore(db)

	j := &jobmodel.Job{Type: jobmodel.TypeSiteCreate}
	if err := s.Create(ctx, j); err != nil {
		t.Fatalf("create job: %v", err)
	}

	// Schedule retry in the future
	retryAt := time.Now().Add(1 * time.Hour)
	if err := s.ScheduleRetry(ctx, j.ID, retryAt); err != nil {
		t.Fatalf("schedule retry: %v", err)
	}

	claimed, err := s.ClaimNext(ctx, "")
	if err != nil {
		t.Fatalf("claim next: %v", err)
	}
	if claimed != nil {
		t.Fatalf("expected no job claimed because next_retry_at is in the future, got job %d", claimed.ID)
	}
}

func TestClaimNext_RespectsNextRetryAt_Past(t *testing.T) {
	db := testDB(t)
	s := NewJobStore(db)

	j := &jobmodel.Job{Type: jobmodel.TypeSiteCreate}
	if err := s.Create(ctx, j); err != nil {
		t.Fatalf("create job: %v", err)
	}

	// Schedule retry in the past
	retryAt := time.Now().Add(-1 * time.Hour)
	if err := s.ScheduleRetry(ctx, j.ID, retryAt); err != nil {
		t.Fatalf("schedule retry: %v", err)
	}

	claimed, err := s.ClaimNext(ctx, "")
	if err != nil {
		t.Fatalf("claim next: %v", err)
	}
	if claimed == nil {
		t.Fatal("expected job to be claimed because next_retry_at is in the past")
	}
	if claimed.ID != j.ID {
		t.Fatalf("expected claimed job id %d, got %d", j.ID, claimed.ID)
	}
}

func TestClaimNext_NoNextRetryAt(t *testing.T) {
	db := testDB(t)
	s := NewJobStore(db)

	j := &jobmodel.Job{Type: jobmodel.TypeSiteCreate}
	if err := s.Create(ctx, j); err != nil {
		t.Fatalf("create job: %v", err)
	}

	claimed, err := s.ClaimNext(ctx, "")
	if err != nil {
		t.Fatalf("claim next: %v", err)
	}
	if claimed == nil {
		t.Fatal("expected job to be claimed when next_retry_at is NULL")
	}
	if claimed.ID != j.ID {
		t.Fatalf("expected claimed job id %d, got %d", j.ID, claimed.ID)
	}
}


