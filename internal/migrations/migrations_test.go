package migrations

import (
	"os"
	"testing"

	"sigardapanel/internal/db"
)

func TestUp_CreatesTables(t *testing.T) {
	// Create a temporary database file
	tmpFile, err := os.CreateTemp("", "sigardapanel-migrations-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)
	defer os.Remove(tmpPath + "-wal")
	defer os.Remove(tmpPath + "-shm")

	database, err := db.Open(tmpPath)
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	defer db.Close(database)

	// Run migrations
	err = Up(database, []Migration{InitialSchema})
	if err != nil {
		t.Fatalf("Up() error = %v", err)
	}

	// Verify tables exist
	tables := []string{"users", "servers", "sites", "jobs", "audit_logs", "schema_migrations"}
	for _, table := range tables {
		var count int
		err := database.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&count)
		if err != nil {
			t.Errorf("failed to check table %s: %v", table, err)
		}
		if count != 1 {
			t.Errorf("table %s does not exist", table)
		}
	}

	// Verify indexes exist
	indexes := []string{"idx_sites_server_id", "idx_sites_domain", "idx_jobs_status", "idx_jobs_server_id", "idx_audit_logs_user_id", "idx_audit_logs_created_at"}
	for _, idx := range indexes {
		var count int
		err := database.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?", idx).Scan(&count)
		if err != nil {
			t.Errorf("failed to check index %s: %v", idx, err)
		}
		if count != 1 {
			t.Errorf("index %s does not exist", idx)
		}
	}
}

func TestUp_Idempotent(t *testing.T) {
	// Create a temporary database file
	tmpFile, err := os.CreateTemp("", "sigardapanel-migrations-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)
	defer os.Remove(tmpPath + "-wal")
	defer os.Remove(tmpPath + "-shm")

	database, err := db.Open(tmpPath)
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	defer db.Close(database)

	// Run migrations first time
	err = Up(database, []Migration{InitialSchema})
	if err != nil {
		t.Fatalf("Up() first run error = %v", err)
	}

	// Run migrations second time - should not error
	err = Up(database, []Migration{InitialSchema})
	if err != nil {
		t.Errorf("Up() second run error = %v, want nil (idempotent)", err)
	}
}
