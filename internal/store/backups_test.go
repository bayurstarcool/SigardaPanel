package store

import (
	"testing"
)

func testBackupDB(t *testing.T) *SQLiteBackupStore {
	db := testDB(t)
	// Disable FK enforcement for isolated backup store tests
	db.ExecContext(ctx, `PRAGMA foreign_keys = OFF`)
	return NewBackupStore(db)
}

func TestBackupCreate_Basic(t *testing.T) {
	s := testBackupDB(t)

	b := &Backup{SiteID: 1, Status: "pending", StorageType: "local"}
	if err := s.Create(ctx, b); err != nil {
		t.Fatalf("create backup: %v", err)
	}
	if b.ID == 0 {
		t.Fatal("backup ID not set after create")
	}
}

func TestBackupGetByID_Found(t *testing.T) {
	s := testBackupDB(t)

	b := &Backup{SiteID: 1, Status: "pending"}
	s.Create(ctx, b)

	got, err := s.GetByID(ctx, b.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if got == nil {
		t.Fatal("expected backup found")
	}
	if got.SiteID != 1 {
		t.Fatalf("expected site_id 1, got %d", got.SiteID)
	}
}

func TestBackupGetByID_NotFound(t *testing.T) {
	s := testBackupDB(t)

	got, err := s.GetByID(ctx, 99999)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != nil {
		t.Fatal("expected nil for nonexistent backup")
	}
}

func TestBackupListBySite(t *testing.T) {
	s := testBackupDB(t)

	b1 := &Backup{SiteID: 1, Status: "pending"}
	b2 := &Backup{SiteID: 1, Status: "completed"}
	b3 := &Backup{SiteID: 2, Status: "pending"}
	s.Create(ctx, b1)
	s.Create(ctx, b2)
	s.Create(ctx, b3)

	list, err := s.ListBySite(ctx, 1, 100, 0)
	if err != nil {
		t.Fatalf("list by site: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 backups, got %d", len(list))
	}
}

func TestBackupListAll(t *testing.T) {
	s := testBackupDB(t)

	s.Create(ctx, &Backup{SiteID: 1, Status: "pending"})
	s.Create(ctx, &Backup{SiteID: 2, Status: "completed"})

	list, err := s.ListAll(ctx, 100, 0)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 backups, got %d", len(list))
	}
}

func TestBackupMarkCompleted(t *testing.T) {
	db := testDB(t)
	db.ExecContext(ctx, `PRAGMA foreign_keys = OFF`)
	s := NewBackupStore(db)

	b := &Backup{SiteID: 1, Status: "pending"}
	s.Create(ctx, b)

	if err := s.MarkCompleted(ctx, b.ID, 2048, 10, "/var/backups/1/1.tar.gz"); err != nil {
		t.Fatalf("mark completed: %v", err)
	}

	got, err := s.GetByID(ctx, b.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if got.Status != "completed" {
		t.Fatalf("expected status completed, got %s", got.Status)
	}
	if got.SizeBytes != 2048 {
		t.Fatalf("expected size 2048, got %d", got.SizeBytes)
	}
	if got.FileCount != 10 {
		t.Fatalf("expected file_count 10, got %d", got.FileCount)
	}
	if got.StoragePath.String != "/var/backups/1/1.tar.gz" {
		t.Fatalf("unexpected storage_path: %v", got.StoragePath.String)
	}
}

func TestBackupMarkFailed(t *testing.T) {
	s := testBackupDB(t)

	b := &Backup{SiteID: 1, Status: "running"}
	s.Create(ctx, b)

	if err := s.MarkFailed(ctx, b.ID, "disk full"); err != nil {
		t.Fatalf("mark failed: %v", err)
	}

	got, err := s.GetByID(ctx, b.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if got.Status != "failed" {
		t.Fatalf("expected status failed, got %s", got.Status)
	}
	if !got.Error.Valid || got.Error.String != "disk full" {
		t.Fatalf("unexpected error: %v", got.Error)
	}
}

func TestBackupDelete(t *testing.T) {
	s := testBackupDB(t)

	b := &Backup{SiteID: 1, Status: "pending"}
	s.Create(ctx, b)

	if err := s.Delete(ctx, b.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	got, _ := s.GetByID(ctx, b.ID)
	if got != nil {
		t.Fatal("expected backup deleted")
	}
}

func TestBackupCleanupExpired(t *testing.T) {
	db := testDB(t)
	db.ExecContext(ctx, `PRAGMA foreign_keys = OFF`)
	s := NewBackupStore(db)

	// Create 4 completed backups for site 1, expire the oldest 2
	b1 := &Backup{SiteID: 1, Status: "completed"}
	s.Create(ctx, b1)
	db.ExecContext(ctx, `UPDATE backups SET expires_at = datetime('now', '-2 day'), completed_at = datetime('now', '-2 day') WHERE id = ?`, b1.ID)

	b2 := &Backup{SiteID: 1, Status: "completed"}
	s.Create(ctx, b2)
	db.ExecContext(ctx, `UPDATE backups SET expires_at = datetime('now', '-1 day'), completed_at = datetime('now', '-1 day') WHERE id = ?`, b2.ID)

	b3 := &Backup{SiteID: 1, Status: "completed"}
	s.Create(ctx, b3)
	db.ExecContext(ctx, `UPDATE backups SET completed_at = datetime('now') WHERE id = ?`, b3.ID)

	b4 := &Backup{SiteID: 1, Status: "completed"}
	s.Create(ctx, b4)
	db.ExecContext(ctx, `UPDATE backups SET completed_at = datetime('now') WHERE id = ?`, b4.ID)

	// keepLast=3 means keep at least 3 completed per site; only the oldest expired (b1) gets deleted
	deleted, err := s.CleanupExpired(ctx, 3)
	if err != nil {
		t.Fatalf("cleanup expired: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deleted, got %d", deleted)
	}

	got, _ := s.GetByID(ctx, b1.ID)
	if got != nil {
		t.Fatal("expected oldest expired backup deleted")
	}

	got2, _ := s.GetByID(ctx, b2.ID)
	if got2 == nil {
		t.Fatal("expected expired backup retained (within keepLast=3)")
	}

	got3, _ := s.GetByID(ctx, b3.ID)
	if got3 == nil {
		t.Fatal("expected no-expiry backup retained")
	}

	got4, _ := s.GetByID(ctx, b4.ID)
	if got4 == nil {
		t.Fatal("expected fourth backup retained")
	}
}

func TestBackupDeleteOldestKeepsNewestPerType(t *testing.T) {
	s := testBackupDB(t)

	var databaseBackups []*Backup
	for i := 0; i < 5; i++ {
		backup := &Backup{SiteID: 1, Status: "completed", BackupType: "database"}
		if err := s.Create(ctx, backup); err != nil {
			t.Fatalf("create database backup %d: %v", i, err)
		}
		databaseBackups = append(databaseBackups, backup)
	}
	fileBackup := &Backup{SiteID: 1, Status: "completed", BackupType: "files"}
	if err := s.Create(ctx, fileBackup); err != nil {
		t.Fatalf("create files backup: %v", err)
	}

	deleted, err := s.DeleteOldest(ctx, 1, 3)
	if err != nil {
		t.Fatalf("delete oldest: %v", err)
	}
	if len(deleted) != 2 {
		t.Fatalf("expected 2 deleted database backups, got %d", len(deleted))
	}

	for i, backup := range databaseBackups {
		got, err := s.GetByID(ctx, backup.ID)
		if err != nil {
			t.Fatalf("get database backup %d: %v", i, err)
		}
		if i < 2 && got != nil {
			t.Fatalf("expected oldest database backup %d deleted", backup.ID)
		}
		if i >= 2 && got == nil {
			t.Fatalf("expected newest database backup %d retained", backup.ID)
		}
	}

	gotFile, err := s.GetByID(ctx, fileBackup.ID)
	if err != nil {
		t.Fatalf("get files backup: %v", err)
	}
	if gotFile == nil {
		t.Fatal("expected files backup retained")
	}
}

func TestBackupUpdateStatus(t *testing.T) {
	s := testBackupDB(t)

	b := &Backup{SiteID: 1, Status: "pending"}
	s.Create(ctx, b)

	if err := s.UpdateStatus(ctx, b.ID, "running"); err != nil {
		t.Fatalf("update status: %v", err)
	}

	got, err := s.GetByID(ctx, b.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if got.Status != "running" {
		t.Fatalf("expected status running, got %s", got.Status)
	}
}

func TestBackupListPagination(t *testing.T) {
	s := testBackupDB(t)

	for i := 0; i < 5; i++ {
		s.Create(ctx, &Backup{SiteID: 1, Status: "completed"})
	}

	list, err := s.ListAll(ctx, 2, 0)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 backups, got %d", len(list))
	}

	list2, err := s.ListAll(ctx, 2, 2)
	if err != nil {
		t.Fatalf("list all offset: %v", err)
	}
	if len(list2) != 2 {
		t.Fatalf("expected 2 backups offset, got %d", len(list2))
	}
}

func TestBackupDefaultValues(t *testing.T) {
	s := testBackupDB(t)

	b := &Backup{SiteID: 1}
	if err := s.Create(ctx, b); err != nil {
		t.Fatalf("create backup: %v", err)
	}
	if b.Status != "pending" {
		t.Fatalf("expected default status pending, got %s", b.Status)
	}
	if b.StorageType != "local" {
		t.Fatalf("expected default storage_type local, got %s", b.StorageType)
	}
}
