package dbserver

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestGetBackupCredentialsReturnsDedicatedCredential(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE database_servers (
		id INTEGER PRIMARY KEY,
		name TEXT, engine TEXT, host TEXT, port INTEGER, username TEXT,
		password_encrypted TEXT, backup_username TEXT, backup_password_encrypted TEXT,
		is_active INTEGER, is_default INTEGER, created_at DATETIME, updated_at DATETIME
	);
	INSERT INTO database_servers VALUES (1,'local','MariaDB','127.0.0.1',3306,'root','root-enc','sigarda_backup','backup-enc',1,1,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP);`)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db, make([]byte, 32))
	user, encrypted, err := store.GetBackupCredentials(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if user != "sigarda_backup" || encrypted != "backup-enc" {
		t.Fatalf("got backup credential %q/%q", user, encrypted)
	}
}

func TestGetBackupCredentialsRejectsMissingDedicatedCredential(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE database_servers (
		id INTEGER PRIMARY KEY,
		name TEXT, engine TEXT, host TEXT, port INTEGER, username TEXT,
		password_encrypted TEXT, backup_username TEXT, backup_password_encrypted TEXT,
		is_active INTEGER, is_default INTEGER, created_at DATETIME, updated_at DATETIME
	);
	INSERT INTO database_servers VALUES (1,'local','MariaDB','127.0.0.1',3306,'root','root-enc','','',1,1,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP);`)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db, make([]byte, 32))
	if _, _, err := store.GetBackupCredentials(context.Background()); err == nil {
		t.Fatal("expected missing backup credential error")
	}
}
