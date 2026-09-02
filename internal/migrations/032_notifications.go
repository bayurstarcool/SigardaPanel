package migrations

// NotificationsSchema creates the in-app notifications table.
var NotificationsSchema = Migration{
	Version: 32,
	SQL: `
CREATE TABLE IF NOT EXISTS notifications (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER NOT NULL,
    type        TEXT    NOT NULL DEFAULT 'info',
    title       TEXT    NOT NULL,
    message     TEXT    NOT NULL DEFAULT '',
    severity    TEXT    NOT NULL DEFAULT 'info',
    read        INTEGER NOT NULL DEFAULT 0,
    entity_type TEXT    NOT NULL DEFAULT '',
    entity_id   INTEGER NOT NULL DEFAULT 0,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    read_at     DATETIME
);
CREATE INDEX IF NOT EXISTS idx_notifications_user    ON notifications(user_id);
CREATE INDEX IF NOT EXISTS idx_notifications_read    ON notifications(user_id, read);
CREATE INDEX IF NOT EXISTS idx_notifications_created ON notifications(created_at DESC);
`,
}
