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
	SecurityBaselines SecurityBaselineRepository
	ServiceTokens      ServiceTokenRepository
	Notifications      NotificationRepository
	RegistrationTokens RegistrationTokenRepository
	Licenses           LicenseRepository
	FeatureFlags       FeatureFlagRepository
	Plans              PlansRepository
	Orders             OrdersRepository
	TwoFactor          *SQLiteTwoFactorStore
	IPRestrictions     *SQLiteIPRestrictionStore
	APIKeys            *SQLiteUserAPIKeyStore
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
		SecurityBaselines: NewSecurityBaselineStore(db),
		ServiceTokens:      NewServiceTokenStore(db),
		Notifications:      NewNotificationStore(db),
		RegistrationTokens: NewRegistrationTokenStore(db),
		Licenses:           NewLicenseStore(db),
		FeatureFlags:       NewFeatureFlagStore(db),
		Plans:              NewPlanStore(db),
		Orders:             NewOrderStore(db),
		TwoFactor:          NewSQLiteTwoFactorStore(db),
		IPRestrictions:     NewSQLiteIPRestrictionStore(db),
		APIKeys:            NewUserAPIKeyStore(db),
	}
	// DBStore's key is derived from the panel encryption key on first use,
	// not here: commands that open the store without touching secrets
	// ("sigardapanel init", CLI imports) must not resolve the key, which on a
	// fresh dev database logged a misleading "derived fallback key" ERROR.
	// The API settles the key (secrets.InitEncryptionKey) before store.New,
	// so for it the resolved key is unchanged.
	s.DBStore = dbserver.NewStoreWithKeyFunc(db, func() []byte {
		encKey := sha256.Sum256(config.EncryptionKey())
		return encKey[:]
	})
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
