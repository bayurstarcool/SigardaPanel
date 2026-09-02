// Package db provides SQLite database connection helpers.
package db

import (
	"database/sql"
	"fmt"
	"time"

	"sigardapanel/internal/config"

	_ "modernc.org/sqlite"
)

// Open opens a SQLite database with WAL mode and recommended settings.
// The path can be overridden via SIGARDAPANEL_DB_PATH environment variable.
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+
		"?_pragma=journal_mode(WAL)"+
		"&_pragma=busy_timeout(5000)"+
		"&_pragma=foreign_keys(ON)"+
		"&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	return db, nil
}

// Close gracefully closes the database connection.
func Close(db *sql.DB) error {
	if db == nil {
		return nil
	}
	return db.Close()
}

// MustOpen opens the database using the default path from config.
// It panics if opening fails.
func MustOpen() *sql.DB {
	database, err := Open(config.DBPath())
	if err != nil {
		panic("failed to open database: " + err.Error())
	}
	return database
}