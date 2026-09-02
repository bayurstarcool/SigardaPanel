package backendserver

import (
	"context"
	"os"
	"testing"

	"sigardapanel/internal/db"
	"sigardapanel/internal/migrations"
	"sigardapanel/internal/store"
)

func TestEnsureLocalServer_Idempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := dir + "/panel.db"
	t.Setenv("SIGARDAPANEL_DB_PATH", dbPath)
	t.Setenv("SIGARDAPANEL_AGENT_ADDR", "127.0.0.1:7710")
	t.Setenv("SIGARDAPANEL_AGENT_TOKEN", "local-test-token")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()
	if err := migrations.Up(database, migrations.AllMigrations); err != nil {
		t.Fatalf("migrate database: %v", err)
	}

	s := store.New(database)
	ctx := context.Background()
	if err := ensureLocalServer(ctx, s); err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	if err := ensureLocalServer(ctx, s); err != nil {
		t.Fatalf("second ensure: %v", err)
	}

	servers, err := s.Servers.List(ctx)
	if err != nil {
		t.Fatalf("list servers: %v", err)
	}
	if len(servers) != 1 {
		t.Fatalf("expected one local server, got %d", len(servers))
	}
	server := servers[0]
	if server.Name != "Local Server" || server.Status != "active" {
		t.Fatalf("unexpected local server: name=%q status=%q", server.Name, server.Status)
	}
	if !server.AgentURL.Valid || server.AgentURL.String != "http://127.0.0.1:7710" {
		t.Fatalf("unexpected agent URL: %#v", server.AgentURL)
	}
	if server.AgentToken != "local-test-token" {
		t.Fatalf("agent token not synchronized")
	}
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("database missing after registration: %v", err)
	}
}
