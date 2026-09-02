package jobmodel

import (
	"database/sql"
	"time"
)

const (
	StatusQueued    = "queued"
	StatusRunning   = "running"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
	StatusCanceled  = "canceled"
)

const (
	TypeSiteCreate     = "site.create"
	TypeSiteDelete     = "site.delete"
	TypeSiteDeploy     = "site.deploy"
	TypeSSLIssue       = "ssl.issue"
	TypeSSLRenew       = "ssl.renew"
	TypeSSLAutoRenew   = "ssl.auto-renew"
	TypeNginxReload    = "nginx.reload"
	TypeBackupCreate   = "backup.create"
	TypeBackupRestore  = "backup.restore"
	TypeBackupUpload   = "backup.upload"
	TypeDatabaseCreate = "database.create"
	TypeDatabaseDelete = "database.delete"
	TypeDBUserCreate   = "dbuser.create"
	TypeDBUserDelete   = "dbuser.delete"
	TypeAppStart       = "app.start"
	TypeAppStop        = "app.stop"
	TypeAppRestart     = "app.restart"
	TypeDBBackupCreate = "db_backup.create"
	TypeBackupDelete   = "backup.delete"
)

// Job represents a queued background task.
type Job struct {
	ID          int64
	ServerID    sql.NullInt64
	SiteID      sql.NullInt64
	Type        string
	Status      string
	Payload     string // JSON
	Result      string // JSON
	Error       sql.NullString
	Description string
	Attempts    int
	MaxAttempts int
	CreatedAt   time.Time
	UpdatedAt   time.Time
	StartedAt   sql.NullTime
	CompletedAt sql.NullTime
	NextRetryAt sql.NullTime
}
