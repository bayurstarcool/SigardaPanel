package store

import (
	"context"
	"testing"

	"sigardapanel/internal/jobmodel"
)

func TestJobLogAppend_Basic(t *testing.T) {
	db := testDB(t)
	s := New(db)
	ctx := context.Background()

	j := &jobmodel.Job{Type: jobmodel.TypeSiteCreate}
	if err := s.Jobs.Create(ctx, j); err != nil {
		t.Fatalf("create job: %v", err)
	}

	log := &JobLog{JobID: j.ID, LineNumber: 1, Message: "hello", Stream: "stdout"}
	if err := s.JobLogs.Append(ctx, log); err != nil {
		t.Fatalf("append log: %v", err)
	}
	if log.ID == 0 {
		t.Fatal("expected log ID to be set")
	}
}

func TestJobLogGetSince(t *testing.T) {
	db := testDB(t)
	s := New(db)
	ctx := context.Background()

	j := &jobmodel.Job{Type: jobmodel.TypeSiteCreate}
	if err := s.Jobs.Create(ctx, j); err != nil {
		t.Fatalf("create job: %v", err)
	}

	for i := 1; i <= 5; i++ {
		log := &JobLog{JobID: j.ID, LineNumber: i, Message: "line", Stream: "stdout"}
		if err := s.JobLogs.Append(ctx, log); err != nil {
			t.Fatalf("append log %d: %v", i, err)
		}
	}

	logs, err := s.JobLogs.GetSince(ctx, j.ID, 2, 100)
	if err != nil {
		t.Fatalf("get since: %v", err)
	}
	if len(logs) != 3 {
		t.Fatalf("expected 3 logs, got %d", len(logs))
	}
	if logs[0].LineNumber != 3 {
		t.Fatalf("expected first line 3, got %d", logs[0].LineNumber)
	}
}

func TestJobLogGetAll(t *testing.T) {
	db := testDB(t)
	s := New(db)
	ctx := context.Background()

	j := &jobmodel.Job{Type: jobmodel.TypeSiteCreate}
	if err := s.Jobs.Create(ctx, j); err != nil {
		t.Fatalf("create job: %v", err)
	}

	for i := 1; i <= 3; i++ {
		log := &JobLog{JobID: j.ID, LineNumber: i, Message: "msg", Stream: "stderr"}
		if err := s.JobLogs.Append(ctx, log); err != nil {
			t.Fatalf("append log %d: %v", i, err)
		}
	}

	logs, err := s.JobLogs.GetAll(ctx, j.ID)
	if err != nil {
		t.Fatalf("get all: %v", err)
	}
	if len(logs) != 3 {
		t.Fatalf("expected 3 logs, got %d", len(logs))
	}
}

func TestJobLogCount(t *testing.T) {
	db := testDB(t)
	s := New(db)
	ctx := context.Background()

	j := &jobmodel.Job{Type: jobmodel.TypeSiteCreate}
	if err := s.Jobs.Create(ctx, j); err != nil {
		t.Fatalf("create job: %v", err)
	}

	for i := 1; i <= 4; i++ {
		log := &JobLog{JobID: j.ID, LineNumber: i, Message: "msg", Stream: "stdout"}
		if err := s.JobLogs.Append(ctx, log); err != nil {
			t.Fatalf("append log %d: %v", i, err)
		}
	}

	count, err := s.JobLogs.Count(ctx, j.ID)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 4 {
		t.Fatalf("expected count 4, got %d", count)
	}
}
