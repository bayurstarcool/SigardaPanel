package apihttp

import (
	"net/http"
	"strconv"
	"testing"

	"sigardapanel/internal/jobmodel"
	"sigardapanel/internal/store"
)

func TestJobLogAppend_Success(t *testing.T) {
	e, s, ctx := newTestAppWithJobsMigration(t)
	token := newAdminToken(t, s, ctx)

	j := &jobmodel.Job{Type: jobmodel.TypeSiteCreate}
	if err := s.Jobs.Create(ctx, j); err != nil {
		t.Fatalf("create job: %v", err)
	}

	body := map[string]any{
		"line_number": 1,
		"message":     "hello world",
		"stream":      "stdout",
	}
	rec := doRequest(t, e, "POST", "/api/v1/jobs/"+strconv.FormatInt(j.ID, 10)+"/logs", body, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d, body: %s", rec.Code, rec.Body.String())
	}

	count, err := s.JobLogs.Count(ctx, j.ID)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 log, got %d", count)
	}
}

func TestJobLogAppend_InvalidID_400(t *testing.T) {
	e, s, ctx := newTestAppWithJobsMigration(t)
	token := newAdminToken(t, s, ctx)

	body := map[string]any{"line_number": 1, "message": "test"}
	rec := doRequest(t, e, "POST", "/api/v1/jobs/abc/logs", body, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestJobLogGetSince_Success(t *testing.T) {
	e, s, ctx := newTestAppWithJobsMigration(t)
	token := newAdminToken(t, s, ctx)

	j := &jobmodel.Job{Type: jobmodel.TypeSiteCreate}
	if err := s.Jobs.Create(ctx, j); err != nil {
		t.Fatalf("create job: %v", err)
	}

	for i := 1; i <= 5; i++ {
		log := &store.JobLog{JobID: j.ID, LineNumber: i, Message: "line", Stream: "stdout"}
		if err := s.JobLogs.Append(ctx, log); err != nil {
			t.Fatalf("append log %d: %v", i, err)
		}
	}

	rec := doRequest(t, e, "GET", "/api/v1/jobs/"+strconv.FormatInt(j.ID, 10)+"/logs?since=2", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestJobLogGetSince_InvalidSince_400(t *testing.T) {
	e, s, ctx := newTestAppWithJobsMigration(t)
	token := newAdminToken(t, s, ctx)

	rec := doRequest(t, e, "GET", "/api/v1/jobs/1/logs?since=invalid", nil, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}
