package store

import (
	"database/sql"
	"testing"
)

func TestServerCreate_Basic(t *testing.T) {
	db := testDB(t)
	s := NewServerStore(db)

	server := &Server{
		Name:       "prod-01",
		Hostname:   "prod-01.example.com",
		IPAddress:  sql.NullString{String: "192.168.1.1", Valid: true},
		Status:     "active",
		AgentToken: "token123",
	}
	if err := s.Create(ctx, server); err != nil {
		t.Fatalf("create server: %v", err)
	}
	if server.ID == 0 {
		t.Fatal("server ID not set after create")
	}
}

func TestServerCreate_DefaultStatus(t *testing.T) {
	db := testDB(t)
	s := NewServerStore(db)

	server := &Server{
		Name:       "test-server",
		Hostname:   "test.example.com",
		AgentToken: "token456",
	}
	if err := s.Create(ctx, server); err != nil {
		t.Fatalf("create server: %v", err)
	}
	if server.Status != "pending" {
		t.Fatalf("expected status pending, got %s", server.Status)
	}
}

func TestServerGetByID_Found(t *testing.T) {
	db := testDB(t)
	s := NewServerStore(db)

	server := &Server{
		Name:       "db-server",
		Hostname:   "db.example.com",
		IPAddress:  sql.NullString{String: "10.0.0.1", Valid: true},
		Status:     "active",
		AgentToken: "token789",
	}
	if err := s.Create(ctx, server); err != nil {
		t.Fatalf("create server: %v", err)
	}

	got, err := s.GetByID(ctx, server.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if got == nil {
		t.Fatal("server not found")
	}
	if got.Name != server.Name {
		t.Fatalf("expected name %s, got %s", server.Name, got.Name)
	}
	if got.Hostname != server.Hostname {
		t.Fatalf("expected hostname %s, got %s", server.Hostname, got.Hostname)
	}
	if got.IPAddress.String != server.IPAddress.String {
		t.Fatalf("expected ip %s, got %s", server.IPAddress.String, got.IPAddress.String)
	}
	if got.Status != server.Status {
		t.Fatalf("expected status %s, got %s", server.Status, got.Status)
	}
	if got.AgentToken != server.AgentToken {
		t.Fatalf("expected agent_token %s, got %s", server.AgentToken, got.AgentToken)
	}
}

func TestServerGetByID_NotFound(t *testing.T) {
	db := testDB(t)
	s := NewServerStore(db)

	got, err := s.GetByID(ctx, 99999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil for nonexistent server")
	}
}

func TestServerList_Empty(t *testing.T) {
	db := testDB(t)
	s := NewServerStore(db)

	servers, err := s.List(ctx)
	if err != nil {
		t.Fatalf("list servers: %v", err)
	}
	if len(servers) != 0 {
		t.Fatalf("expected empty list, got %d servers", len(servers))
	}
}

func TestServerList_Multiple(t *testing.T) {
	db := testDB(t)
	s := NewServerStore(db)

	names := []string{"server-a", "server-b", "server-c"}
	hostnames := []string{"host-a.example.com", "host-b.example.com", "host-c.example.com"}
	for i := 0; i < 3; i++ {
		server := &Server{
			Name:       names[i],
			Hostname:   hostnames[i],
			Status:     "active",
			AgentToken: "token-" + names[i],
		}
		if err := s.Create(ctx, server); err != nil {
			t.Fatalf("create server %d: %v", i, err)
		}
	}

	servers, err := s.List(ctx)
	if err != nil {
		t.Fatalf("list servers: %v", err)
	}
	if len(servers) != 3 {
		t.Fatalf("expected 3 servers, got %d", len(servers))
	}
	// Verify ordered by id ASC
	for i := 1; i < len(servers); i++ {
		if servers[i].ID <= servers[i-1].ID {
			t.Fatal("servers not ordered by id ASC")
		}
	}
}

func TestServerUpdate_Basic(t *testing.T) {
	db := testDB(t)
	s := NewServerStore(db)

	server := &Server{
		Name:       "old-name",
		Hostname:   "old.example.com",
		Status:     "pending",
		AgentToken: "token",
	}
	if err := s.Create(ctx, server); err != nil {
		t.Fatalf("create server: %v", err)
	}

	server.Name = "new-name"
	if err := s.Update(ctx, server); err != nil {
		t.Fatalf("update server: %v", err)
	}

	got, err := s.GetByID(ctx, server.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if got.Name != "new-name" {
		t.Fatalf("expected name new-name, got %s", got.Name)
	}
}

func TestServerUpdateStatus(t *testing.T) {
	db := testDB(t)
	s := NewServerStore(db)

	server := &Server{
		Name:       "status-test",
		Hostname:   "status.example.com",
		Status:     "pending",
		AgentToken: "token",
	}
	if err := s.Create(ctx, server); err != nil {
		t.Fatalf("create server: %v", err)
	}

	if err := s.UpdateStatus(ctx, server.ID, "active"); err != nil {
		t.Fatalf("update status: %v", err)
	}

	got, err := s.GetByID(ctx, server.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if got.Status != "active" {
		t.Fatalf("expected status active, got %s", got.Status)
	}
}

func TestServerDelete_Success(t *testing.T) {
	db := testDB(t)
	s := NewServerStore(db)

	server := &Server{
		Name:       "delete-me",
		Hostname:   "delete.example.com",
		Status:     "pending",
		AgentToken: "token",
	}
	if err := s.Create(ctx, server); err != nil {
		t.Fatalf("create server: %v", err)
	}

	if err := s.Delete(ctx, server.ID); err != nil {
		t.Fatalf("delete server: %v", err)
	}

	got, err := s.GetByID(ctx, server.ID)
	if err != nil {
		t.Fatalf("get by id after delete: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil after delete")
	}
}

func TestServerCreate_WithNilIP(t *testing.T) {
	db := testDB(t)
	s := NewServerStore(db)

	server := &Server{
		Name:       "no-ip",
		Hostname:   "noip.example.com",
		Status:     "pending",
		AgentToken: "token",
		IPAddress:  sql.NullString{Valid: false},
	}
	if err := s.Create(ctx, server); err != nil {
		t.Fatalf("create server: %v", err)
	}

	got, err := s.GetByID(ctx, server.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if got.IPAddress.Valid {
		t.Fatal("expected ip_address to be NULL (Valid=false)")
	}
}
