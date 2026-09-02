package store

import (
	"context"
	"testing"

	"sigardapanel/internal/audit"
)

func TestSQLiteAuditLogStore_List_Basic(t *testing.T) {
	db := testDB(t)
	store := NewAuditLogStore(db)
	ctx := context.Background()

	// Insert 3 entries directly (user_id NULL to avoid FK issues)
	for i := 1; i <= 3; i++ {
		_, err := db.ExecContext(ctx,
			`INSERT INTO audit_logs (action, resource, resource_id, details) VALUES (?, ?, ?, ?)`,
			audit.ActionLogin, audit.ResourceSession, int64(i), "",
		)
		if err != nil {
			t.Fatalf("insert audit log %d: %v", i, err)
		}
	}

	entries, err := store.List(ctx, audit.AuditLogFilter{Limit: 10})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	// Should be in DESC order by id
	if entries[0].ID < entries[1].ID || entries[1].ID < entries[2].ID {
		t.Fatal("expected descending order by id")
	}
}

func TestSQLiteAuditLogStore_List_FilterUserID(t *testing.T) {
	db := testDB(t)
	users := NewUserStore(db)
	store := NewAuditLogStore(db)
	ctx := context.Background()

	u1 := &User{Username: "alice", Email: "alice@example.com", PasswordHash: "secret", Role: "user"}
	u2 := &User{Username: "bob", Email: "bob@example.com", PasswordHash: "secret", Role: "user"}
	users.Create(ctx, u1)
	users.Create(ctx, u2)

	for i := 1; i <= 3; i++ {
		uid := u1.ID
		if i == 2 {
			uid = u2.ID
		}
		if i == 3 {
			uid = u2.ID
		}
		_, err := db.ExecContext(ctx,
			`INSERT INTO audit_logs (user_id, action, resource, details) VALUES (?, ?, ?, ?)`,
			uid, audit.ActionLogin, audit.ResourceSession, "",
		)
		if err != nil {
			t.Fatalf("insert audit log %d: %v", i, err)
		}
	}

	entries, err := store.List(ctx, audit.AuditLogFilter{UserID: u2.ID, Limit: 10})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].UserID.Int64 != u2.ID {
		t.Fatalf("expected user_id %d, got %d", u2.ID, entries[0].UserID.Int64)
	}
}

func TestSQLiteAuditLogStore_List_FilterResourceAction(t *testing.T) {
	db := testDB(t)
	store := NewAuditLogStore(db)
	ctx := context.Background()

	_, _ = db.ExecContext(ctx,
		`INSERT INTO audit_logs (action, resource, details) VALUES (?, ?, ?)`,
		audit.ActionLogin, audit.ResourceSession, "")
	_, _ = db.ExecContext(ctx,
		`INSERT INTO audit_logs (action, resource, details) VALUES (?, ?, ?)`,
		audit.ActionServerCreate, audit.ResourceServer, "")
	_, _ = db.ExecContext(ctx,
		`INSERT INTO audit_logs (action, resource, details) VALUES (?, ?, ?)`,
		audit.ActionLogout, audit.ResourceSession, "")

	entries, err := store.List(ctx, audit.AuditLogFilter{
		Resource: audit.ResourceSession,
		Action:   audit.ActionLogin,
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Action != audit.ActionLogin || entries[0].Resource != audit.ResourceSession {
		t.Fatalf("unexpected filter result: %+v", entries[0])
	}
}

func TestSQLiteAuditLogStore_List_Limit(t *testing.T) {
	db := testDB(t)
	store := NewAuditLogStore(db)
	ctx := context.Background()

	for i := 1; i <= 5; i++ {
		_, err := db.ExecContext(ctx,
			`INSERT INTO audit_logs (action, resource, details) VALUES (?, ?, ?)`,
			audit.ActionLogin, audit.ResourceSession, "")
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	entries, err := store.List(ctx, audit.AuditLogFilter{Limit: 2})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
}

func TestSQLiteAuditLogStore_List_Offset(t *testing.T) {
	db := testDB(t)
	store := NewAuditLogStore(db)
	ctx := context.Background()

	for i := 1; i <= 5; i++ {
		_, err := db.ExecContext(ctx,
			`INSERT INTO audit_logs (action, resource, details) VALUES (?, ?, ?)`,
			audit.ActionLogin, audit.ResourceSession, "")
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	entries, err := store.List(ctx, audit.AuditLogFilter{Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	all, _ := store.List(ctx, audit.AuditLogFilter{Limit: 10})
	if entries[0].ID != all[2].ID {
		t.Fatalf("offset did not skip correctly")
	}
}
