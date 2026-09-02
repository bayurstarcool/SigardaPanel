package db

import (
	"os"
	"testing"
)

func TestOpen(t *testing.T) {
	// Create a temporary database file
	tmpFile, err := os.CreateTemp("", "sigardapanel-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	// Test Open
	db, err := Open(tmpPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	// Verify connection works
	if err := db.Ping(); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
}

func TestOpen_WALMode(t *testing.T) {
	// Create a temporary database file
	tmpFile, err := os.CreateTemp("", "sigardapanel-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	db, err := Open(tmpPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	// Check WAL mode is active
	var journalMode string
	err = db.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	if err != nil {
		t.Fatalf("failed to query journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("journal_mode = %q, want %q", journalMode, "wal")
	}
}

func TestClose(t *testing.T) {
	// Create a temporary database file
	tmpFile, err := os.CreateTemp("", "sigardapanel-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	db, err := Open(tmpPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	// Test Close
	if err := Close(db); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestClose_NilDB(t *testing.T) {
	if err := Close(nil); err != nil {
		t.Errorf("Close(nil) error = %v, want nil", err)
	}
}