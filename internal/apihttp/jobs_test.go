package apihttp

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"testing"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/db"
	"sigardapanel/internal/jobmodel"
	"sigardapanel/internal/migrations"
	"sigardapanel/internal/store"
)

func newTestAppWithJobsMigration(t *testing.T) (*echo.Echo, *store.Store, context.Context) {
	t.Helper()
	f, _ := os.CreateTemp("", "api_test_*.db")
	t.Cleanup(func() { os.Remove(f.Name()) })
	d, err := db.Open(f.Name())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	migrations.Up(d, migrations.AllMigrations)
	s := store.New(d)
	e := NewRouter(s, d, nil)
	return e, s, t.Context()
}

func TestJobList_Empty(t *testing.T) {
	e, s, ctx := newTestAppWithJobsMigration(t)
	token := newAdminToken(t, s, ctx)

	rec := doRequest(t, e, "GET", "/api/v1/jobs", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result struct {
		Data struct {
			Jobs  []map[string]any `json:"jobs"`
			Count int              `json:"count"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Data.Count != 0 {
		t.Fatalf("expected count 0, got %d", result.Data.Count)
	}
}

func TestJobList_WithJobs(t *testing.T) {
	e, s, ctx := newTestAppWithJobsMigration(t)
	token := newAdminToken(t, s, ctx)

	for i := 0; i < 2; i++ {
		j := &jobmodel.Job{Type: jobmodel.TypeSiteCreate}
		if err := s.Jobs.Create(ctx, j); err != nil {
			t.Fatalf("create job %d: %v", i, err)
		}
	}

	rec := doRequest(t, e, "GET", "/api/v1/jobs", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result struct {
		Data struct {
			Jobs  []map[string]any `json:"jobs"`
			Count int              `json:"count"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Data.Count != 2 {
		t.Fatalf("expected count 2, got %d", result.Data.Count)
	}
}

func TestJobGet_Found(t *testing.T) {
	e, s, ctx := newTestAppWithJobsMigration(t)
	token := newAdminToken(t, s, ctx)

	j := &jobmodel.Job{Type: jobmodel.TypeSSLIssue}
	if err := s.Jobs.Create(ctx, j); err != nil {
		t.Fatalf("create job: %v", err)
	}

	rec := doRequest(t, e, "GET", "/api/v1/jobs/"+strconv.FormatInt(j.ID, 10), nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Data["type"] != jobmodel.TypeSSLIssue {
		t.Fatalf("unexpected type: %v", result.Data["type"])
	}
}

func TestJobGet_NotFound(t *testing.T) {
	e, s, ctx := newTestAppWithJobsMigration(t)
	token := newAdminToken(t, s, ctx)

	rec := doRequest(t, e, "GET", "/api/v1/jobs/99999", nil, token)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestJobGet_InvalidID(t *testing.T) {
	e, s, ctx := newTestAppWithJobsMigration(t)
	token := newAdminToken(t, s, ctx)

	rec := doRequest(t, e, "GET", "/api/v1/jobs/abc", nil, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestJobCancel_Queued(t *testing.T) {
	e, s, ctx := newTestAppWithJobsMigration(t)
	token := newAdminToken(t, s, ctx)

	j := &jobmodel.Job{Type: jobmodel.TypeSiteCreate}
	if err := s.Jobs.Create(ctx, j); err != nil {
		t.Fatalf("create job: %v", err)
	}

	rec := doRequest(t, e, "POST", "/api/v1/jobs/"+strconv.FormatInt(j.ID, 10)+"/cancel", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	got, err := s.Jobs.GetByID(ctx, j.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if got.Status != jobmodel.StatusCanceled {
		t.Fatalf("expected canceled, got %s", got.Status)
	}
}

func TestJobCancel_Running(t *testing.T) {
	e, s, ctx := newTestAppWithJobsMigration(t)
	token := newAdminToken(t, s, ctx)

	j := &jobmodel.Job{Type: jobmodel.TypeSiteCreate}
	if err := s.Jobs.Create(ctx, j); err != nil {
		t.Fatalf("create job: %v", err)
	}
	if _, err := s.Jobs.ClaimNext(ctx, ""); err != nil {
		t.Fatalf("claim next: %v", err)
	}

	rec := doRequest(t, e, "POST", "/api/v1/jobs/"+strconv.FormatInt(j.ID, 10)+"/cancel", nil, token)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
}

func TestJobList_RequiresAuth(t *testing.T) {
	e, _, _ := newTestAppWithJobsMigration(t)

	rec := doRequest(t, e, "GET", "/api/v1/jobs", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestJobCancel_RequiresAuth(t *testing.T) {
	e, _, _ := newTestAppWithJobsMigration(t)

	rec := doRequest(t, e, "POST", "/api/v1/jobs/1/cancel", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestJobGet_RequiresAuth(t *testing.T) {
	e, _, _ := newTestAppWithJobsMigration(t)

	rec := doRequest(t, e, "GET", "/api/v1/jobs/1", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
