package store

import (
	"crypto/sha256"
	"database/sql"
	"fmt"

	"sigardapanel/internal/audit"
	"sigardapanel/internal/config"
	"sigardapanel/internal/dbserver"
)

// Store combines all repository interfaces under a single struct.
type Store struct {
	db            *sql.DB
	Users         UserRepository
	Sessions      SessionRepository
	AuditLogs     audit.AuditLogStore
	Servers       ServerRepository
	Sites         SiteRepository
	Jobs          JobRepository
	JobLogs       JobLogRepository
	Backups       BackupRepository
	Metrics       MetricRepository
	AlertChannels AlertChannelRepository
	Alerts        AlertRepository
	Databases     DatabaseRepository
	DBUsers       DBUserRepository
	DBStore       *dbserver.Store
	Vhosts        VhostRepository
	Varnish       VarnishRepository
	Security      SecurityRepository
	SSH           SSHRepository
	Cron          CronRepository
	Cloudflare    *CloudflareStore
	UserSites     *SQLiteUserSiteStore
	BackupProviders *SQLiteBackupStorageProviderStore
	BackupSchedules BackupScheduleRepository
	BackupConfigs      BackupConfigRepository
	ServiceTokens      ServiceTokenRepository
	Notifications      NotificationRepository
	RegistrationTokens RegistrationTokenRepository
	Licenses           LicenseRepository
	FeatureFlags       FeatureFlagRepository
	Plans              PlansRepository
	Orders             OrdersRepository
	TwoFactor          *SQLiteTwoFactorStore
	IPRestrictions     *SQLiteIPRestrictionStore
}

// New returns a Store with all repositories backed by the given SQLite database.
func New(db *sql.DB) *Store {
	s := &Store{
		db:            db,
		Users:         NewUserStore(db),
		Sessions:      NewSessionStore(db),
		AuditLogs:     NewAuditLogStore(db),
		Servers:       NewServerStore(db),
		Sites:         NewSiteStore(db),
		Jobs:          NewJobStore(db),
		JobLogs:       NewJobLogStore(db),
		Backups:       NewBackupStore(db),
		Metrics:       NewMetricStore(db),
		AlertChannels: NewAlertChannelStore(db),
		Alerts:        NewAlertStore(db),
		Databases:     NewDatabaseStore(db),
		DBUsers:       NewDBUserStore(db),
		Vhosts:        NewVhostStore(db),
		Varnish:       NewVarnishStore(db),
		Security:      NewSecurityStore(db),
		SSH:           NewSSHStore(db),
		Cron:          NewCronStore(db),
		Cloudflare:    NewCloudflareStore(db),
		UserSites:     NewUserSiteStore(db),
		BackupProviders: NewBackupStorageProviderStore(db),
		BackupSchedules: NewBackupScheduleStore(db),
		BackupConfigs:      NewBackupConfigStore(db),
		ServiceTokens:      NewServiceTokenStore(db),
		Notifications:      NewNotificationStore(db),
		RegistrationTokens: NewRegistrationTokenStore(db),
		Licenses:           NewLicenseStore(db),
		FeatureFlags:       NewFeatureFlagStore(db),
		Plans:              NewPlanStore(db),
		Orders:             NewOrderStore(db),
		TwoFactor:          NewSQLiteTwoFactorStore(db),
		IPRestrictions:     NewSQLiteIPRestrictionStore(db),
	}
	// Initialize DBStore with encryption key derived from config
	encKey := sha256.Sum256(config.EncryptionKey())
	s.DBStore = dbserver.NewStore(db, encKey[:])
	return s
}

// DB returns the underlying database connection.
func (s *Store) DB() *sql.DB {
	return s.db
}

// IsNotFound returns true when err is sql.ErrNoRows.
func IsNotFound(err error) bool {
	return err == sql.ErrNoRows
}

// FormatNotFound wraps a not-found error with context.
func FormatNotFound(entity string, err error) error {
	return fmt.Errorf("%s not found: %w", entity, err)
}
