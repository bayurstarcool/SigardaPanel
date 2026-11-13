package store

import (
	"strings"
	"testing"

	"sigardapanel/internal/jobmodel"
)

func TestQueueBackupFileDelete(t *testing.T) {
	s := New(testDB(t))
	srv := &Server{Name: "s", Hostname: "h", Status: "active", AgentToken: "t"}
	s.Servers.Create(ctx, srv)
	site := &Site{ServerID: srv.ID, Name: "n", Domain: "q.example.com", Runtime: "static", RootPath: "/home/u/htdocs/q.example.com"}
	if err := s.Sites.Create(ctx, site); err != nil {
		t.Fatal(err)
	}
	if err := s.QueueBackupFileDelete(ctx, 9, site.ID, "s3://bucket/x.tar.gz", false); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ { // the second call is a duplicate
		if err := s.QueueBackupFileDelete(ctx, 9, site.ID, "/var/backups/sigardapanel/x.tar.gz", true); err != nil {
			t.Fatal(err)
		}
	}
	jobs, _ := s.Jobs.List(ctx, JobListFilter{})
	if len(jobs) != 1 {
		t.Fatalf("jobs = %d, want 1", len(jobs))
	}
	j := jobs[0]
	if j.Type != jobmodel.TypeBackupDelete || !j.ServerID.Valid || j.ServerID.Int64 != srv.ID || !strings.Contains(j.Payload, `"clear_storage_path":true`) {
		t.Fatalf("job = %+v", j)
	}
}
