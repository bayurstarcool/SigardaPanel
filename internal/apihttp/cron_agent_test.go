package apihttp

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"sigardapanel/internal/auth"
	"sigardapanel/internal/store"
)

func TestSiteCronIsInstalledByTheAgent(t *testing.T) {
	var mu sync.Mutex
	var calls []map[string]any
	refuse := true
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, body)
		if refuse {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"ok":false,"message":"not a regular user"}`))
			return
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer agent.Close()
	e, s, ctx := newTestAppWithAllMigrations(t)
	root := roleToken(t, s, ctx, "root", auth.RoleSuperAdmin)
	srv := &store.Server{Name: "s", Hostname: "h", Status: "active", AgentToken: "t", AgentURL: sql.NullString{String: agent.URL, Valid: true}}
	s.Servers.Create(ctx, srv)
	site := &store.Site{ServerID: srv.ID, Name: "x", Domain: "x.example.com", Runtime: "php", RootPath: "/home/u/htdocs/x.example.com",
		Status: "active", SystemUser: sql.NullString{String: "u", Valid: true}}
	if err := s.Sites.Create(ctx, site); err != nil {
		t.Fatal(err)
	}
	s.Sites.UpdateStatus(ctx, site.ID, "active")
	url := "/api/v1/sites/" + strconv.FormatInt(site.ID, 10) + "/cron"

	for _, bad := range []map[string]any{
		{"schedule": "61 * * * *", "command": "php artisan schedule:run"},
		{"schedule": "* * * *", "command": "id"},
		{"schedule": "* * * * *", "command": "id\n* * * * * root sh"},
		{"schedule": "* * * * *", "command": ""},
	} {
		if rec := doRequest(t, e, http.MethodPost, url, bad, root); rec.Code != http.StatusBadRequest {
			t.Errorf("%v = %d %s", bad, rec.Code, rec.Body.String())
		}
	}
	if len(calls) != 0 {
		t.Fatalf("invalid jobs reached the agent: %v", calls)
	}

	job := map[string]any{"schedule": "*/5 * * * *", "command": "php artisan schedule:run"}
	if rec := doRequest(t, e, http.MethodPost, url, job, root); rec.Code != http.StatusBadRequest {
		t.Fatalf("refused by agent = %d %s", rec.Code, rec.Body.String())
	}
	if jobs, _ := s.Cron.ListBySiteID(ctx, site.ID); len(jobs) != 0 {
		t.Fatal("a job the server refused was recorded")
	}

	refuse = false
	if rec := doRequest(t, e, http.MethodPost, url, job, root); rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	last := calls[len(calls)-1]
	if jobs, _ := last["jobs"].([]any); len(jobs) != 1 || last["system_user"] != "u" || last["root_path"] != site.RootPath {
		t.Fatalf("sync call = %v", last)
	}
	jobs, _ := s.Cron.ListBySiteID(ctx, site.ID)
	if len(jobs) != 1 {
		t.Fatalf("stored = %v", jobs)
	}

	// Transfer is refused while cron jobs exist.
	s.Users.Create(ctx, &store.User{Username: "other", Email: "x@example.com", PasswordHash: "x", Role: auth.RoleUser})
	if rec := doRequest(t, e, http.MethodPost, "/api/v1/sites/"+strconv.FormatInt(site.ID, 10)+"/transfer",
		map[string]string{"new_owner_email": "x@example.com"}, root); rec.Code != http.StatusConflict {
		t.Fatalf("transfer with cron jobs = %d %s", rec.Code, rec.Body.String())
	}

	if rec := doRequest(t, e, http.MethodDelete, url+"/"+strconv.FormatInt(jobs[0].ID, 10), nil, root); rec.Code != http.StatusOK {
		t.Fatalf("delete = %d %s", rec.Code, rec.Body.String())
	}
	last = calls[len(calls)-1]
	if j, _ := last["jobs"].([]any); len(j) != 0 {
		t.Fatalf("delete sync = %v", last)
	}
	if jobs, _ := s.Cron.ListBySiteID(ctx, site.ID); len(jobs) != 0 {
		t.Fatal("job not deleted")
	}
}
