package apihttp

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"sigardapanel/internal/store"
)

func TestServerRegenerateToken_IsStored(t *testing.T) {
	e, s, ctx := newTestApp(t)
	token := newSuperAdminToken(t, s, ctx)
	srv := &store.Server{Name: "r", Hostname: "r.example.com", AgentToken: "old-token",
		AgentURL: sql.NullString{String: "http://203.0.113.5:7710", Valid: true}}
	if err := s.Servers.Create(ctx, srv); err != nil {
		t.Fatal(err)
	}

	rec := doRequest(t, e, "POST", "/api/v1/servers/"+strconv.FormatInt(srv.ID, 10)+"/token", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var res struct {
		Data struct {
			AgentToken string `json:"agent_token"`
		} `json:"data"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&res)
	got, _ := s.Servers.GetByID(ctx, srv.ID)
	if res.Data.AgentToken == "" || got.AgentToken != res.Data.AgentToken {
		t.Fatalf("returned token %q was not stored (db has %q)", res.Data.AgentToken, got.AgentToken)
	}
}

func TestServerDelete_Guards(t *testing.T) {
	e, s, ctx := newTestApp(t)
	token := newSuperAdminToken(t, s, ctx)

	local := &store.Server{Name: "Local Server", Hostname: "panel", AgentToken: "t",
		AgentURL: sql.NullString{String: "http://127.0.0.1:7710", Valid: true}}
	remote := &store.Server{Name: "r", Hostname: "r.example.com", AgentToken: "t",
		AgentURL: sql.NullString{String: "http://203.0.113.5:7710", Valid: true}}
	for _, srv := range []*store.Server{local, remote} {
		if err := s.Servers.Create(ctx, srv); err != nil {
			t.Fatal(err)
		}
	}
	site := &store.Site{ServerID: remote.ID, Name: "a", Domain: "a.example.com", Runtime: "static"}
	if err := s.Sites.Create(ctx, site); err != nil {
		t.Fatal(err)
	}

	if rec := doRequest(t, e, "DELETE", "/api/v1/servers/"+strconv.FormatInt(local.ID, 10), nil, token); rec.Code != http.StatusConflict {
		t.Fatalf("deleting the local server: expected 409, got %d", rec.Code)
	}
	if rec := doRequest(t, e, "DELETE", "/api/v1/servers/"+strconv.FormatInt(remote.ID, 10), nil, token); rec.Code != http.StatusConflict {
		t.Fatalf("deleting a server with sites: expected 409, got %d", rec.Code)
	}
	if err := s.Sites.Delete(ctx, site.ID); err != nil {
		t.Fatal(err)
	}
	if rec := doRequest(t, e, "DELETE", "/api/v1/servers/"+strconv.FormatInt(remote.ID, 10), nil, token); rec.Code != http.StatusOK {
		t.Fatalf("deleting an empty server: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestServerUpdate_AgentURL(t *testing.T) {
	e, s, ctx := newTestApp(t)
	token := newSuperAdminToken(t, s, ctx)
	srv := &store.Server{Name: "r", Hostname: "r.example.com", AgentToken: "secret-token",
		AgentURL: sql.NullString{String: "http://1:7710", Valid: true}}
	if err := s.Servers.Create(ctx, srv); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/servers/" + strconv.FormatInt(srv.ID, 10)

	if rec := doRequest(t, e, "PATCH", path, map[string]any{"agent_url": "ftp://x"}, token); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid agent_url: expected 400, got %d", rec.Code)
	}
	rec := doRequest(t, e, "PATCH", path, map[string]any{"agent_url": "http://203.0.113.5:7710"}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); strings.Contains(body, "secret-token") {
		t.Fatal("PATCH response leaks the agent token")
	}
	got, _ := s.Servers.GetByID(ctx, srv.ID)
	if got.AgentURL.String != "http://203.0.113.5:7710" {
		t.Fatalf("agent_url = %q", got.AgentURL.String)
	}
	if got.AgentToken != "secret-token" {
		t.Fatal("agent token changed by an unrelated update")
	}
}
