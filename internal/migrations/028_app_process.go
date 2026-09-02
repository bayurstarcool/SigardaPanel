package migrations

// AppProcessSchema adds app process management columns to sites table.
// app_port: assigned port for app runtime (node/go/python)
// app_command: the start command (e.g. "node server.js", "./main")
// app_status: running/stopped/errored
var AppProcessSchema = Migration{
	Version: 28,
	SQL: `ALTER TABLE sites ADD COLUMN app_port INTEGER;
ALTER TABLE sites ADD COLUMN app_command TEXT DEFAULT '';
ALTER TABLE sites ADD COLUMN app_status TEXT DEFAULT 'stopped';`,
}
