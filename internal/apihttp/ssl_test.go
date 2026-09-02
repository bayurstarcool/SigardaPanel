package apihttp

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"sigardapanel/internal/auth"
	"sigardapanel/internal/jobmodel"
	"sigardapanel/internal/store"
)

func TestSSLStatus_Found(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)
	srvID := createServer(t, s, ctx)

	adminUser, _ := s.Users.GetByUsername(ctx, "admin")
	site := &store.Site{ServerID: srvID, Name: "SSL", Domain: "ssl.example.com", Runtime: "static", OwnerID: nullInt(adminUser.ID)}
	s.Sites.Create(ctx, site)

	rec := doRequest(t, e, "GET", "/api/v1/sites/"+strconv.FormatInt(site.ID, 10)+"/ssl", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result struct {
		Data map[string]any `json:"data"`
	}
	json.NewDecoder(rec.Body).Decode(&result)
	if result.Data["ssl_status"] != "none" {
		t.Fatalf("expected ssl_status none, got %v", result.Data["ssl_status"])
	}
}

func TestSSLStatus_NotFound_404(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)

	rec := doRequest(t, e, "GET", "/api/v1/sites/9999/ssl", nil, token)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestSSLStatus_OtherUser_404(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	userToken := newUserToken(t, s, ctx)
	srvID := createServer(t, s, ctx)

	otherUser := &store.User{Username: "other", Email: "other@example.com", PasswordHash: "pass", Role: auth.RoleUser}
	s.Users.Create(ctx, otherUser)

	site := &store.Site{ServerID: srvID, Name: "SSL", Domain: "ssl-other.example.com", Runtime: "static", OwnerID: nullInt(otherUser.ID)}
	s.Sites.Create(ctx, site)

	rec := doRequest(t, e, "GET", "/api/v1/sites/"+strconv.FormatInt(site.ID, 10)+"/ssl", nil, userToken)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestSSLStatus_InvalidID_400(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)

	rec := doRequest(t, e, "GET", "/api/v1/sites/abc/ssl", nil, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestSSLIssue_Success_AsOwner(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	userToken := newUserToken(t, s, ctx)
	srvID := createServer(t, s, ctx)

	regularUser, _ := s.Users.GetByUsername(ctx, "regular")
	site := &store.Site{ServerID: srvID, Name: "SSL", Domain: "ssl-issue.example.com", Runtime: "static", OwnerID: nullInt(regularUser.ID)}
	s.Sites.Create(ctx, site)

	rec := doRequest(t, e, "POST", "/api/v1/sites/"+strconv.FormatInt(site.ID, 10)+"/ssl/issue", map[string]string{"email": "admin@example.com"}, userToken)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var result struct {
		Data struct {
			JobID  float64 `json:"job_id"`
			Status string  `json:"status"`
		} `json:"data"`
	}
	json.NewDecoder(rec.Body).Decode(&result)
	if result.Data.JobID == 0 {
		t.Fatal("expected non-zero job_id")
	}
	if result.Data.Status != jobmodel.StatusQueued {
		t.Fatalf("expected status %s, got %s", jobmodel.StatusQueued, result.Data.Status)
	}
}

func TestSSLIssue_InvalidEmailFallback_202(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	userToken := newUserToken(t, s, ctx)
	srvID := createServer(t, s, ctx)

	regularUser, _ := s.Users.GetByUsername(ctx, "regular")
	site := &store.Site{ServerID: srvID, Name: "SSL", Domain: "ssl-badmail.example.com", Runtime: "static", OwnerID: nullInt(regularUser.ID)}
	s.Sites.Create(ctx, site)

	// Invalid email in body is ignored; resolved from owner email instead → 202
	rec := doRequest(t, e, "POST", "/api/v1/sites/"+strconv.FormatInt(site.ID, 10)+"/ssl/issue", map[string]string{"email": "not-an-email"}, userToken)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}
}

func TestSSLIssue_OtherUser_404(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	userToken := newUserToken(t, s, ctx)
	srvID := createServer(t, s, ctx)

	otherUser := &store.User{Username: "other", Email: "other@example.com", PasswordHash: "pass", Role: auth.RoleUser}
	s.Users.Create(ctx, otherUser)

	site := &store.Site{ServerID: srvID, Name: "SSL", Domain: "ssl-issue-other.example.com", Runtime: "static", OwnerID: nullInt(otherUser.ID)}
	s.Sites.Create(ctx, site)

	rec := doRequest(t, e, "POST", "/api/v1/sites/"+strconv.FormatInt(site.ID, 10)+"/ssl/issue", map[string]string{"email": "admin@example.com"}, userToken)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestSSLRenew_Success(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	userToken := newUserToken(t, s, ctx)
	srvID := createServer(t, s, ctx)

	regularUser, _ := s.Users.GetByUsername(ctx, "regular")
	site := &store.Site{ServerID: srvID, Name: "SSL", Domain: "ssl-renew.example.com", Runtime: "static", OwnerID: nullInt(regularUser.ID)}
	s.Sites.Create(ctx, site)

	rec := doRequest(t, e, "POST", "/api/v1/sites/"+strconv.FormatInt(site.ID, 10)+"/ssl/renew", nil, userToken)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var result struct {
		Data struct {
			JobID  float64 `json:"job_id"`
			Status string  `json:"status"`
		} `json:"data"`
	}
	json.NewDecoder(rec.Body).Decode(&result)
	if result.Data.JobID == 0 {
		t.Fatal("expected non-zero job_id")
	}
	if result.Data.Status != jobmodel.StatusQueued {
		t.Fatalf("expected status %s, got %s", jobmodel.StatusQueued, result.Data.Status)
	}
}
