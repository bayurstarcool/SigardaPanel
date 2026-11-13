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

// A backup remembers the provider its S3 copy went to, and the local copy
// is then removed by the agent (a backup.delete job), not by the panel.
func TestBackupRecordsProviderAndQueuesLocalDelete(t *testing.T) {
	s := regenWorkerDB(t)
	ctx := t.Context()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(agentclient.TaskResult{OK: true, Path: "/var/backups/sigardapanel/b.tar.gz", S3Path: "s3://bk/b.tar.gz"})
	}))
	defer ts.Close()
	w := NewWorker(s, agentclient.New(ts.URL, "token"))
	srv := &store.Server{Name: "s", Hostname: "h", Status: "active", AgentToken: "token", AgentURL: sql.NullString{String: ts.URL, Valid: true}}
	s.Servers.Create(ctx, srv)
	site := &store.Site{ServerID: srv.ID, Name: "n", Domain: "b.example.com", Runtime: "static", RootPath: "/home/u/htdocs/b.example.com", Status: "active"}
	s.Sites.Create(ctx, site)
	other := &store.BackupStorageProvider{Name: "default", ProviderType: "s3", Bucket: "other", Endpoint: "s3.example.com", AccessKey: "A2", SecretKey: "x", IsDefault: true}
	used := &store.BackupStorageProvider{Name: "used", ProviderType: "s3", Bucket: "bk", Endpoint: "s3.example.com", AccessKey: "A1", SecretKey: "x"}
	s.BackupProviders.Create(ctx, other)
	s.BackupProviders.Create(ctx, used)
	b := &store.Backup{SiteID: site.ID, StorageType: "s3"}
	if err := s.Backups.Create(ctx, b); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]any{"backup_id": b.ID, "s3_config": map[string]string{"bucket": "bk", "endpoint": "s3.example.com", "access_key": "A1", "secret_key": "x"}})
	s.Jobs.Create(ctx, &jobmodel.Job{Type: jobmodel.TypeBackupCreate, ServerID: sql.NullInt64{Int64: srv.ID, Valid: true},
		SiteID: sql.NullInt64{Int64: site.ID, Valid: true}, Payload: string(payload)})
	w.processOne(ctx)

	got, _ := s.Backups.GetByID(ctx, b.ID)
	if !got.ProviderID.Valid || got.ProviderID.Int64 != used.ID {
		t.Fatalf("provider_id = %+v, want %d", got.ProviderID, used.ID)
	}
	if p := s.BackupProviderFor(ctx, got, site); p == nil || p.ID != used.ID {
		t.Fatalf("resolved provider = %+v", p)
	}
	jobs, _ := s.Jobs.List(ctx, store.JobListFilter{})
	found := false
	for _, j := range jobs {
		if j.Type == jobmodel.TypeBackupDelete && strings.Contains(j.Payload, "b.tar.gz") && strings.Contains(j.Payload, `"clear_storage_path":true`) {
			found = true
		}
	}
	if !found {
		t.Fatal("local copy not handed to the agent for removal")
	}
	w.processOne(ctx) // the backup.delete job succeeds → storage_path cleared
	if got, _ := s.Backups.GetByID(ctx, b.ID); got.StoragePath.Valid && got.StoragePath.String != "" {
		t.Fatalf("storage_path = %q after the agent removed the file", got.StoragePath.String)
	}
}
