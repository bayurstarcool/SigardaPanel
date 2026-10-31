// Package migrations provides database migration utilities.
package migrations

import (
	"database/sql"
	"fmt"
)

// Migration represents a single database migration. SQL is executed as-is;
// Func, when set, runs instead of SQL for migrations that must inspect the
// database before acting (idempotent repairs).
type Migration struct {
	Version int
	SQL     string
	Func    func(tx *sql.Tx) error
}

// Up runs all pending migrations up.
func Up(db *sql.DB, migrations []Migration) error {
	if err := createSchemaMigrationsTable(db); err != nil {
		return err
	}

	for _, m := range migrations {
		applied, err := isApplied(db, m.Version)
		if err != nil {
			return err
		}
		if applied {
			continue
		}

		if err := applyMigration(db, m); err != nil {
			return fmt.Errorf("migration %d failed: %w", m.Version, err)
		}
	}

	return nil
}

// AllMigrations is the complete ordered list of all migrations.
var AllMigrations = []Migration{
	InitialSchema,
	AuthSchema,
	JobsSchema,
	SitesOwnerSchema,
	SSLSchema,
	GitDeploySchema,
	BackupsSchema,
	MetricsSchema,
	AlertsSchema,
	JobLogsSchema,
	JobRetrySchema,
	DatabasesSchema,
	ServerAgentURLSchema,
	UserCreatedBySchema,
	DatabaseServersSchema,
	EncryptedPasswordSchema,
	SiteAdvancedSchema,
	SiteOriginSchema,
	CloudflareSchema,
	UserSitesSchema,
	ImpersonationSchema,
	SSHKeysSchema,
	UserDiskLimitSchema,
	UserMaxSitesSchema,
	BackupProvidersSchema,
	MetricsSwapServerInfoSchema,
	ServiceTokensSchema,
	AppProcessSchema,
	AppMemoryLimitSchema,
	PHPConfigSchema,
	CloudflarePerUserSchema,
	NotificationsSchema,
	BackupSchedulesSchema,
	BackupConfigsSchema,
	BackupProgressSchema,
	BackupConfigRetentionSchema,
	ServerEditFieldsSchema,
	AgentHealthSchema,
	RegistrationTokensSchema,
	LicensingSchema,
	PlansLicenseOrdersSchema,
	GPUMetricsSchema,
	JobDescriptionSchema,
	TwoFactorSchema,
	IPRestrictionSchema,
	PasswordChangedSchema,
	RecoveryCodesSchema,
	BackupCredentialsSchema,
	CloudflareConfigPerUserSchema,
	SecurityBaselineSchema,
	RepairBackupsSchema,
	BackupStartedAtSchema,
}

func Down(db *sql.DB) error {
	return nil
}

func createSchemaMigrationsTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return err
}

func isApplied(db *sql.DB, version int) (bool, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = ?", version).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func applyMigration(db *sql.DB, m Migration) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if m.Func != nil {
		if err := m.Func(tx); err != nil {
			return err
		}
	} else if _, err := tx.Exec(m.SQL); err != nil {
		return err
	}

	if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", m.Version); err != nil {
		return err
	}

	return tx.Commit()
}
