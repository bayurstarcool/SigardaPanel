package apihttp

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"sigardapanel/internal/auth"
	"sigardapanel/internal/jobmodel"
	"sigardapanel/internal/store"
)

func TestBackupCreate_Success(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)
	srvID := createServer(t, s, ctx)

	adminUser, _ := s.Users.GetByUsername(ctx, "admin")
	site := &store.Site{ServerID: srvID, Name: "Bak", Domain: "bak.example.com", Runtime: "static", OwnerID: nullInt(adminUser.ID)}
	s.Sites.Create(ctx, site)

	rec := doRequest(t, e, "POST", "/api/v1/sites/"+strconv.FormatInt(site.ID, 10)+"/backups", nil, token)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var result struct {
		Data struct {
			BackupID int64  `json:"backup_id"`
			JobID    int64  `json:"job_id"`
			Status   string `json:"status"`
		} `json:"data"`
	}
	json.NewDecoder(rec.Body).Decode(&result)
	if result.Data.BackupID == 0 {
		t.Fatal("expected non-zero backup_id")
	}
	if result.Data.JobID == 0 {
		t.Fatal("expected non-zero job_id")
	}
	if result.Data.Status != "pending" {
		t.Fatalf("expected status pending, got %s", result.Data.Status)
	}
}

func TestBackupCreate_SiteNotFound_404(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)

	rec := doRequest(t, e, "POST", "/api/v1/sites/99999/backups", nil, token)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestBackupCreate_OtherUser_404(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	userToken := newUserToken(t, s, ctx)
	srvID := createServer(t, s, ctx)

	otherUser := &store.User{Username: "other", Email: "other@example.com", PasswordHash: "pass", Role: auth.RoleUser}
	s.Users.Create(ctx, otherUser)

	site := &store.Site{ServerID: srvID, Name: "Bak", Domain: "bak-other.example.com", Runtime: "static", OwnerID: nullInt(otherUser.ID)}
	s.Sites.Create(ctx, site)

	rec := doRequest(t, e, "POST", "/api/v1/sites/"+strconv.FormatInt(site.ID, 10)+"/backups", nil, userToken)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestBackupList_Success(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)
	srvID := createServer(t, s, ctx)

	adminUser, _ := s.Users.GetByUsername(ctx, "admin")
	site := &store.Site{ServerID: srvID, Name: "Bak", Domain: "bak-list.example.com", Runtime: "static", OwnerID: nullInt(adminUser.ID)}
	s.Sites.Create(ctx, site)

	s.Backups.Create(ctx, &store.Backup{SiteID: site.ID, Status: "pending"})
	s.Backups.Create(ctx, &store.Backup{SiteID: site.ID, Status: "completed"})

	rec := doRequest(t, e, "GET", "/api/v1/backups", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result struct {
		Data struct {
			Backups []map[string]any `json:"backups"`
			Count   int              `json:"count"`
		} `json:"data"`
	}
	json.NewDecoder(rec.Body).Decode(&result)
	if result.Data.Count != 2 {
		t.Fatalf("expected count 2, got %d", result.Data.Count)
	}
}

func TestBackupList_FilterBySite(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)
	srvID := createServer(t, s, ctx)

	adminUser, _ := s.Users.GetByUsername(ctx, "admin")
	site1 := &store.Site{ServerID: srvID, Name: "Bak1", Domain: "bak1.example.com", Runtime: "static", OwnerID: nullInt(adminUser.ID)}
	site2 := &store.Site{ServerID: srvID, Name: "Bak2", Domain: "bak2.example.com", Runtime: "static", OwnerID: nullInt(adminUser.ID)}
	s.Sites.Create(ctx, site1)
	s.Sites.Create(ctx, site2)

	s.Backups.Create(ctx, &store.Backup{SiteID: site1.ID, Status: "pending"})
	s.Backups.Create(ctx, &store.Backup{SiteID: site2.ID, Status: "completed"})

	rec := doRequest(t, e, "GET", "/api/v1/backups?site_id="+strconv.FormatInt(site1.ID, 10), nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result struct {
		Data struct {
			Backups []map[string]any `json:"backups"`
			Count   int              `json:"count"`
		} `json:"data"`
	}
	json.NewDecoder(rec.Body).Decode(&result)
	if result.Data.Count != 1 {
		t.Fatalf("expected count 1, got %d", result.Data.Count)
	}
	if result.Data.Backups[0]["site_id"] != float64(site1.ID) {
		t.Fatalf("unexpected site_id: %v", result.Data.Backups[0]["site_id"])
	}
}

func TestBackupRestore_Success(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	userToken := newUserToken(t, s, ctx)
	srvID := createServer(t, s, ctx)

	regularUser, _ := s.Users.GetByUsername(ctx, "regular")
	site := &store.Site{ServerID: srvID, Name: "Bak", Domain: "bak-restore.example.com", Runtime: "static", OwnerID: nullInt(regularUser.ID)}
	s.Sites.Create(ctx, site)

	backup := &store.Backup{SiteID: site.ID, Status: "completed", StoragePath: sql.NullString{String: "/path/b.tar.gz", Valid: true}}
	s.Backups.Create(ctx, backup)

	rec := doRequest(t, e, "POST", "/api/v1/backups/"+strconv.FormatInt(backup.ID, 10)+"/restore", nil, userToken)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var result struct {
		Data struct {
			BackupID int64 `json:"backup_id"`
			JobID    int64 `json:"job_id"`
		} `json:"data"`
	}
	json.NewDecoder(rec.Body).Decode(&result)
	if result.Data.BackupID != backup.ID {
		t.Fatalf("expected backup_id %d, got %d", backup.ID, result.Data.BackupID)
	}
	if result.Data.JobID == 0 {
		t.Fatal("expected non-zero job_id")
	}

	// Verify job type
	job, _ := s.Jobs.GetByID(ctx, result.Data.JobID)
	if job == nil || job.Type != jobmodel.TypeBackupRestore {
		t.Fatalf("expected restore job, got %v", job)
	}
}

func TestBackupRestore_NotCompleted_409(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	userToken := newUserToken(t, s, ctx)
	srvID := createServer(t, s, ctx)

	regularUser, _ := s.Users.GetByUsername(ctx, "regular")
	site := &store.Site{ServerID: srvID, Name: "Bak", Domain: "bak-restore-pending.example.com", Runtime: "static", OwnerID: nullInt(regularUser.ID)}
	s.Sites.Create(ctx, site)

	backup := &store.Backup{SiteID: site.ID, Status: "pending"}
	s.Backups.Create(ctx, backup)

	rec := doRequest(t, e, "POST", "/api/v1/backups/"+strconv.FormatInt(backup.ID, 10)+"/restore", nil, userToken)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
}

func TestBackupRestore_NotFound_404(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	userToken := newUserToken(t, s, ctx)

	rec := doRequest(t, e, "POST", "/api/v1/backups/99999/restore", nil, userToken)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestBackupRestore_InvalidID_400(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	userToken := newUserToken(t, s, ctx)

	rec := doRequest(t, e, "POST", "/api/v1/backups/abc/restore", nil, userToken)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestBackupList_RequiresAuth(t *testing.T) {
	e, _, _ := newTestAppWithAllMigrations(t)

	rec := doRequest(t, e, "GET", "/api/v1/backups", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestBackupRestore_RequiresAuth(t *testing.T) {
	e, _, _ := newTestAppWithAllMigrations(t)

	rec := doRequest(t, e, "POST", "/api/v1/backups/1/restore", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestBackupCreate_InvalidID_400(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)

	rec := doRequest(t, e, "POST", "/api/v1/sites/abc/backups", nil, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}
