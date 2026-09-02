package store

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"sigardapanel/internal/db"
	"sigardapanel/internal/migrations"
)

func testDB(t *testing.T) *sql.DB {
	f, _ := os.CreateTemp("", "store_test_*.db")
	f.Close()
	database, err := db.Open(f.Name())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := migrations.Up(database, migrations.AllMigrations); err != nil {
		database.Close()
		os.Remove(f.Name())
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() {
		database.Close()
		os.Remove(f.Name())
	})
	return database
}

var ctx = context.Background()

func TestSQLiteUserStore_CreateAndGet(t *testing.T) {
	db := testDB(t)
	s := NewUserStore(db)

	u := &User{Username: "alice", Email: "alice@example.com", PasswordHash: "secret", Role: "user"}
	if err := s.Create(ctx, u); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if u.ID == 0 {
		t.Fatal("user ID not set after create")
	}

	got, err := s.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if got == nil {
		t.Fatal("user not found")
	}
	if got.Username != "alice" {
		t.Fatalf("expected username alice, got %s", got.Username)
	}

	got2, err := s.GetByUsername(ctx, "alice")
	if err != nil {
		t.Fatalf("get by username: %v", err)
	}
	if got2.ID != u.ID {
		t.Fatal("mismatch between get by id and get by username")
	}
}

func TestSQLiteUserStore_GetByUsername_NotFound(t *testing.T) {
	db := testDB(t)
	s := NewUserStore(db)

	got, err := s.GetByUsername(ctx, "nobody")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil for nonexistent user")
	}
}

func TestSQLiteSessionStore_CreateGetRevoke(t *testing.T) {
	db := testDB(t)
	users := NewUserStore(db)
	sessions := NewSessionStore(db)

	u := &User{Username: "bob", Email: "bob@example.com", PasswordHash: "pass", Role: "user"}
	users.Create(ctx, u)

	sess := &Session{
		UserID:    u.ID,
		TokenHash: "abc123def456",
	}
	if err := sessions.Create(ctx, sess); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if sess.ID == 0 {
		t.Fatal("session ID not set")
	}

	got, err := sessions.GetByTokenHash(ctx, "abc123def456")
	if err != nil {
		t.Fatalf("get by token hash: %v", err)
	}
	if got == nil {
		t.Fatal("session not found")
	}
	if got.UserID != u.ID {
		t.Fatalf("expected user_id %d, got %d", u.ID, got.UserID)
	}

	if err := sessions.Revoke(ctx, sess.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	got2, err := sessions.GetByTokenHash(ctx, "abc123def456")
	if err != nil {
		t.Fatalf("get after revoke: %v", err)
	}
	if got2 != nil {
		t.Fatal("expected nil session after revoke")
	}
}

func TestSQLiteSessionStore_GetByTokenHash_NotFound(t *testing.T) {
	db := testDB(t)
	s := NewSessionStore(db)

	got, err := s.GetByTokenHash(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil for nonexistent token hash")
	}
}

func TestCreateUser_InvalidRole(t *testing.T) {
	db := testDB(t)
	s := NewUserStore(db)

	u := &User{Username: "badactor", Email: "bad@example.com", PasswordHash: "secret", Role: "superuser"}
	err := s.Create(ctx, u)
	if err == nil {
		t.Fatal("expected error for invalid role, got nil")
	}

	// Verify user was NOT inserted
	count, _ := s.GetByID(ctx, u.ID)
	if count != nil {
		t.Fatal("user with invalid role should not be inserted")
	}

	// Verify we can still create a valid user with same username after failed insert
	validUser := &User{Username: "badactor", Email: "valid@example.com", PasswordHash: "secret", Role: "user"}
	if err := s.Create(ctx, validUser); err != nil {
		t.Fatalf("create valid user after failed insert: %v", err)
	}
	if validUser.ID == 0 {
		t.Fatal("valid user ID not set")
	}
}

func TestStore_Combined(t *testing.T) {
	db := testDB(t)
	store := New(db)

	u := &User{Username: "carol", Email: "carol@example.com", PasswordHash: "secret123", Role: "admin"}
	if err := store.Users.Create(ctx, u); err != nil {
		t.Fatalf("create user: %v", err)
	}

	sess := &Session{UserID: u.ID, TokenHash: "session-hash-abc"}
	if err := store.Sessions.Create(ctx, sess); err != nil {
		t.Fatalf("create session: %v", err)
	}

	got, err := store.Sessions.GetByTokenHash(ctx, "session-hash-abc")
	if err != nil || got == nil || got.UserID != u.ID {
		t.Fatal("store sessions lookup failed")
	}
}
