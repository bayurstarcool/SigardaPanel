package migrations

import (
	"database/sql"
	"strings"
	"time"
)

// NormalizeSSLExpiresAtSchema rewrites sites.ssl_expires_at into the canonical
// UTC "2006-01-02 15:04:05" form. Older builds passed a time.Time straight to
// the driver, which stored time.String() output such as
// "2026-12-04 11:41:55.264167352 +0000 UTC m=+7776485.03"; those values do not
// compare correctly against the text cutoffs used by the SSL expiry queries.
// Values that cannot be parsed are left untouched. Idempotent.
var NormalizeSSLExpiresAtSchema = Migration{
	Version: 53,
	Func: func(tx *sql.Tx) error {
		rows, err := tx.Query(`SELECT id, CAST(ssl_expires_at AS TEXT) FROM sites WHERE ssl_expires_at IS NOT NULL`)
		if err != nil {
			return err
		}
		type fix struct {
			id  int64
			val string
		}
		var fixes []fix
		for rows.Next() {
			var id int64
			var raw string
			if err := rows.Scan(&id, &raw); err != nil {
				rows.Close()
				return err
			}
			if t, ok := ParseLegacyTimestamp(raw); ok {
				if canon := t.UTC().Format(canonicalTimestamp); canon != raw {
					fixes = append(fixes, fix{id, canon})
				}
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, f := range fixes {
			if _, err := tx.Exec(`UPDATE sites SET ssl_expires_at = ? WHERE id = ?`, f.val, f.id); err != nil {
				return err
			}
		}
		return nil
	},
}

const canonicalTimestamp = "2006-01-02 15:04:05"

var legacyTimestampLayouts = []string{
	"2006-01-02 15:04:05.999999999 -0700 MST", // time.Time.String()
	"2006-01-02 15:04:05.999999999 -0700",
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02T15:04:05.999999999Z07:00", // RFC3339(Nano)
	"2006-01-02T15:04:05.999999999",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02T15:04",
	"2006-01-02 15:04",
	"2006-01-02",
}

// ParseLegacyTimestamp parses the timestamp shapes found in SQLite text
// columns: canonical UTC, RFC3339, and Go's time.String() output including a
// trailing monotonic clock reading (" m=+123.45"), which is stripped.
// Values without a zone are taken as UTC.
func ParseLegacyTimestamp(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, " m="); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range legacyTimestampLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}
