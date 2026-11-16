package migrations

import (
	"database/sql"
)

// DatabaseManagedBySchema adds databases.managed_by: "sigardapanel" for
// databases the panel created (every existing row), "external" for databases
// imported from a server. The panel never drops, alters or re-grants an
// external database. Idempotent: checks pragma table_info first, like
// BackupStartedAtSchema.
var DatabaseManagedBySchema = Migration{
	Version: 61,
	Func: func(db *sql.Tx) error {
		has, err := tableHasColumn(db, "databases", "managed_by")
		if err != nil || has {
			return err
		}
		_, err = db.Exec(`ALTER TABLE databases ADD COLUMN managed_by TEXT NOT NULL DEFAULT 'sigardapanel'`)
		return err
	},
}

// tableHasColumn reports whether table has a column named column.
func tableHasColumn(db *sql.Tx, table, column string) (bool, error) {
	rows, err := db.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}
