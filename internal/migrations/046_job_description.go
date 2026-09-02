package migrations

// JobDescriptionSchema adds a human-readable description column to jobs.
var JobDescriptionSchema = Migration{
	Version: 46,
	SQL:     `ALTER TABLE jobs ADD COLUMN description TEXT;`,
}
