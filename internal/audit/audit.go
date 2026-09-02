package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"time"
)

// Action constants — naming: verb_noun (lowercase, underscore)
const (
	ActionLogin         = "login"
	ActionLogout        = "logout"
	ActionUserCreate    = "user_create"
	ActionUserDelete    = "user_delete"
	ActionUserUpdate    = "user_update"
	ActionServerCreate  = "server_create"
	ActionServerDelete  = "server_delete"
	ActionServerUpdate  = "server_update"
	ActionSiteCreate    = "site_create"
	ActionSiteDelete    = "site_delete"
	ActionSiteUpdate    = "site_update"
	ActionSiteDeploy    = "site_deploy"
	ActionSSLIssue      = "ssl_issue"
	ActionSSLRenew      = "ssl_renew"
	ActionBackupCreate  = "backup_create"
	ActionBackupRestore = "backup_restore"
	ActionJobCancel     = "job_cancel"
	ActionTransfer      = "transfer"
)

// Resource constants
const (
	ResourceUser    = "user"
	ResourceServer  = "server"
	ResourceSite    = "site"
	ResourceSSL     = "ssl"
	ResourceBackup  = "backup"
	ResourceSession = "session"
	ResourceJob     = "job"
)

// Entry represents one audit log record.
type Entry struct {
	ID         int64         `json:"id"`
	UserID     sql.NullInt64 `json:"user_id"`
	Action     string        `json:"action"`
	Resource   string        `json:"resource"`
	ResourceID sql.NullInt64 `json:"resource_id"`
	Details    sql.NullString `json:"details,omitempty"`
	CreatedAt  time.Time     `json:"created_at"`
}

// Recorder writes audit entries.
type Recorder struct {
	db *sql.DB
}

// NewRecorder returns a new audit Recorder backed by db.
func NewRecorder(db *sql.DB) *Recorder {
	return &Recorder{db: db}
}

// Record writes an audit entry. Best-effort — errors are logged but
// don't fail the calling operation (audit should not block business logic).
func (r *Recorder) Record(ctx context.Context, entry Entry) {
	var detailsParam any = entry.Details
	if !entry.Details.Valid {
		detailsParam = sql.NullString{}
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO audit_logs (user_id, action, resource, resource_id, details) VALUES (?, ?, ?, ?, ?)`,
		entry.UserID, entry.Action, entry.Resource, entry.ResourceID, detailsParam,
	)
	if err != nil {
		slog.Error("audit: insert failed", "error", err, "action", entry.Action)
	}
}

// RecordAction is a convenience helper that serializes details as JSON.
func (r *Recorder) RecordAction(ctx context.Context, userID int64, action, resource string, resourceID int64, details any) {
	var detailsJSON string
	if details != nil {
		b, err := json.Marshal(details)
		if err != nil {
			slog.Error("audit: marshal details", "error", err)
			return
		}
		detailsJSON = string(b)
	}

	var uid sql.NullInt64
	if userID > 0 {
		uid = sql.NullInt64{Int64: userID, Valid: true}
	}

	var rid sql.NullInt64
	if resourceID > 0 {
		rid = sql.NullInt64{Int64: resourceID, Valid: true}
	}

	var det sql.NullString
	if detailsJSON != "" {
		det = sql.NullString{String: detailsJSON, Valid: true}
	}

	r.Record(ctx, Entry{
		UserID:     uid,
		Action:     action,
		Resource:   resource,
		ResourceID: rid,
		Details:    det,
	})
}
