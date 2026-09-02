package audit

import (
	"bytes"
	"context"
	"database/sql"
	"log/slog"
	"os"
	"testing"

	"sigardapanel/internal/db"
	"sigardapanel/internal/migrations"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	f, _ := os.CreateTemp("", "audit_test_*.db")
	t.Cleanup(func() { os.Remove(f.Name()) })
	database, err := db.Open(f.Name())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
			if err := migrations.Up(database, migrations.AllMigrations); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return database
}

func countAuditRows(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_logs`).Scan(&n); err != nil {
		t.Fatalf("count audit rows: %v", err)
	}
	return n
}

func TestRecord_Success(t *testing.T) {
	database := testDB(t)
	rec := NewRecorder(database)
	ctx := context.Background()

	rec.Record(ctx, Entry{
		Action:   ActionLogin,
		Resource: ResourceSession,
	})

	if got := countAuditRows(t, database); got != 1 {
		t.Fatalf("expected 1 audit row, got %d", got)
	}
}

func TestRecord_WithoutUser(t *testing.T) {
	database := testDB(t)
	rec := NewRecorder(database)
	ctx := context.Background()

	rec.Record(ctx, Entry{
		Action:   ActionLogin,
		Resource: ResourceUser,
	})

	var userID sql.NullInt64
	if err := database.QueryRow(`SELECT user_id FROM audit_logs LIMIT 1`).Scan(&userID); err != nil {
		t.Fatalf("scan user_id: %v", err)
	}
	if userID.Valid {
		t.Fatalf("expected user_id NULL, got %v", userID.Int64)
	}
}

func TestRecord_WithDetails(t *testing.T) {
	database := testDB(t)
	rec := NewRecorder(database)
	ctx := context.Background()

	rec.Record(ctx, Entry{
		Action:   ActionServerCreate,
		Resource: ResourceServer,
		Details:  sql.NullString{String: `{"hostname":"srv1.example.com"}`, Valid: true},
	})

	var details string
	if err := database.QueryRow(`SELECT details FROM audit_logs LIMIT 1`).Scan(&details); err != nil {
		t.Fatalf("scan details: %v", err)
	}
	if details != `{"hostname":"srv1.example.com"}` {
		t.Fatalf("unexpected details: %s", details)
	}
}

func TestRecord_DatabaseClosed(t *testing.T) {
	database := testDB(t)
	rec := NewRecorder(database)
	ctx := context.Background()

	// Capture slog output
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError})
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(handler))
	defer slog.SetDefault(oldLogger)

	database.Close()

	// Must not panic
	rec.Record(ctx, Entry{
		Action:   ActionLogin,
		Resource: ResourceSession,
	})

	if !bytes.Contains(buf.Bytes(), []byte("audit: insert failed")) {
		t.Fatal("expected error log for closed database")
	}
}

func TestRecordAction_MarshalsDetails(t *testing.T) {
	database := testDB(t)
	rec := NewRecorder(database)
	ctx := context.Background()

	rec.RecordAction(ctx, 0, ActionSiteCreate, ResourceSite, 10, map[string]any{"domain": "example.com"})

	var details string
	if err := database.QueryRow(`SELECT details FROM audit_logs LIMIT 1`).Scan(&details); err != nil {
		t.Fatalf("scan details: %v", err)
	}
	if details == "" {
		t.Fatal("expected non-empty JSON details")
	}
}

func TestRecordAction_NilDetails(t *testing.T) {
	database := testDB(t)
	rec := NewRecorder(database)
	ctx := context.Background()

	rec.RecordAction(ctx, 0, ActionLogin, ResourceSession, 0, nil)

	var details sql.NullString
	if err := database.QueryRow(`SELECT details FROM audit_logs LIMIT 1`).Scan(&details); err != nil {
		t.Fatalf("scan details: %v", err)
	}
	if details.Valid {
		t.Fatalf("expected NULL details, got %s", details.String)
	}
}

func TestRecordAction_ZeroIDs(t *testing.T) {
	database := testDB(t)
	rec := NewRecorder(database)
	ctx := context.Background()

	rec.RecordAction(ctx, 0, ActionLogin, ResourceSession, 0, nil)

	var userID, resourceID sql.NullInt64
	if err := database.QueryRow(`SELECT user_id, resource_id FROM audit_logs LIMIT 1`).Scan(&userID, &resourceID); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if userID.Valid {
		t.Fatalf("expected user_id NULL")
	}
	if resourceID.Valid {
		t.Fatalf("expected resource_id NULL")
	}
}
