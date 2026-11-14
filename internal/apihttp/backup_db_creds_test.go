package apihttp

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"sigardapanel/internal/auth"
	"sigardapanel/internal/store"
)

// Database backups use the database's own panel-managed account (no dedicated
// backup account is provisioned by default); "both" puts the dumps in the
// files archive; nothing queued is an error, not "started".
func TestSiteBackupsIncludeDatabasesWithTheirOwnAccount(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	owner := roleToken(t, s, ctx, "owner", auth.RoleUser)
	ownerUser, _ := s.Users.GetByUsername(ctx, "owner")
	srv := &store.Server{Name: "local", Hostname: "h", Status: "active", AgentToken: "t"}
	s.Servers.Create(ctx, srv)
	site := &store.Site{ServerID: srv.ID, Name: "x", Domain: "x.example.com", Runtime: "php", RootPath: "/home/owner/htdocs/x.example.com",
		Status: "active", OwnerID: sql.NullInt64{Int64: ownerUser.ID, Valid: true}}
	if err := s.Sites.Create(ctx, site); err != nil {
		t.Fatal(err)
	}
	d := &store.Database{SiteID: site.ID, Name: "shop", Type: "mysql"}
	if err := s.Databases.Create(ctx, d); err != nil {
		t.Fatal(err)
	}
	sid := strconv.FormatInt(site.ID, 10)

	// No account yet: both requests say why instead of queueing nothing.
	rec := doRequest(t, e, http.MethodPost, "/api/v1/sites/"+sid+"/backups/database", map[string]any{}, owner)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "shop") {
		t.Fatalf("database backup without account = %d %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, e, http.MethodPost, "/api/v1/sites/"+sid+"/backups", map[string]any{"backup_type": "both"}, owner)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("both without account = %d %s", rec.Code, rec.Body.String())
	}

	enc, err := s.DBStore.EncryptPassword("Shop-pass-123")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DBUsers.Create(ctx, &store.DBUser{DatabaseID: d.ID, Username: "shopu", PasswordHash: "x", EncryptedPassword: enc, HostPattern: "localhost", Privileges: "ALL"}); err != nil {
		t.Fatal(err)
	}

	rec = doRequest(t, e, http.MethodPost, "/api/v1/sites/"+sid+"/backups", map[string]any{"backup_type": "both"}, owner)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("both = %d %s", rec.Code, rec.Body.String())
	}
	var res struct {
		Data struct {
			JobID int64 `json:"job_id"`
		} `json:"data"`
	}
	json.Unmarshal(rec.Body.Bytes(), &res)
	job, err := s.Jobs.GetByID(ctx, res.Data.JobID)
	if err != nil || job == nil {
		t.Fatalf("job: %v", err)
	}
	var payload struct {
		BackupType string              `json:"backup_type"`
		Databases  []map[string]string `json:"databases"`
	}
	json.Unmarshal([]byte(job.Payload), &payload)
	if payload.BackupType != "both" || len(payload.Databases) != 1 || payload.Databases[0]["user"] != "shopu" || payload.Databases[0]["password"] == "" {
		t.Fatalf("both payload = %s", job.Payload)
	}

	rec = doRequest(t, e, http.MethodPost, "/api/v1/sites/"+sid+"/backups/database", map[string]any{}, owner)
	if rec.Code != http.StatusAccepted || !strings.Contains(rec.Body.String(), `"job_ids":[`) {
		t.Fatalf("database backup = %d %s", rec.Code, rec.Body.String())
	}

	if rec := doRequest(t, e, http.MethodPost, "/api/v1/sites/"+sid+"/backups", map[string]any{"backup_type": "nope"}, owner); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad backup_type = %d", rec.Code)
	}
}
