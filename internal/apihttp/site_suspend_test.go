package apihttp

import (
	"net/http"
	"strconv"
	"testing"

	"sigardapanel/internal/jobmodel"
	"sigardapanel/internal/store"
)

func TestSuspend_AdminOwner_QueuesJob(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	adminToken := newAdminToken(t, s, ctx)
	srvID := createServer(t, s, ctx)
	admin, _ := s.Users.GetByUsername(ctx, "admin")

	site := &store.Site{ServerID: srvID, Name: "S", Domain: "suspend.example.com", Runtime: "static", OwnerID: nullInt(admin.ID)}
	if err := s.Sites.Create(ctx, site); err != nil {
		t.Fatal(err)
	}
	s.Sites.UpdateStatus(ctx, site.ID, "active")
	path := "/api/v1/sites/" + strconv.FormatInt(site.ID, 10)

	rec := doRequest(t, e, "POST", path+"/suspend", nil, adminToken)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("suspend: expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	jobs, _ := s.Jobs.List(ctx, store.JobListFilter{SiteID: site.ID, Type: jobmodel.TypeSiteSuspend})
	if len(jobs) != 1 {
		t.Fatalf("expected 1 suspend job, got %d", len(jobs))
	}

	// A second toggle while the first is queued is refused.
	if rec := doRequest(t, e, "POST", path+"/suspend", nil, adminToken); rec.Code != http.StatusConflict {
		t.Fatalf("double suspend: expected 409, got %d", rec.Code)
	}
	// Resume needs status=suspended.
	if rec := doRequest(t, e, "POST", path+"/resume", nil, adminToken); rec.Code != http.StatusConflict {
		t.Fatalf("resume active site: expected 409, got %d", rec.Code)
	}

	s.Jobs.MarkCompleted(ctx, jobs[0].ID, true, "{}", "")
	s.Sites.UpdateStatus(ctx, site.ID, "suspended")
	if rec := doRequest(t, e, "POST", path+"/resume", nil, adminToken); rec.Code != http.StatusAccepted {
		t.Fatalf("resume: expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSuspend_RegularUserForbidden(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	userToken := newUserToken(t, s, ctx)
	srvID := createServer(t, s, ctx)
	user, _ := s.Users.GetByUsername(ctx, "regular")

	site := &store.Site{ServerID: srvID, Name: "S", Domain: "own.example.com", Runtime: "static", OwnerID: nullInt(user.ID)}
	s.Sites.Create(ctx, site)
	s.Sites.UpdateStatus(ctx, site.ID, "suspended")

	// Even the owner cannot lift a suspension.
	rec := doRequest(t, e, "POST", "/api/v1/sites/"+strconv.FormatInt(site.ID, 10)+"/resume", nil, userToken)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}
