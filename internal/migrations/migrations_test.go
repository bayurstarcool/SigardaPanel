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

// Migration 061 adds databases.managed_by ("sigardapanel" for existing rows)
// and is safe to run on a database that already has the column.
func TestDatabaseManagedByMigration(t *testing.T) {
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

	before := AllMigrations[:len(AllMigrations)-1]
	if err := Up(database, before); err != nil {
		t.Fatalf("Up() error = %v", err)
	}
	if _, err := database.Exec(`INSERT INTO servers (name, hostname, agent_token) VALUES ('s', 'h', 't')`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO sites (server_id, name, domain, runtime, root_path) VALUES (1, 'x', 'x.example.com', 'php', '/srv/x')`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO databases (site_id, name, type) VALUES (1, 'old', 'mysql')`); err != nil {
		t.Fatal(err)
	}
	if err := Up(database, AllMigrations); err != nil {
		t.Fatalf("Up() with 061 error = %v", err)
	}
	var managedBy string
	if err := database.QueryRow(`SELECT managed_by FROM databases WHERE name = 'old'`).Scan(&managedBy); err != nil || managedBy != "sigardapanel" {
		t.Fatalf("managed_by = %q, %v", managedBy, err)
	}
	// Re-running the migration itself (column present) is a no-op.
	if _, err := database.Exec(`DELETE FROM schema_migrations WHERE version = 61`); err != nil {
		t.Fatal(err)
	}
	if err := Up(database, AllMigrations); err != nil {
		t.Fatalf("re-run 061 error = %v", err)
	}
}
