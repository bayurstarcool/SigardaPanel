package store

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"sigardapanel/internal/db"
	"sigardapanel/internal/migrations"
)

func TestConsumeRegistrationTokenAndCreateServerIsAtomic(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := migrations.Up(database, migrations.AllMigrations); err != nil {
		t.Fatal(err)
	}

	s := New(database)
	rt := &RegistrationToken{TokenHash: "atomic-token", ExpiresAt: time.Now().Add(time.Hour)}
	if err := s.RegistrationTokens.Create(context.Background(), rt); err != nil {
		t.Fatal(err)
	}

	first := &Server{Name: "one", Hostname: "one", Status: "online", AgentToken: "token-one", AgentURL: sql.NullString{String: "http://127.0.0.1:7710", Valid: true}, SSHPort: 22}
	if err := s.ConsumeRegistrationTokenAndCreateServer(context.Background(), rt.ID, first); err != nil {
		t.Fatalf("first registration: %v", err)
	}

	second := &Server{Name: "two", Hostname: "two", Status: "online", AgentToken: "token-two", AgentURL: sql.NullString{String: "http://127.0.0.2:7710", Valid: true}, SSHPort: 22}
	if err := s.ConsumeRegistrationTokenAndCreateServer(context.Background(), rt.ID, second); err == nil {
		t.Fatal("second registration unexpectedly reused the one-time token")
	}

	servers, err := s.Servers.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 1 || servers[0].Hostname != "one" {
		t.Fatalf("expected exactly first server, got %#v", servers)
	}
}

func TestConsumeRegistrationTokenRollsBackWhenServerInsertFails(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := migrations.Up(database, migrations.AllMigrations); err != nil {
		t.Fatal(err)
	}

	s := New(database)
	rt := &RegistrationToken{TokenHash: "rollback-token", ExpiresAt: time.Now().Add(time.Hour)}
	if err := s.RegistrationTokens.Create(context.Background(), rt); err != nil {
		t.Fatal(err)
	}

	if _, err := database.Exec(`CREATE TRIGGER fail_registered_server BEFORE INSERT ON servers BEGIN SELECT RAISE(ABORT, 'forced insert failure'); END`); err != nil {
		t.Fatal(err)
	}
	valid := &Server{Name: "rollback", Hostname: "rollback", Status: "online", AgentToken: "agent-token", AgentURL: sql.NullString{String: "http://8.8.8.8:7710", Valid: true}, SSHPort: 22}
	if err := s.ConsumeRegistrationTokenAndCreateServer(context.Background(), rt.ID, valid); err == nil {
		t.Fatal("expected forced SQL insert failure")
	}

	stored, err := s.RegistrationTokens.GetByHash(context.Background(), rt.TokenHash)
	if err != nil {
		t.Fatal(err)
	}
	if stored.UsedAt.Valid {
		t.Fatal("token was consumed even though server creation failed")
	}
}

func TestReRegistrationRebindsTheExistingServer(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := migrations.Up(database, migrations.AllMigrations); err != nil {
		t.Fatal(err)
	}
	s := New(database)
	ctx := context.Background()
	url := sql.NullString{String: "http://203.0.113.5:7710", Valid: true}
	register := func(hash, token string) *Server {
		rt := &RegistrationToken{TokenHash: hash, ExpiresAt: time.Now().Add(time.Hour)}
		if err := s.RegistrationTokens.Create(ctx, rt); err != nil {
			t.Fatal(err)
		}
		srv := &Server{Name: "web1", Hostname: "web1", Status: "online", AgentToken: token, AgentURL: url, SSHPort: 22}
		if err := s.ConsumeRegistrationTokenAndCreateServer(ctx, rt.ID, srv); err != nil {
			t.Fatal(err)
		}
		return srv
	}
	first := register("t1", "token-one")
	second := register("t2", "token-two")
	if second.ID != first.ID || !second.Rebound || first.Rebound {
		t.Fatalf("first=%d second=%d rebound=%v", first.ID, second.ID, second.Rebound)
	}
	servers, _ := s.Servers.List(ctx)
	if len(servers) != 1 || servers[0].AgentToken != "token-two" {
		t.Fatalf("servers = %+v", servers)
	}
}
