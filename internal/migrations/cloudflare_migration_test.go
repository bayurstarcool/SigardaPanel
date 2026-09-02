package migrations

import (
	"os"
	"testing"

	"sigardapanel/internal/db"
)

func TestCloudflareConfigPerUserMigration_PreservesRowsAndAllowsMultipleUsers(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "sigardapanel-cloudflare-migration-*.db")
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

	if _, err := database.Exec(`
		CREATE TABLE schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE cloudflare_config (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			api_token TEXT NOT NULL DEFAULT '',
			email TEXT DEFAULT '',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			user_id INTEGER DEFAULT NULL
		);
		INSERT INTO cloudflare_config (id, api_token, email, user_id)
		VALUES (1, 'legacy-token', 'legacy@example.com', 3);
	`); err != nil {
		t.Fatalf("create legacy Cloudflare schema: %v", err)
	}

	if err := applyMigration(database, CloudflareConfigPerUserSchema); err != nil {
		t.Fatalf("apply Cloudflare migration: %v", err)
	}

	var token string
	if err := database.QueryRow("SELECT api_token FROM cloudflare_config WHERE user_id = 3").Scan(&token); err != nil {
		t.Fatalf("read preserved Cloudflare config: %v", err)
	}
	if token != "legacy-token" {
		t.Fatalf("preserved token = %q, want legacy-token", token)
	}

	if _, err := database.Exec(`
		INSERT INTO cloudflare_config (api_token, email, user_id)
		VALUES ('new-token', 'new@example.com', 1)
	`); err != nil {
		t.Fatalf("insert second user Cloudflare config: %v", err)
	}

	var count int
	if err := database.QueryRow("SELECT COUNT(*) FROM cloudflare_config").Scan(&count); err != nil {
		t.Fatalf("count Cloudflare configs: %v", err)
	}
	if count != 2 {
		t.Fatalf("Cloudflare config count = %d, want 2", count)
	}

	if _, err := database.Exec(`
		INSERT INTO cloudflare_config (api_token, email, user_id)
		VALUES ('duplicate-token', 'duplicate@example.com', 1)
	`); err == nil {
		t.Fatal("duplicate per-user Cloudflare config inserted; want unique constraint error")
	}
}
