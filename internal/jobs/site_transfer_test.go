package jobs

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sigardapanel/internal/agentclient"
	"sigardapanel/internal/jobmodel"
	"sigardapanel/internal/store"
)

// The panel records a transfer only after the agent moved the files, and
// then repoints the vhost at the new root.
func TestSiteTransferUpdatesRecordAfterAgent(t *testing.T) {
	s := regenWorkerDB(t)
	ctx := t.Context()
	fail := true
	var paths []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if fail {
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]any{"message": "target exists"})
			return
		}
		json.NewEncoder(w).Encode(agentclient.TaskResult{OK: true})
	}))
	defer ts.Close()
	w := NewWorker(s, agentclient.New(ts.URL, "token"))

	alice := &store.User{Username: "alice", Email: "a@example.com", PasswordHash: "x", Role: "user"}
	bob := &store.User{Username: "bob", Email: "b@example.com", PasswordHash: "x", Role: "user"}
	s.Users.Create(ctx, alice)
	s.Users.Create(ctx, bob)
	srv := &store.Server{Name: "s", Hostname: "s.example.com", Status: "active", AgentToken: "token", AgentURL: sql.NullString{String: ts.URL, Valid: true}}
	s.Servers.Create(ctx, srv)
	site := &store.Site{ServerID: srv.ID, Name: "n", Domain: "t.example.com", Runtime: "php", RootPath: "/home/alice/htdocs/t.example.com",
		Status: "active", OwnerID: sql.NullInt64{Int64: alice.ID, Valid: true}}
	if err := s.Sites.Create(ctx, site); err != nil {
		t.Fatal(err)
	}
	s.Sites.UpdateStatus(ctx, site.ID, "active")
	payload, _ := json.Marshal(map[string]any{"site_id": site.ID, "domain": site.Domain, "old_root": site.RootPath,
		"new_root": store.TransferRoot("bob", site.Domain), "system_user": "bob", "new_owner_id": bob.ID})
	newJob := func() {
		s.Jobs.Create(ctx, &jobmodel.Job{Type: jobmodel.TypeSiteTransfer, ServerID: sql.NullInt64{Int64: srv.ID, Valid: true},
			SiteID: sql.NullInt64{Int64: site.ID, Valid: true}, Payload: string(payload)})
	}

	newJob()
	w.processOne(ctx)
	if got, _ := s.Sites.GetByID(ctx, site.ID); got.OwnerID.Int64 != alice.ID || got.RootPath != site.RootPath {
		t.Fatalf("a failed move changed the record: owner=%d root=%s", got.OwnerID.Int64, got.RootPath)
	}

	fail = false
	newJob()
	w.processOne(ctx)
	got, _ := s.Sites.GetByID(ctx, site.ID)
	if got.OwnerID.Int64 != bob.ID || got.RootPath != "/home/bob/htdocs/t.example.com" {
		t.Fatalf("record after transfer: owner=%d root=%s", got.OwnerID.Int64, got.RootPath)
	}
	w.processOne(ctx) // the queued vhost regeneration
	if len(paths) != 3 || paths[1] != "/tasks/site/transfer" || !strings.Contains(paths[2], "site") {
		t.Fatalf("agent calls = %v", paths)
	}
	jobs, _ := s.Jobs.List(ctx, store.JobListFilter{})
	found := false
	for _, j := range jobs {
		if j.Type == jobmodel.TypeSiteCreate && strings.Contains(j.Payload, `"vhost_regen":true`) && strings.Contains(j.Payload, "/home/bob/htdocs/") {
			found = true
		}
	}
	if !found {
		t.Fatal("no vhost regeneration queued for the new root")
	}
}
