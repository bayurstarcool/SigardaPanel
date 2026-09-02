package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// BackupConfig represents a flexible backup configuration that can target
// multiple sites with a chosen scope (files, database, or both).
type BackupConfig struct {
	ID              int64
	Name            string
	ScopeType       string  // files|database|both
	SiteIDs         []int64 // JSON array
	ProviderID      sql.NullInt64
	CronExpr        string
	Enabled         bool
	RetentionDays   int      // days to keep backups; 0 = no auto-delete
	ExcludePatterns []string // JSON array of glob patterns
	DatabaseIDs     []int64  // JSON array of DB IDs; empty = all
	LastRunAt       sql.NullTime
	NextRunAt       sql.NullTime
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// BackupConfigRepository defines persistence operations for backup configs.
type BackupConfigRepository interface {
	Create(ctx context.Context, c *BackupConfig) error
	GetByID(ctx context.Context, id int64) (*BackupConfig, error)
	ListAll(ctx context.Context) ([]BackupConfig, error)
	Update(ctx context.Context, c *BackupConfig) error
	Delete(ctx context.Context, id int64) error
	ListDue(ctx context.Context) ([]BackupConfig, error)
	MarkRun(ctx context.Context, id int64) error
}

// SQLiteBackupConfigStore implements BackupConfigRepository.
type SQLiteBackupConfigStore struct {
	db *sql.DB
}

// NewBackupConfigStore returns a new store.
func NewBackupConfigStore(db *sql.DB) *SQLiteBackupConfigStore {
	return &SQLiteBackupConfigStore{db: db}
}

func (s *SQLiteBackupConfigStore) Create(ctx context.Context, cfg *BackupConfig) error {
	if cfg.ScopeType == "" {
		cfg.ScopeType = "files"
	}
	enabled := 0
	if cfg.Enabled {
		enabled = 1
	}
	siteIDsJSON := marshalInt64Slice(cfg.SiteIDs)
	excludeJSON := marshalStringSlice(cfg.ExcludePatterns)
	dbIDsJSON := marshalInt64Slice(cfg.DatabaseIDs)

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO backup_configs (name, scope_type, site_ids, provider_id, cron_expr, enabled, retention_days, exclude_patterns, database_ids, next_run_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		cfg.Name, cfg.ScopeType, siteIDsJSON, cfg.ProviderID, cfg.CronExpr, enabled, cfg.RetentionDays, excludeJSON, dbIDsJSON,
		NextCronRun(cfg.CronExpr, time.Now()).UTC().Format("2006-01-02 15:04:05"),
	)
	if err != nil {
		return fmt.Errorf("insert backup config: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("last insert id: %w", err)
	}
	cfg.ID = id
	return nil
}

func (s *SQLiteBackupConfigStore) GetByID(ctx context.Context, id int64) (*BackupConfig, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, scope_type, site_ids, provider_id, cron_expr, enabled, retention_days, exclude_patterns, database_ids,
		        last_run_at, next_run_at, created_at, updated_at
		 FROM backup_configs WHERE id = ?`, id)
	return scanBackupConfig(row)
}

func (s *SQLiteBackupConfigStore) ListAll(ctx context.Context) ([]BackupConfig, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, scope_type, site_ids, provider_id, cron_expr, enabled, retention_days, exclude_patterns, database_ids,
		        last_run_at, next_run_at, created_at, updated_at
		 FROM backup_configs ORDER BY id DESC`)
	if err != nil {
		return nil, fmt.Errorf("query backup configs: %w", err)
	}
	defer rows.Close()
	var out []BackupConfig
	for rows.Next() {
		var cfg BackupConfig
		if err := scanBackupConfigRows(rows, &cfg); err != nil {
			return nil, err
		}
		out = append(out, cfg)
	}
	return out, rows.Err()
}

func (s *SQLiteBackupConfigStore) Update(ctx context.Context, cfg *BackupConfig) error {
	enabled := 0
	if cfg.Enabled {
		enabled = 1
	}
	siteIDsJSON := marshalInt64Slice(cfg.SiteIDs)
	excludeJSON := marshalStringSlice(cfg.ExcludePatterns)
	dbIDsJSON := marshalInt64Slice(cfg.DatabaseIDs)

	_, err := s.db.ExecContext(ctx,
		`UPDATE backup_configs SET name = ?, scope_type = ?, site_ids = ?, provider_id = ?, cron_expr = ?,
		        enabled = ?, retention_days = ?, exclude_patterns = ?, database_ids = ?,
		        next_run_at = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		cfg.Name, cfg.ScopeType, siteIDsJSON, cfg.ProviderID, cfg.CronExpr, enabled, cfg.RetentionDays, excludeJSON, dbIDsJSON,
		NextCronRun(cfg.CronExpr, time.Now()).UTC().Format("2006-01-02 15:04:05"),
		cfg.ID,
	)
	if err != nil {
		return fmt.Errorf("update backup config: %w", err)
	}
	return nil
}

func (s *SQLiteBackupConfigStore) Delete(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM backup_configs WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete backup config: %w", err)
	}
	return nil
}

func (s *SQLiteBackupConfigStore) ListDue(ctx context.Context) ([]BackupConfig, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, scope_type, site_ids, provider_id, cron_expr, enabled, retention_days, exclude_patterns, database_ids,
		        last_run_at, next_run_at, created_at, updated_at
		 FROM backup_configs WHERE enabled = 1 AND (next_run_at IS NULL OR next_run_at <= CURRENT_TIMESTAMP)`)
	if err != nil {
		return nil, fmt.Errorf("query due backup configs: %w", err)
	}
	defer rows.Close()
	var out []BackupConfig
	for rows.Next() {
		var cfg BackupConfig
		if err := scanBackupConfigRows(rows, &cfg); err != nil {
			return nil, err
		}
		out = append(out, cfg)
	}
	return out, rows.Err()
}

func (s *SQLiteBackupConfigStore) MarkRun(ctx context.Context, id int64) error {
	var cronExpr string
	err := s.db.QueryRowContext(ctx, `SELECT cron_expr FROM backup_configs WHERE id = ?`, id).Scan(&cronExpr)
	if err != nil {
		return fmt.Errorf("get cron expr: %w", err)
	}
	now := time.Now().UTC()
	nextRun := NextCronRun(cronExpr, now).UTC()
	_, err = s.db.ExecContext(ctx,
		`UPDATE backup_configs SET last_run_at = ?, next_run_at = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		now.Format("2006-01-02 15:04:05"), nextRun.Format("2006-01-02 15:04:05"), id,
	)
	if err != nil {
		return fmt.Errorf("mark run: %w", err)
	}
	return nil
}

// ─── JSON helpers ──────────────────────────────────────────

func marshalInt64Slice(vals []int64) string {
	if vals == nil {
		return "[]"
	}
	b, _ := json.Marshal(vals)
	return string(b)
}

func marshalStringSlice(vals []string) string {
	if vals == nil {
		return "[]"
	}
	b, _ := json.Marshal(vals)
	return string(b)
}

func unmarshalInt64Slice(s string) []int64 {
	var out []int64
	if s == "" || s == "null" {
		return out
	}
	_ = json.Unmarshal([]byte(s), &out)
	return out
}

func unmarshalStringSlice(s string) []string {
	var out []string
	if s == "" || s == "null" {
		return out
	}
	_ = json.Unmarshal([]byte(s), &out)
	return out
}

// ─── Scanner helpers ──────────────────────────────────────

func scanBackupConfig(row *sql.Row) (*BackupConfig, error) {
	var cfg BackupConfig
	var siteIDsJSON, excludeJSON, dbIDsJSON string
	err := row.Scan(
		&cfg.ID, &cfg.Name, &cfg.ScopeType, &siteIDsJSON, &cfg.ProviderID, &cfg.CronExpr, &cfg.Enabled, &cfg.RetentionDays,
		&excludeJSON, &dbIDsJSON, &cfg.LastRunAt, &cfg.NextRunAt, &cfg.CreatedAt, &cfg.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan backup config: %w", err)
	}
	cfg.SiteIDs = unmarshalInt64Slice(siteIDsJSON)
	cfg.ExcludePatterns = unmarshalStringSlice(excludeJSON)
	cfg.DatabaseIDs = unmarshalInt64Slice(dbIDsJSON)
	return &cfg, nil
}

func scanBackupConfigRows(rows *sql.Rows, cfg *BackupConfig) error {
	var siteIDsJSON, excludeJSON, dbIDsJSON string
	err := rows.Scan(
		&cfg.ID, &cfg.Name, &cfg.ScopeType, &siteIDsJSON, &cfg.ProviderID, &cfg.CronExpr, &cfg.Enabled, &cfg.RetentionDays,
		&excludeJSON, &dbIDsJSON, &cfg.LastRunAt, &cfg.NextRunAt, &cfg.CreatedAt, &cfg.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("scan backup config row: %w", err)
	}
	cfg.SiteIDs = unmarshalInt64Slice(siteIDsJSON)
	cfg.ExcludePatterns = unmarshalStringSlice(excludeJSON)
	cfg.DatabaseIDs = unmarshalInt64Slice(dbIDsJSON)
	return nil
}
