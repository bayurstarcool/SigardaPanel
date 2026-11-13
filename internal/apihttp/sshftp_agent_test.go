package apihttp

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"sigardapanel/internal/auth"
	"sigardapanel/internal/store"
)

func TestSSHFTPAccountsAreProvisionedByTheAgent(t *testing.T) {
	var mu sync.Mutex
	var calls []map[string]any
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		body["_path"] = r.URL.Path
		mu.Lock()
		calls = append(calls, body)
		mu.Unlock()
		if r.URL.Path == "/tasks/sshftp/create" {
			json.NewEncoder(w).Encode(map[string]any{"ok": true, "ssh_public_key": "ssh-ed25519 AAAA test", "ssh_private_key": "PRIVATE"})
			return
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer agent.Close()
	e, s, ctx := newTestAppWithAllMigrations(t)
	root := roleToken(t, s, ctx, "root", auth.RoleSuperAdmin)
	srv := &store.Server{Name: "s", Hostname: "h", Status: "active", AgentToken: "t", AgentURL: sql.NullString{String: agent.URL, Valid: true}}
	s.Servers.Create(ctx, srv)
	site := &store.Site{ServerID: srv.ID, Name: "x", Domain: "x.example.com", Runtime: "static", RootPath: "/home/u/htdocs/x.example.com", Status: "active"}
	s.Sites.Create(ctx, site)
	id := strconv.FormatInt(site.ID, 10)

	rec := doRequest(t, e, http.MethodPost, "/api/v1/sites/"+id+"/ssh-ftp", map[string]any{"username": "deploy-x"}, root)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), "PRIVATE") {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	create := calls[0]
	accts, _ := create["accounts"].([]any)
	if create["_path"] != "/tasks/sshftp/create" || create["site_root"] != site.RootPath || len(accts) != 1 {
		t.Fatalf("create call = %v", create)
	}
	users, _ := s.SSH.ListBySiteID(ctx, site.ID)
	if len(users) != 1 || users[0].Username != "deploy-x" {
		t.Fatalf("stored users = %+v", users)
	}

	// Deleting the site deprovisions its SSH accounts on the server.
	if rec := doRequest(t, e, http.MethodDelete, "/api/v1/sites/"+id, nil, root); rec.Code >= 400 {
		t.Fatalf("site delete = %d %s", rec.Code, rec.Body.String())
	}
	found := false
	for _, c := range calls {
		if c["_path"] == "/tasks/sshftp/delete" && c["username"] == "deploy-x" {
			found = true
		}
	}
	if !found {
		t.Fatalf("site delete did not remove the SSH account on the server: %v", calls)
	}
}
