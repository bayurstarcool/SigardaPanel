package store

import (
	"database/sql"
	"errors"
	"testing"
	"time"
)

// Each database of a site gets its own backup record; previously the second
// insert was "skipped" and handed back the first record's id.
func TestBackupCreate_DatabaseBackupsGetDistinctIDs(t *testing.T) {
	s := testBackupDB(t)
	a := &Backup{SiteID: 1, Status: "pending", BackupType: "database"}
	b := &Backup{SiteID: 1, Status: "pending", BackupType: "database"}
	if err := s.Create(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := s.Create(ctx, b); err != nil {
		t.Fatal(err)
	}
	if a.ID == 0 || b.ID == 0 || a.ID == b.ID {
		t.Fatalf("expected two distinct ids, got %d and %d", a.ID, b.ID)
	}
}

func TestBackupCreate_FileBackupStillDeduplicated(t *testing.T) {
	s := testBackupDB(t)
	if err := s.Create(ctx, &Backup{SiteID: 7, Status: "pending"}); err != nil {
		t.Fatal(err)
	}
	// Touch another table so the connection's last rowid is non-zero and stale.
	if _, err := s.db.ExecContext(ctx, `INSERT INTO alert_channels (type, config) VALUES ('webhook', '{}')`); err != nil {
		t.Logf("seed other table: %v (continuing)", err)
	}
	dup := &Backup{SiteID: 7, Status: "pending"}
	if err := s.Create(ctx, dup); !errors.Is(err, ErrDuplicateBackup) {
		t.Fatalf("expected ErrDuplicateBackup, got %v (id %d)", err, dup.ID)
	}
	if dup.ID != 0 {
		t.Fatalf("duplicate must not receive an id, got %d", dup.ID)
	}
}

func TestBackupCreate_PersistsExpiresAt(t *testing.T) {
	s := testBackupDB(t)
	exp := time.Now().Add(7 * 24 * time.Hour).UTC().Truncate(time.Second)
	b := &Backup{SiteID: 1, Status: "pending", ExpiresAt: sql.NullTime{Time: exp, Valid: true}}
	if err := s.Create(ctx, b); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetByID(ctx, b.ID)
	if err != nil || got == nil {
		t.Fatalf("get: %v", err)
	}
	if !got.ExpiresAt.Valid || !got.ExpiresAt.Time.UTC().Equal(exp) {
		t.Fatalf("expires_at not persisted: %+v", got.ExpiresAt)
	}
}

// A backup that waited in the queue must not be failed as "stuck" the moment
// it starts running.
func TestBackupCleanupStuckRunning_UsesStartTime(t *testing.T) {
	s := testBackupDB(t)
	b := &Backup{SiteID: 1, Status: "pending"}
	if err := s.Create(ctx, b); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE backups SET created_at = datetime('now','-2 hours') WHERE id = ?`, b.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateStatus(ctx, b.ID, "running"); err != nil {
		t.Fatal(err)
	}
	n, err := s.CleanupStuckRunning(ctx, 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("freshly started backup was marked stuck")
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE backups SET started_at = datetime('now','-2 hours') WHERE id = ?`, b.ID); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.CleanupStuckRunning(ctx, 30*time.Minute); n != 1 {
		t.Fatalf("backup running for 2h should be marked stuck, affected %d", n)
	}
}

// Pending backups whose job is still queued are not orphans.
func TestBackupCleanupOrphanedPending_KeepsQueued(t *testing.T) {
	s := testBackupDB(t)
	queued := &Backup{SiteID: 1, Status: "pending", BackupType: "database"}
	orphan := &Backup{SiteID: 2, Status: "pending", BackupType: "database"}
	for _, b := range []*Backup{queued, orphan} {
		if err := s.Create(ctx, b); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE backups SET created_at = datetime('now','-3 hours')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO jobs (type, status, payload) VALUES ('db_backup.create', 'queued', json_object('backup_id', ?))`, queued.ID); err != nil {
		t.Fatal(err)
	}
	n, err := s.CleanupOrphanedPending(ctx, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected only the orphan to be removed, removed %d", n)
	}
	if got, _ := s.GetByID(ctx, queued.ID); got == nil {
		t.Fatal("pending backup with a queued job was deleted")
	}
}
