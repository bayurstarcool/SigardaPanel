package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// BackupStorageProvider represents an S3-compatible storage config.
type BackupStorageProvider struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	ProviderType string    `json:"provider_type"` // s3, b2, r2, wasabi, digitalocean, generic
	Bucket       string    `json:"bucket"`
	Endpoint     string    `json:"endpoint"`
	Region       string    `json:"region"`
	AccessKey    string    `json:"access_key"`
	SecretKey    string    `json:"-"` // never exposed to frontend
	IsDefault    bool      `json:"is_default"`
	RetentionDays int     `json:"retention_days"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// BackupStorageProviderRepository defines persistence operations.
type BackupStorageProviderRepository interface {
	Create(ctx context.Context, p *BackupStorageProvider) error
	GetByID(ctx context.Context, id int64) (*BackupStorageProvider, error)
	List(ctx context.Context) ([]BackupStorageProvider, error)
	Update(ctx context.Context, p *BackupStorageProvider) error
	Delete(ctx context.Context, id int64) error
	GetDefault(ctx context.Context) (*BackupStorageProvider, error)
}

// SQLiteBackupStorageProviderStore implements BackupStorageProviderRepository.
type SQLiteBackupStorageProviderStore struct {
	db *sql.DB
}

// NewBackupStorageProviderStore returns a new store.
func NewBackupStorageProviderStore(db *sql.DB) *SQLiteBackupStorageProviderStore {
	return &SQLiteBackupStorageProviderStore{db: db}
}

// Create inserts a new provider. If is_default is true, clears other defaults first.
func (s *SQLiteBackupStorageProviderStore) Create(ctx context.Context, p *BackupStorageProvider) error {
	if p.IsDefault {
		if _, err := s.db.ExecContext(ctx, `UPDATE backup_storage_providers SET is_default = 0`); err != nil {
			return fmt.Errorf("clear defaults: %w", err)
		}
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO backup_storage_providers (name, provider_type, bucket, endpoint, region, access_key, secret_key, is_default, retention_days)
	 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Name, p.ProviderType, p.Bucket, p.Endpoint, p.Region, p.AccessKey, p.SecretKey, providerBoolToInt(p.IsDefault), p.RetentionDays,
	)
	if err != nil {
		return fmt.Errorf("insert backup provider: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("last insert id: %w", err)
	}
	p.ID = id
	return nil
}

// GetByID retrieves a provider by ID.
func (s *SQLiteBackupStorageProviderStore) GetByID(ctx context.Context, id int64) (*BackupStorageProvider, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, provider_type, bucket, endpoint, region, access_key, secret_key, is_default, retention_days, created_at, updated_at
	 FROM backup_storage_providers WHERE id = ?`, id)
	return scanBackupProvider(row)
}

// List returns all providers ordered by is_default DESC, name ASC.
func (s *SQLiteBackupStorageProviderStore) List(ctx context.Context) ([]BackupStorageProvider, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, provider_type, bucket, endpoint, region, access_key, secret_key, is_default, retention_days, created_at, updated_at
	 FROM backup_storage_providers ORDER BY is_default DESC, name ASC`)
	if err != nil {
		return nil, fmt.Errorf("query backup providers: %w", err)
	}
	defer rows.Close()

	var out []BackupStorageProvider
	for rows.Next() {
		var p BackupStorageProvider
		if err := scanBackupProviderRows(rows, &p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Update modifies an existing provider.
func (s *SQLiteBackupStorageProviderStore) Update(ctx context.Context, p *BackupStorageProvider) error {
	if p.IsDefault {
		if _, err := s.db.ExecContext(ctx, `UPDATE backup_storage_providers SET is_default = 0 WHERE id != ?`, p.ID); err != nil {
			return fmt.Errorf("clear defaults: %w", err)
		}
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE backup_storage_providers SET name=?, provider_type=?, bucket=?, endpoint=?, region=?, access_key=?, secret_key=?, is_default=?, retention_days=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		p.Name, p.ProviderType, p.Bucket, p.Endpoint, p.Region, p.AccessKey, p.SecretKey, providerBoolToInt(p.IsDefault), p.RetentionDays, p.ID,
	)
	if err != nil {
		return fmt.Errorf("update backup provider: %w", err)
	}
	return nil
}

// Delete removes a provider.
func (s *SQLiteBackupStorageProviderStore) Delete(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM backup_storage_providers WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete backup provider: %w", err)
	}
	return nil
}

// GetDefault returns the default provider, or nil if none set.
func (s *SQLiteBackupStorageProviderStore) GetDefault(ctx context.Context) (*BackupStorageProvider, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, provider_type, bucket, endpoint, region, access_key, secret_key, is_default, retention_days, created_at, updated_at
	 FROM backup_storage_providers WHERE is_default = 1 LIMIT 1`)
	p, err := scanBackupProvider(row)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// ─── helpers ────────────────────────────────────────────────

func scanBackupProvider(row *sql.Row) (*BackupStorageProvider, error) {
	var p BackupStorageProvider
	err := row.Scan(
		&p.ID, &p.Name, &p.ProviderType, &p.Bucket, &p.Endpoint, &p.Region,
		&p.AccessKey, &p.SecretKey, &p.IsDefault, &p.RetentionDays, &p.CreatedAt, &p.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan backup provider: %w", err)
	}
	return &p, nil
}

func scanBackupProviderRows(rows *sql.Rows, p *BackupStorageProvider) error {
	return rows.Scan(
		&p.ID, &p.Name, &p.ProviderType, &p.Bucket, &p.Endpoint, &p.Region,
		&p.AccessKey, &p.SecretKey, &p.IsDefault, &p.RetentionDays, &p.CreatedAt, &p.UpdatedAt,
	)
}

func providerBoolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
