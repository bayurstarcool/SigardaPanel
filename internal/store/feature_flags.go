package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// FeatureFlag represents a runtime feature flag tied to a tier.
type FeatureFlag struct {
	ID        int64
	Key       string
	Name      string
	Tier      string
	Enabled   bool
	CreatedAt time.Time
}

// FeatureFlagRepository defines feature flag persistence operations.
type FeatureFlagRepository interface {
	GetByKey(ctx context.Context, key string) (*FeatureFlag, error)
	List(ctx context.Context) ([]FeatureFlag, error)
	Upsert(ctx context.Context, flag *FeatureFlag) error
	Delete(ctx context.Context, key string) error
}

// SQLiteFeatureFlagStore implements FeatureFlagRepository backed by SQLite.
type SQLiteFeatureFlagStore struct {
	db *sql.DB
}

// NewFeatureFlagStore creates a new SQLiteFeatureFlagStore.
func NewFeatureFlagStore(db *sql.DB) *SQLiteFeatureFlagStore {
	return &SQLiteFeatureFlagStore{db: db}
}

// GetByKey returns a feature flag by its key.
func (s *SQLiteFeatureFlagStore) GetByKey(ctx context.Context, key string) (*FeatureFlag, error) {
	var f FeatureFlag
	var enabled int
	var createdAtStr string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, key, name, tier, enabled, created_at
		 FROM feature_flags WHERE key = ? LIMIT 1`, key,
	).Scan(&f.ID, &f.Key, &f.Name, &f.Tier, &enabled, &createdAtStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get feature flag: %w", err)
	}
	f.Enabled = enabled != 0
	f.CreatedAt, err = parseTimestamp(createdAtStr)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	return &f, nil
}

// List returns all feature flags.
func (s *SQLiteFeatureFlagStore) List(ctx context.Context) ([]FeatureFlag, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, key, name, tier, enabled, created_at FROM feature_flags ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("query feature flags: %w", err)
	}
	defer rows.Close()

	var flags []FeatureFlag
	for rows.Next() {
		var f FeatureFlag
		var enabled int
		if err := rows.Scan(&f.ID, &f.Key, &f.Name, &f.Tier, &enabled, &f.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan feature flag: %w", err)
		}
		f.Enabled = enabled != 0
		flags = append(flags, f)
	}
	return flags, rows.Err()
}

// Upsert inserts or updates a feature flag.
func (s *SQLiteFeatureFlagStore) Upsert(ctx context.Context, flag *FeatureFlag) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO feature_flags (key, name, tier, enabled)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET name=excluded.name, tier=excluded.tier, enabled=excluded.enabled`,
		flag.Key, flag.Name, flag.Tier, boolToInt(flag.Enabled),
	)
	if err != nil {
		return fmt.Errorf("upsert feature flag %s: %w", flag.Key, err)
	}
	return nil
}

// Delete removes a feature flag by key.
func (s *SQLiteFeatureFlagStore) Delete(ctx context.Context, key string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM feature_flags WHERE key = ?`, key)
	if err != nil {
		return fmt.Errorf("delete feature flag: %w", err)
	}
	return nil
}

// SeedDefaultFlags inserts all default feature flags. Flags that already
// exist (by key) are updated via upsert.
func (s *SQLiteFeatureFlagStore) SeedDefaultFlags(ctx context.Context) error {
	flags := []struct {
		key  string
		name string
		tier string
	}{
		// Free
		{"single_server", "Single Server", "free"},
		{"static_sites", "Static Sites", "free"},
		{"php", "PHP Support", "free"},
		{"ssl_certificates", "SSL Certificates", "free"},
		{"basic_monitoring", "Basic Monitoring", "free"},
		{"file_manager", "File Manager", "free"},
		{"terminal", "Terminal", "free"},
		{"job_history", "Job History", "free"},
		// Pro
		{"multi_server", "Multi Server", "pro"},
		{"advanced_runtimes", "Advanced Runtimes", "pro"},
		{"database_management", "Database Management", "pro"},
		{"cloud_backups", "Cloud Backups", "pro"},
		{"user_management", "User Management", "pro"},
		{"alerting", "Alerting", "pro"},
		{"scheduled_backups", "Scheduled Backups", "pro"},
		{"agent_management", "Agent Management", "pro"},
		{"custom_domains", "Custom Domains", "pro"},
		{"docker", "Docker", "pro"},
		// Enterprise
		{"white_label", "White Label", "enterprise"},
		{"api_access", "API Access", "enterprise"},
		{"audit_log", "Audit Log", "enterprise"},
		{"priority_support", "Priority Support", "enterprise"},
	}

	for _, f := range flags {
		flag := &FeatureFlag{
			Key:     f.key,
			Name:    f.name,
			Tier:    f.tier,
			Enabled: true,
		}
		if err := s.Upsert(ctx, flag); err != nil {
			return err
		}
	}
	return nil
}
