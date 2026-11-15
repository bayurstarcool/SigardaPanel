package backendserver

import (
	"context"
	"database/sql"
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
	prev := detectLocalPublicIP
	detectLocalPublicIP = func(context.Context) (string, error) { return "", os.ErrNotExist }
	t.Cleanup(func() { detectLocalPublicIP = prev })

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

func TestFixLocalServerIPUsesPublicAddress(t *testing.T) {
	database, err := db.Open(t.TempDir() + "/panel.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := migrations.Up(database, migrations.AllMigrations); err != nil {
		t.Fatal(err)
	}
	s := store.New(database)
	ctx := context.Background()
	prev := detectLocalPublicIP
	detectLocalPublicIP = func(context.Context) (string, error) { return "43.173.51.154", nil }
	t.Cleanup(func() { detectLocalPublicIP = prev })

	mk := func(ip string) int64 {
		srv := &store.Server{Name: "Local Server", Hostname: "h" + ip, Status: "active", AgentToken: "t",
			AgentURL: sql.NullString{String: "http://127.0.0.1:7710", Valid: true}, IPAddress: sql.NullString{String: ip, Valid: ip != ""}}
		if err := s.Servers.Create(ctx, srv); err != nil {
			t.Fatal(err)
		}
		return srv.ID
	}
	for ip, want := range map[string]string{"10.0.0.4": "43.173.51.154", "": "43.173.51.154", "8.8.8.8": "8.8.8.8"} {
		id := mk(ip)
		fixLocalServerIP(ctx, s, id)
		if got, _ := s.Servers.GetByID(ctx, id); got.IPAddress.String != want {
			t.Errorf("%q -> %q, want %q", ip, got.IPAddress.String, want)
		}
	}
}

func TestFixLocalServerIPPrefersPublicURL(t *testing.T) {
	database, err := db.Open(t.TempDir() + "/panel.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := migrations.Up(database, migrations.AllMigrations); err != nil {
		t.Fatal(err)
	}
	s := store.New(database)
	ctx := context.Background()
	prev := detectLocalPublicIP
	detectLocalPublicIP = func(context.Context) (string, error) { return "198.18.0.9", nil }
	t.Cleanup(func() { detectLocalPublicIP = prev })
	t.Setenv("SIGARDAPANEL_PUBLIC_URL", "https://43.173.51.154")
	srv := &store.Server{Name: "Local Server", Hostname: "h", Status: "active", AgentToken: "t",
		AgentURL: sql.NullString{String: "http://127.0.0.1:7710", Valid: true}, IPAddress: sql.NullString{String: "10.0.0.4", Valid: true}}
	s.Servers.Create(ctx, srv)
	fixLocalServerIP(ctx, s, srv.ID)
	if got, _ := s.Servers.GetByID(ctx, srv.ID); got.IPAddress.String != "43.173.51.154" {
		t.Fatalf("ip = %s", got.IPAddress.String)
	}
	t.Setenv("SIGARDAPANEL_PUBLIC_URL", "https://panel.example.com")
	if publicURLIPv4() != "" {
		t.Fatal("a domain was taken as the address")
	}
}
