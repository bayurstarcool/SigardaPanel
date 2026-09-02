package apihttp

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"sigardapanel/internal/auth"
	"sigardapanel/internal/store"
)

func TestLogsTail_Success(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)
	srvID := createServer(t, s, ctx)

	adminUser, _ := s.Users.GetByUsername(ctx, "admin")
	site := &store.Site{ServerID: srvID, Name: "Logs", Domain: "logs.example.com", Runtime: "static", OwnerID: nullInt(adminUser.ID)}
	s.Sites.Create(ctx, site)

	rec := doRequest(t, e, "GET", "/api/v1/sites/"+strconv.FormatInt(site.ID, 10)+"/logs", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result struct {
		Data map[string]any `json:"data"`
	}
	json.NewDecoder(rec.Body).Decode(&result)
	if result.Data["type"] != "access" {
		t.Fatalf("expected type access, got %v", result.Data["type"])
	}
}

func TestLogsTail_InvalidID_400(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)

	rec := doRequest(t, e, "GET", "/api/v1/sites/abc/logs", nil, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestLogsTail_NotFound_404(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)

	rec := doRequest(t, e, "GET", "/api/v1/sites/9999/logs", nil, token)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestLogsTail_OtherUser_404(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	userToken := newUserToken(t, s, ctx)
	srvID := createServer(t, s, ctx)

	otherUser := &store.User{Username: "other", Email: "other@example.com", PasswordHash: "pass", Role: auth.RoleUser}
	s.Users.Create(ctx, otherUser)

	site := &store.Site{ServerID: srvID, Name: "Logs", Domain: "logs-other.example.com", Runtime: "static", OwnerID: nullInt(otherUser.ID)}
	s.Sites.Create(ctx, site)

	rec := doRequest(t, e, "GET", "/api/v1/sites/"+strconv.FormatInt(site.ID, 10)+"/logs", nil, userToken)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestLogsTail_InvalidType_400(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)
	srvID := createServer(t, s, ctx)

	adminUser, _ := s.Users.GetByUsername(ctx, "admin")
	site := &store.Site{ServerID: srvID, Name: "Logs", Domain: "logs.example.com", Runtime: "static", OwnerID: nullInt(adminUser.ID)}
	s.Sites.Create(ctx, site)

	rec := doRequest(t, e, "GET", "/api/v1/sites/"+strconv.FormatInt(site.ID, 10)+"/logs?type=invalid", nil, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestLogsTail_LinesParam(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)
	srvID := createServer(t, s, ctx)

	adminUser, _ := s.Users.GetByUsername(ctx, "admin")
	site := &store.Site{ServerID: srvID, Name: "Logs", Domain: "logs.example.com", Runtime: "static", OwnerID: nullInt(adminUser.ID)}
	s.Sites.Create(ctx, site)

	rec := doRequest(t, e, "GET", "/api/v1/sites/"+strconv.FormatInt(site.ID, 10)+"/logs?type=error&lines=500", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result struct {
		Data map[string]any `json:"data"`
	}
	json.NewDecoder(rec.Body).Decode(&result)
	if result.Data["type"] != "error" {
		t.Fatalf("expected type error, got %v", result.Data["type"])
	}
}
