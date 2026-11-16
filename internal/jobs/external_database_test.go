package jobs

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"sigardapanel/internal/agentclient"
	"sigardapanel/internal/jobmodel"
	"sigardapanel/internal/store"
)

// Jobs that would create, drop or change the accounts of an external
// database never reach the agent, whether its record still exists (found by
// id) or was already removed and another external record with the name
// remains on the server; the same jobs for a panel-managed database do.
func TestExternalDatabaseJobsNeverReachTheAgent(t *testing.T) {
	s := regenWorkerDB(t)
	ctx := t.Context()
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		json.NewEncoder(w).Encode(agentclient.TaskResult{OK: true})
	}))
	defer ts.Close()
	w := NewWorker(s, agentclient.New(ts.URL, "token"))

	srv := &store.Server{Name: "s", Hostname: "s.example.com", Status: "active", AgentToken: "t", AgentURL: sql.NullString{String: ts.URL, Valid: true}}
	s.Servers.Create(ctx, srv)
	site := &store.Site{ServerID: srv.ID, Name: "app", Domain: "app.example.com", Runtime: "php", RootPath: "/home/u/htdocs/app.example.com"}
	if err := s.Sites.Create(ctx, site); err != nil {
		t.Fatal(err)
	}
	ext := &store.Database{SiteID: site.ID, Name: "legacy_app", Type: "mysql", ManagedBy: store.DatabaseManagedByExternal}
	own := &store.Database{SiteID: site.ID, Name: "own_db", Type: "mysql"}
	s.Databases.Create(ctx, ext)
	s.Databases.Create(ctx, own)

	run := func(typ, payload string) *jobmodel.Job {
		job := &jobmodel.Job{Type: typ, SiteID: sql.NullInt64{Int64: site.ID, Valid: true},
			ServerID: sql.NullInt64{Int64: srv.ID, Valid: true}, Payload: payload}
		s.Jobs.Create(ctx, job)
		w.processOne(ctx)
		got, _ := s.Jobs.GetByID(ctx, job.ID)
		return got
	}

	for _, tc := range []struct{ typ, payload string }{
		{jobmodel.TypeDatabaseDelete, `{"database_id":` + itoa(ext.ID) + `,"name":"legacy_app","type":"mysql"}`},
		{jobmodel.TypeDatabaseDelete, `{"database_id":999,"name":"legacy_app","type":"mysql"}`},
		{jobmodel.TypeDatabaseCreate, `{"database_id":` + itoa(ext.ID) + `,"name":"legacy_app","type":"mysql"}`},
		{jobmodel.TypeDBUserCreate, `{"user_id":1,"database_id":` + itoa(ext.ID) + `,"database_name":"legacy_app","type":"mysql","username":"u","password":"p","host_pattern":"localhost","privileges":"ALL"}`},
		{jobmodel.TypeDBUserDelete, `{"user_id":1,"database_id":999,"database_name":"legacy_app","type":"mysql","username":"u","host_pattern":"localhost"}`},
	} {
		got := run(tc.typ, tc.payload)
		if got.Status != jobmodel.StatusFailed || got.Error.String != ExternalDatabaseMessage {
			t.Fatalf("%s %s: status %s error %q", tc.typ, tc.payload, got.Status, got.Error.String)
		}
	}
	if n := calls.Load(); n != 0 {
		t.Fatalf("agent called %d times for an external database", n)
	}

	got := run(jobmodel.TypeDatabaseDelete, `{"database_id":`+itoa(own.ID)+`,"name":"own_db","type":"mysql"}`)
	if got.Error.String == ExternalDatabaseMessage || calls.Load() == 0 {
		t.Fatalf("panel-managed database.delete refused: status %s error %q", got.Status, got.Error.String)
	}
}

func itoa(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}
