package store

import (
	"context"
	"database/sql"
	"testing"

	"sigardapanel/internal/db"
	"sigardapanel/internal/migrations"
)

func TestDeleteOldestKeepsInFlightAndCountsOnlyCompleted(t *testing.T) {
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
	srv := &Server{Name: "s", Hostname: "s", Status: "online", AgentToken: "t", SSHPort: 22}
	s.Servers.Create(ctx, srv)
	site := &Site{ServerID: srv.ID, Name: "n", Domain: "ret.example.com", Runtime: "static", RootPath: "/home/u/htdocs/ret.example.com"}
	if err := s.Sites.Create(ctx, site); err != nil {
		t.Fatal(err)
	}
	add := func(typ, status, path string) int64 {
		b := &Backup{SiteID: site.ID, BackupType: typ, StoragePath: sql.NullString{String: path, Valid: true}}
		if err := s.Backups.Create(ctx, b); err != nil {
			t.Fatal(err)
		}
		database.Exec(`UPDATE backups SET status = ? WHERE id = ?`, status, b.ID)
		return b.ID
	}
	good := add("files", "completed", "/x/files_20261101-020000.tar.gz")
	for i := 0; i < 3; i++ {
		add("files", "failed", "")
	}
	pending := add("files", "pending", "")
	// Two databases, two completed dumps each.
	shop1 := add("database", "completed", "/x/shop_20261101-020000.sql.gz")
	blog1 := add("database", "completed", "/x/blog_20261101-020000.sql.gz")
	add("database", "completed", "/x/shop_20261102-020000.sql.gz")
	add("database", "completed", "/x/blog_20261102-020000.sql.gz")

	deleted, err := s.Backups.DeleteOldest(ctx, site.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	gone := map[int64]bool{}
	for _, b := range deleted {
		gone[b.ID] = true
		if b.Status != "completed" {
			t.Errorf("deleted a %s backup", b.Status)
		}
	}
	if gone[good] || gone[pending] {
		t.Fatal("the only good files backup or a pending one was deleted")
	}
	if !gone[shop1] || !gone[blog1] || len(deleted) != 2 {
		t.Fatalf("expected the older dump of each database to go, got %v", gone)
	}
}
