package migrations

import (
	"database/sql"
)

// BackupStartedAtSchema adds backups.started_at. Stuck-run detection used
// created_at, so a backup that waited in the queue for over 30 minutes was
// marked failed the moment it started running. Idempotent: checks
// pragma table_info first, like RepairBackupsSchema.
var BackupStartedAtSchema = Migration{
	Version: 52,
	Func: func(db *sql.Tx) error {
		rows, err := db.Query(`PRAGMA table_info(backups)`)
		if err != nil {
			return err
		}
		has := false
		for rows.Next() {
			var cid, notnull, pk int
			var name, ctype string
			var dflt any
			if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
				rows.Close()
				return err
			}
			if name == "started_at" {
				has = true
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if has {
			return nil
		}
		_, err = db.Exec(`ALTER TABLE backups ADD COLUMN started_at DATETIME`)
		return err
	},
}
