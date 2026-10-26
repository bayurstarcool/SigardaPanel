package migrations

import (
	"database/sql"
	"fmt"
)

// RepairBackupsSchema re-applies schema drift the version ledger cannot see:
// migration 035 was edited in place after production had already recorded it,
// so deployments from the early releases lack the backup_type column while
// schema_migrations claims version 35 applied. This migration is idempotent —
// it checks pragma table_info before altering — so it is safe on databases
// that already have the column.
var RepairBackupsSchema = Migration{
	Version: 51,
	// repair backups.backup_type column (idempotent)
	Func: func(db *sql.Tx) error {
		rows, err := db.Query(`PRAGMA table_info(backups)`)
		if err != nil {
			return err
		}
		defer rows.Close()
		has := false
		for rows.Next() {
			var cid int
			var name, ctype string
			var notnull int
			var dflt any
			var pk int
			if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
				return err
			}
			if name == "backup_type" {
				has = true
			}
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if has {
			return nil
		}
		_, err = db.Exec(`ALTER TABLE backups ADD COLUMN backup_type TEXT DEFAULT 'files'`)
		if err != nil {
			return fmt.Errorf("add backup_type: %w", err)
		}
		return nil
	},
}
