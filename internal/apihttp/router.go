package apihttp

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"sigardapanel/internal/audit"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/member"
	"sigardapanel/internal/store"
)

const version = "0.5.0"

// NewRouter returns a new Echo router wired with the given store and database.
func NewRouter(s *store.Store, db *sql.DB, memberDB *sql.DB) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Use(middleware.BodyLimit("32M"))

	// Request ID middleware — Echo built-in
	e.Use(middleware.RequestIDWithConfig(middleware.RequestIDConfig{
		Generator: func() string {
			b := make([]byte, 16)
			if _, err := rand.Read(b); err != nil {
				return "req_unknown"
			}
			return "req_" + hex.EncodeToString(b)
		},
	}))

	// Rate limiting middleware
	apiLimiter := APIlimiter()
	e.Use(apiLimiter.Middleware())

	// Request timeout middleware
	e.Use(TimeoutMiddleware())

	// Custom HTTP error handler to use our JSON format
	e.HTTPErrorHandler = customErrorHandler

	auditRec := audit.NewRecorder(db)
	ah := newAuthHandler(s, auditRec)
	uh := newUserHandler(s, auditRec)

	// Public endpoints
	e.GET("/health", healthHandler)
	e.GET("/api/v1/health", healthHandler)
	e.GET("/api/v1/version", versionHandler)
	// Auth endpoints
	loginLimiter := StrictLimiter()
	registerLimiter := StrictLimiter()
	e.POST("/api/v1/auth/login", ah.login, loginLimiter.Middleware())
	e.POST("/api/v1/auth/register", ah.register, registerLimiter.Middleware())

	// Two-Factor Authentication (2FA) endpoints
	tfaH := newTwoFactorHandler(s)
	e.POST("/api/v1/auth/2fa/setup", tfaH.setup2FA, RequireAuth(s))
	e.POST("/api/v1/auth/2fa/enable", tfaH.enable2FA, RequireAuth(s))
	e.POST("/api/v1/auth/2fa/disable", tfaH.disable2FA, RequireAuth(s))
	e.POST("/api/v1/auth/2fa/verify", tfaH.verify2FA, RequireAuth(s))
	e.GET("/api/v1/auth/2fa/status", tfaH.status2FA, RequireAuth(s))
	e.POST("/api/v1/auth/2fa/recovery-codes", tfaH.generateRecoveryCodes, RequireAuth(s))

	// Member routes (separate database)
	if memberDB != nil {
		member.RegisterRoutes(e, memberDB)
	}

	e.GET("/api/v1/auth/me", ah.me, RequireAuth(s))
	e.PUT("/api/v1/auth/profile", ah.updateProfile, RequireAuth(s))
	e.POST("/api/v1/auth/logout", ah.logout, RequireAuth(s))
	e.POST("/api/v1/auth/stop-impersonation", ah.stopImpersonation, RequireAuth(s), RequireUserSession)

	// User management (admin only)
	e.GET("/api/v1/users", uh.list, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/users", uh.create, RequireAuth(s), RequireAdmin(s))
	e.DELETE("/api/v1/users/:id", uh.delete, RequireAuth(s), RequireAdmin(s))
	e.PATCH("/api/v1/users/:id", uh.update, RequireAuth(s), RequireAdmin(s))

	// User usage/quota (authenticated users can check their own)
	e.GET("/api/v1/users/:id/usage", uh.usage, RequireAuth(s))

	// User ↔ Site access mappings (admin only)
	e.GET("/api/v1/users/:id/sites", uh.listUserSites, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/users/:id/sites", uh.assignUserSite, RequireAuth(s), RequireAdmin(s))
	e.DELETE("/api/v1/users/:id/sites/:siteId", uh.unassignUserSite, RequireAuth(s), RequireAdmin(s))
	e.PATCH("/api/v1/users/:id/sites/:siteId", uh.updateSitePermission, RequireAuth(s), RequireAdmin(s))
	e.GET("/api/v1/sites/:id/users", uh.listSiteUsers, RequireAuth(s), RequireAdmin(s))

	// Impersonation (admin only)
	e.POST("/api/v1/users/:id/impersonate", ah.impersonate, RequireAuth(s), RequireAdmin(s), RequireUserSession)

	// Protected endpoints
	sh := newServerHandler(s, auditRec)
	e.GET("/api/v1/servers", sh.list, RequireAuth(s))
	e.POST("/api/v1/servers", sh.create, RequireAuth(s), RequireAdmin(s))
	e.GET("/api/v1/servers/:id", sh.get, RequireAuth(s))
	e.PATCH("/api/v1/servers/:id", sh.update, RequireAuth(s), RequireAdmin(s))
	e.DELETE("/api/v1/servers/:id", sh.delete, RequireAuth(s), RequireAdmin(s))
	e.GET("/api/v1/servers/health", sh.getHealthAll, RequireAuth(s))
	e.GET("/api/v1/servers/:id/health", sh.getHealth, RequireAuth(s))
	e.POST("/api/v1/servers/:id/token", sh.regenerateToken, RequireAuth(s), RequireAdmin(s))

	sitesH := newSiteHandler(s, auditRec)
	e.GET("/api/v1/sites", sitesH.list, RequireAuth(s))
	e.POST("/api/v1/sites", sitesH.create, RequireAuth(s))
	e.GET("/api/v1/sites/:id", sitesH.get, RequireAuth(s))
	e.DELETE("/api/v1/sites/:id", sitesH.delete, RequireAuth(s))
	e.POST("/api/v1/sites/:id/deploy", sitesH.deploy, RequireAuth(s))
	e.POST("/api/v1/sites/:id/config", sitesH.updateConfig, RequireAuth(s))
	e.POST("/api/v1/sites/:id/reload-nginx", sitesH.reloadNginx, RequireAuth(s))
	e.POST("/api/v1/sites/:id/transfer", sitesH.transfer, RequireAuth(s))

	// Public webhook endpoints (no auth)
	wh := newWebhookHandler(s)
	e.POST("/api/v1/webhooks/deploy/:site_id", wh.deploy)

	jh := newJobHandler(s, auditRec)
	e.GET("/api/v1/jobs", jh.list, RequireAuth(s))
	e.GET("/api/v1/jobs/:id", jh.get, RequireAuth(s))
	e.POST("/api/v1/jobs/:id/cancel", jh.cancel, RequireAuth(s))

	jobLogH := newJobLogHandler(s)
	e.POST("/api/v1/jobs/:id/logs", jobLogH.append, RequireAuth(s))
	e.GET("/api/v1/jobs/:id/logs", jobLogH.getSince, RequireAuth(s))

	sslH := newSSLHandler(s, auditRec)
	e.GET("/api/v1/sites/:id/ssl", sslH.status, RequireAuth(s))
	e.POST("/api/v1/sites/:id/ssl/issue", sslH.issue, RequireAuth(s))
	e.POST("/api/v1/sites/:id/ssl/renew", sslH.renew, RequireAuth(s))
	e.POST("/api/v1/ssl/renew-all", sslH.renewAll, RequireAuth(s), RequireAdmin(s))

	// App process management (node/go/python runtimes)
	e.GET("/api/v1/sites/:id/app/status", sitesH.appStatus, RequireAuth(s))
	e.POST("/api/v1/sites/:id/app/start", sitesH.appStart, RequireAuth(s))
	e.POST("/api/v1/sites/:id/app/stop", sitesH.appStop, RequireAuth(s))
	e.POST("/api/v1/sites/:id/app/restart", sitesH.appRestart, RequireAuth(s))
	e.PUT("/api/v1/sites/:id/app/config", sitesH.appUpdateConfig, RequireAuth(s))
	e.GET("/api/v1/sites/:id/app/resource-usage", sitesH.appResourceUsage, RequireAuth(s))
	e.PUT("/api/v1/sites/:id/app/resource-limits", sitesH.appUpdateResourceLimits, RequireAuth(s), RequireAdmin(s))

	// PHP runtime config (per-site version + php.ini)
	e.GET("/api/v1/sites/:id/php/config", sitesH.getPHPConfig, RequireAuth(s))
	e.PUT("/api/v1/sites/:id/php/config", sitesH.updatePHPConfig, RequireAuth(s))

	// SSH info (read-only, displays connection details)
	e.GET("/api/v1/sites/:id/ssh/info", sitesH.sshInfo, RequireAuth(s))

	logsH := newLogsHandler(s)

	// Site management advanced tabs
	e.GET("/api/v1/sites/:id/vhost", sitesH.getVhost, RequireAuth(s))
	e.PUT("/api/v1/sites/:id/vhost", sitesH.updateVhost, RequireAuth(s))
	e.POST("/api/v1/sites/:id/vhost/validate", sitesH.validateVhost, RequireAuth(s))
	e.GET("/api/v1/sites/:id/varnish", sitesH.getVarnish, RequireAuth(s))
	e.POST("/api/v1/sites/:id/varnish/purge", sitesH.purgeVarnish, RequireAuth(s))
	e.GET("/api/v1/sites/:id/security", sitesH.getSecurity, RequireAuth(s))
	e.PUT("/api/v1/sites/:id/security", sitesH.updateSecurity, RequireAuth(s))

	// IP restrictions per site
	ipH := newIPRestrictionHandler(s)
	e.GET("/api/v1/sites/:id/ip-restrictions", ipH.list, RequireAuth(s))
	e.POST("/api/v1/sites/:id/ip-restrictions", ipH.create, RequireAuth(s))
	e.DELETE("/api/v1/sites/:id/ip-restrictions/:restriction_id", ipH.delete, RequireAuth(s))
	e.GET("/api/v1/sites/:id/ssh-ftp", sitesH.getSshFtp, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/sites/:id/ssh-ftp", sitesH.createSshFtp, RequireAuth(s), RequireAdmin(s))
	e.DELETE("/api/v1/sites/:id/ssh-ftp/:ssh_id", sitesH.deleteSshFtp, RequireAuth(s), RequireAdmin(s))
	e.GET("/api/v1/sites/:id/ssh-ftp/:ssh_id/key", sitesH.getSSHPrivateKey, RequireAuth(s), RequireAdmin(s))
	e.GET("/api/v1/sites/:id/cron", sitesH.listCron, RequireAuth(s))
	e.POST("/api/v1/sites/:id/cron", sitesH.createCron, RequireAuth(s), RequireAdmin(s))
	e.DELETE("/api/v1/sites/:id/cron/:cron_id", sitesH.deleteCron, RequireAuth(s), RequireAdmin(s))
	e.GET("/api/v1/sites/:id/logs", logsH.tail, RequireAuth(s))
	e.GET("/api/v1/sites/:id/usage", sitesH.usage, RequireAuth(s))

	// ─── File Manager ────────────────────────────────────────
	fmH := newFileManagerHandler(s, auditRec)
	e.GET("/api/v1/sites/:id/files", fmH.listFiles, RequireAuth(s))
	e.GET("/api/v1/sites/:id/files/read", fmH.readFile, RequireAuth(s))
	e.POST("/api/v1/sites/:id/files/write", fmH.writeFile, RequireAuth(s))
	e.POST("/api/v1/sites/:id/files/create", fmH.createEntry, RequireAuth(s))
	e.DELETE("/api/v1/sites/:id/files", fmH.deleteEntry, RequireAuth(s))
	e.POST("/api/v1/sites/:id/files/rename", fmH.renameEntry, RequireAuth(s))
	e.POST("/api/v1/sites/:id/files/move", fmH.moveEntries, RequireAuth(s))
	e.POST("/api/v1/sites/:id/files/compress", fmH.compressEntries, RequireAuth(s))
	e.POST("/api/v1/sites/:id/files/upload", fmH.uploadFile, RequireAuth(s))
	e.GET("/api/v1/sites/:id/files/download", fmH.downloadFile, RequireAuth(s))

	// ─── Backup Storage Providers ─────────────────────────
	bpH := newBackupProviderHandler(s, auditRec)
	e.GET("/api/v1/backup-providers", bpH.list, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/backup-providers", bpH.create, RequireAuth(s), RequireAdmin(s))
	e.PATCH("/api/v1/backup-providers/:id", bpH.update, RequireAuth(s), RequireAdmin(s))
	e.DELETE("/api/v1/backup-providers/:id", bpH.delete, RequireAuth(s), RequireAdmin(s))
	e.GET("/api/v1/backup-providers/:id/usage", bpH.usage, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/backup-providers/:id/test", bpH.testConnection, RequireAuth(s), RequireAdmin(s))

	backupH := newBackupHandler(s, auditRec)
	e.POST("/api/v1/backups/bulk", backupH.bulkCreate, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/sites/:id/backups", backupH.createWithProvider, RequireAuth(s))
	e.GET("/api/v1/sites/:id/backups", backupH.list, RequireAuth(s))
	e.POST("/api/v1/sites/:id/backups/database", backupH.createDatabaseBackup, RequireAuth(s))
	e.GET("/api/v1/backups", backupH.list, RequireAuth(s))
	e.GET("/api/v1/backups/:id/download", backupH.download, RequireAuth(s))
	e.DELETE("/api/v1/backups/:id", backupH.delete, RequireAuth(s))
	e.POST("/api/v1/backups/:id/restore", backupH.restore, RequireAuth(s))

	// ─── Backup Schedules (DEPRECATED) ────────────────────
	// Deprecated: use /api/v1/backup-configs instead.
	// These per-site schedule routes are kept for backward compatibility
	// and will be removed in a future release. Use backup-configs (global,
	// multi-site capable) as the replacement.
	bscH := newBackupScheduleHandler(s, auditRec)
	e.POST("/api/v1/sites/:id/schedules", bscH.create, RequireAuth(s))
	e.GET("/api/v1/sites/:id/schedules", bscH.listBySite, RequireAuth(s))
	e.PATCH("/api/v1/schedules/:id", bscH.update, RequireAuth(s))
	e.DELETE("/api/v1/schedules/:id", bscH.delete, RequireAuth(s))

	// ─── Backup Configs ────────────────────────────────────
	bcfgH := newBackupConfigHandler(s, auditRec)
	e.GET("/api/v1/backup-configs", bcfgH.list, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/backup-configs", bcfgH.create, RequireAuth(s), RequireAdmin(s))
	e.PATCH("/api/v1/backup-configs/:id", bcfgH.update, RequireAuth(s), RequireAdmin(s))
	e.DELETE("/api/v1/backup-configs/:id", bcfgH.delete, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/backup-configs/:id/run", bcfgH.run, RequireAuth(s), RequireAdmin(s))

	// ─── Retention Settings ─────────────────────────────────
	e.GET("/api/v1/settings/retention", getRetention(db), RequireAuth(s), RequireAdmin(s))
	e.PUT("/api/v1/settings/retention", setRetention(db), RequireAuth(s), RequireAdmin(s))

	metricH := newMetricHandler(s)
	// POST metrics — accepts both user sessions AND service tokens
	e.POST("/api/v1/servers/:id/metrics", metricH.ingest, RequireAuth(s))
	// GET metrics = user-facing dashboard
	e.GET("/api/v1/servers/:id/metrics/latest", metricH.latest, RequireAuth(s))
	e.GET("/api/v1/servers/:id/metrics", metricH.range_, RequireAuth(s))
	e.GET("/api/v1/servers/:id/gpu", metricH.gpu, RequireAuth(s))
	// Service Tokens (admin only)
	stH := newServiceTokenHandler(s)
	e.GET("/api/v1/service-tokens", stH.list, RequireAuth(s), RequireAdmin(s), RequireUserSession)
	e.POST("/api/v1/service-tokens", stH.create, RequireAuth(s), RequireAdmin(s), RequireUserSession)
	e.DELETE("/api/v1/service-tokens/:id", stH.revoke, RequireAuth(s), RequireAdmin(s), RequireUserSession)

	// Alert endpoints (admin only)
	alertH := newAlertHandler(s, auditRec)
	e.POST("/api/v1/alert-channels", alertH.createChannel, RequireAuth(s), RequireAdmin(s))
	e.GET("/api/v1/alert-channels", alertH.listChannels, RequireAuth(s), RequireAdmin(s))
	e.DELETE("/api/v1/alert-channels/:id", alertH.deleteChannel, RequireAuth(s), RequireAdmin(s))
	e.GET("/api/v1/alerts", alertH.listAlerts, RequireAuth(s), RequireAdmin(s))
	// In-app notifications (per-user)
	notifH := newNotificationHandler(s)
	e.GET("/api/v1/notifications", notifH.list, RequireAuth(s))
	e.GET("/api/v1/notifications/unread-count", notifH.count, RequireAuth(s))
	e.POST("/api/v1/notifications/:id/read", notifH.markRead, RequireAuth(s))
	e.POST("/api/v1/notifications/read-all", notifH.markAllRead, RequireAuth(s))
	e.DELETE("/api/v1/notifications/:id", notifH.delete, RequireAuth(s))
	e.DELETE("/api/v1/notifications/read", notifH.deleteAllRead, RequireAuth(s))

	dbH := newDBHandler(s, auditRec)
	e.GET("/api/v1/databases", dbH.listAllDatabases, RequireAuth(s))
	e.POST("/api/v1/sites/:id/databases", dbH.createDatabase, RequireAuth(s))
	e.GET("/api/v1/sites/:id/databases", dbH.listDatabases, RequireAuth(s))
	e.GET("/api/v1/databases/:id", dbH.getDatabase, RequireAuth(s))
	e.DELETE("/api/v1/databases/:id", dbH.deleteDatabase, RequireAuth(s))
	e.POST("/api/v1/databases/:id/users", dbH.createDBUser, RequireAuth(s))
	e.GET("/api/v1/databases/:id/users", dbH.listDBUsers, RequireAuth(s))
	e.DELETE("/api/v1/databases/:id/users/:user_id", dbH.deleteDBUser, RequireAuth(s))
	e.POST("/api/v1/databases/:id/users/:user_id/rotate-password", dbH.rotateDBUserPassword, RequireAuth(s))
	e.PATCH("/api/v1/databases/:id", dbH.updateDatabase, RequireAuth(s))

	// Grant existing user to database
	e.POST("/api/v1/databases/:id/grant-user", dbH.grantExistingUser, RequireAuth(s))
	e.GET("/api/v1/databases/:id/available-users", dbH.listAvailableMySQLUsers, RequireAuth(s))

	// Global database users management
	e.GET("/api/v1/db-users", dbH.listAllDBUsers, RequireAuth(s))
	e.PATCH("/api/v1/db-users/:id", dbH.updateDBUser, RequireAuth(s))

	// phpMyAdmin credentials (internal only — localhost calls for auto-login)
	e.GET("/api/v1/databases/:id/pma-credentials", dbH.pmaCredentials, RequireAuth(s))
	e.POST("/api/v1/databases/exec-sql", dbH.execSQL, RequireAuth(s), RequireAdmin(s))

	// Reset passwords for imported users (admin only)
	e.POST("/api/v1/databases/phpmyadmin-reset-passwords", dbH.phpMyAdminResetPasswords, RequireAuth(s), RequireAdmin(s))

	// Database server management
	dbServerH := newDBServerHandler(s, auditRec)
	e.GET("/api/v1/database-server", dbServerH.getDBServer, RequireAuth(s))
	e.POST("/api/v1/database-server/import", dbServerH.importDatabases, RequireAuth(s))

	// Audit log endpoint (admin only)
	auditH := audit.NewHandler(s.AuditLogs, RequireAdmin(s))
	auditGroup := e.Group("/api/v1", RequireAuth(s))
	auditH.Register(auditGroup)

	// ─── Agent Registration ────────────────────────────────
	agH := newAgentHandler(s, auditRec)
	e.POST("/api/v1/agents/register", agH.registerAgent)
	e.GET("/api/v1/agents/install", agH.installScript)
	e.GET("/api/v1/agents/status-dashboard", agH.statusDashboard)
	e.POST("/api/v1/agents/tokens", agH.createRegistrationToken, RequireAuth(s), RequireAdmin(s))

	// ─── Cloudflare Integration ──────────────────────────────
	cfH := newCloudflareHandler(s, auditRec)
	cf := e.Group("/api/v1/cloudflare", RequireAuth(s))
	cf.GET("/config", cfH.getConfig)
	cf.PUT("/config", cfH.saveConfig)
	cf.DELETE("/config", cfH.deleteConfig)
	cf.GET("/verify", cfH.verify)
	cf.GET("/zones", cfH.listZones)
	cf.GET("/zones/search", cfH.searchZones)
	cf.GET("/zones/:id", cfH.getZone)
	cf.GET("/zones/:id/dns", cfH.listDNSRecords)
	cf.POST("/zones/:id/dns", cfH.createDNSRecord)
	cf.DELETE("/zones/:id/dns/:recordId", cfH.deleteDNSRecord)
	cf.PUT("/zones/:id/dns/:recordId", cfH.updateDNSRecord)
	cf.PATCH("/zones/:id/dns/:recordId", cfH.patchDNSRecord)
	cf.POST("/zones/:id/dns/bulk-delete", cfH.bulkDeleteDNS)
	cf.POST("/zones/:id/dns/import", cfH.importDNSRecords)
	cf.POST("/zones/:id/purge-cache", cfH.purgeCache)
	cf.GET("/zones/:id/analytics", cfH.getAnalytics)
	cf.GET("/zones/:id/analytics/range", cfH.getAnalyticsRange)

	// ─── License & Feature Gating ───────────────────────────────
	licH := newLicenseHandler(s)
	e.GET("/api/v1/license", licH.getLicense, RequireAuth(s))
	e.POST("/api/v1/license/activate", licH.activateLicense, RequireAuth(s), RequireAdmin(s))
	e.DELETE("/api/v1/license", licH.deactivateLicense, RequireAuth(s), RequireAdmin(s))
	e.GET("/api/v1/license/features", licH.getFeatures, RequireAuth(s))

	// ─── Stack Management (install/manage software on servers) ──
	stackH := newStackHandler(s, auditRec)
	e.GET("/api/v1/servers/:id/stack", stackH.stackInfo, RequireAuth(s))
	e.POST("/api/v1/servers/:id/stack/action", stackH.stackAction, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/servers/:id/stack/restart", stackH.stackRestart, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/servers/:id/command", stackH.commandExec, RequireAuth(s), RequireAdmin(s))

	// ─── Docker Container Management ──
	e.GET("/api/v1/servers/:id/docker/containers", stackH.dockerContainers, RequireAuth(s))
	e.POST("/api/v1/servers/:id/docker/container/start", stackH.dockerContainerStart, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/servers/:id/docker/container/stop", stackH.dockerContainerStop, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/servers/:id/docker/container/restart", stackH.dockerContainerRestart, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/servers/:id/docker/container/remove", stackH.dockerContainerRemove, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/servers/:id/docker/container/create", stackH.dockerContainerCreate, RequireAuth(s), RequireAdmin(s))
	e.GET("/api/v1/servers/:id/docker/container/logs", stackH.dockerContainerLogs, RequireAuth(s))
	e.POST("/api/v1/servers/:id/docker/container/exec", stackH.dockerContainerExec, RequireAuth(s), RequireAdmin(s))
	e.GET("/api/v1/servers/:id/docker/images", stackH.dockerImages, RequireAuth(s))
	e.GET("/api/v1/servers/:id/docker/images/search", stackH.dockerSearchImages, RequireAuth(s))
	e.POST("/api/v1/servers/:id/docker/compose/up", stackH.dockerComposeUp, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/servers/:id/docker/compose/down", stackH.dockerComposeDown, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/servers/:id/docker/image/pull", stackH.dockerImagePull, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/servers/:id/docker/image/remove", stackH.dockerImageRemove, RequireAuth(s), RequireAdmin(s))
	e.GET("/api/v1/servers/:id/docker/volumes", stackH.dockerVolumes, RequireAuth(s))
	e.POST("/api/v1/servers/:id/docker/volume/create", stackH.dockerVolumeCreate, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/servers/:id/docker/volume/remove", stackH.dockerVolumeRemove, RequireAuth(s), RequireAdmin(s))
	e.GET("/api/v1/servers/:id/docker/networks", stackH.dockerNetworks, RequireAuth(s))
	e.POST("/api/v1/servers/:id/docker/network/create", stackH.dockerNetworkCreate, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/servers/:id/docker/network/remove", stackH.dockerNetworkRemove, RequireAuth(s), RequireAdmin(s))
	e.GET("/api/v1/servers/:id/docker/info", stackH.dockerInfo, RequireAuth(s))
	e.GET("/api/v1/servers/:id/docker/disk-usage", stackH.dockerDiskUsage, RequireAuth(s))

	// ─── Git Deploy v2 ──
	e.GET("/api/v1/sites/:id/git/branches", sitesH.gitBranches, RequireAuth(s))
	e.GET("/api/v1/sites/:id/git/log", sitesH.gitLog, RequireAuth(s))
	e.POST("/api/v1/sites/:id/git/rollback", sitesH.gitRollback, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/sites/:id/git/checkout", sitesH.gitCheckout, RequireAuth(s), RequireAdmin(s))

	// ─── Firewall (UFW) Management ──
	e.GET("/api/v1/servers/:id/firewall/status", stackH.firewallStatus, RequireAuth(s))
	e.POST("/api/v1/servers/:id/firewall/enable", stackH.firewallEnable, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/servers/:id/firewall/disable", stackH.firewallDisable, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/servers/:id/firewall/allow", stackH.firewallAllow, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/servers/:id/firewall/deny", stackH.firewallDeny, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/servers/:id/firewall/delete", stackH.firewallDelete, RequireAuth(s), RequireAdmin(s))
	e.GET("/api/v1/servers/:id/firewall/rules", stackH.firewallRules, RequireAuth(s))
	e.POST("/api/v1/servers/:id/firewall/reset", stackH.firewallReset, RequireAuth(s), RequireAdmin(s))

	// ─── PM2 Process Management ──
	e.POST("/api/v1/servers/:id/pm2/manage", stackH.pm2Manage, RequireAuth(s), RequireAdmin(s))

	// ─── Redis Management ──
	e.GET("/api/v1/servers/:id/redis/stats", stackH.redisStats, RequireAuth(s))
	e.POST("/api/v1/servers/:id/redis/flush", stackH.redisFlush, RequireAuth(s), RequireAdmin(s))
	e.GET("/api/v1/servers/:id/redis/info", stackH.redisInfo, RequireAuth(s))
	e.POST("/api/v1/servers/:id/redis/flushdb", stackH.redisFlushDB, RequireAuth(s), RequireAdmin(s))

	// ─── Fail2ban Management ──
	e.GET("/api/v1/servers/:id/fail2ban/status", stackH.fail2banStatus, RequireAuth(s))
	e.GET("/api/v1/servers/:id/security/overview", stackH.securityOverview, RequireAuth(s))
	e.GET("/api/v1/servers/:id/fail2ban/jail", stackH.fail2banJail, RequireAuth(s))
	e.POST("/api/v1/servers/:id/fail2ban/unban", stackH.fail2banUnban, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/servers/:id/fail2ban/ban", stackH.fail2banBan, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/servers/:id/fail2ban/enable", stackH.fail2banEnable, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/servers/:id/fail2ban/disable", stackH.fail2banDisable, RequireAuth(s), RequireAdmin(s))

	// ─── Bot Blocker Management ──
	e.POST("/api/v1/servers/:id/bot-blocker/manage", stackH.botBlockerManage, RequireAuth(s), RequireAdmin(s))

	// ─── Server Specs & Benchmarks ─────────────────────────────
	e.GET("/api/v1/servers/:id/specs", stackH.serverSpecs, RequireAuth(s))
	e.GET("/api/v1/servers/:id/benchmark", stackH.serverBenchmark, RequireAuth(s), RequireAdmin(s))

	// ─── Plans ─────────────────────────────────────────────────
	planH := newPlanHandler(s)
	e.GET("/api/v1/plans", planH.listPlans, RequireAuth(s))
	e.POST("/api/v1/plans", planH.createPlan, RequireAuth(s), RequireAdmin(s))
	e.GET("/api/v1/plans/:id", planH.getPlan, RequireAuth(s))
	e.PATCH("/api/v1/plans/:id", planH.updatePlan, RequireAuth(s), RequireAdmin(s))
	e.DELETE("/api/v1/plans/:id", planH.deletePlan, RequireAuth(s), RequireAdmin(s))

	// ─── License Orders ────────────────────────────────────────
	orderH := newOrderHandler(s)
	e.GET("/api/v1/orders", orderH.listOrders, RequireAuth(s))
	e.POST("/api/v1/orders", orderH.createOrder, RequireAuth(s))
	e.GET("/api/v1/orders/:id", orderH.getOrder, RequireAuth(s))
	e.PATCH("/api/v1/orders/:id", orderH.updateOrderStatus, RequireAuth(s), RequireAdmin(s))

	// ─── Assign Plan to User ──────────────────────────────────
	e.POST("/api/v1/users/:id/assign-plan", uh.assignPlan, RequireAuth(s), RequireAdmin(s))

	// Example: feature-gated route (uncomment when ready to gate specific endpoints)
	// e.GET("/api/v1/servers", sh.list, RequireAuth(s), RequireFeature(s, "multi_server"))

	return e
}

func customErrorHandler(err error, c echo.Context) {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	if c.Response().Committed {
		return
	}

	he, ok := err.(*echo.HTTPError)
	if !ok {
		he = &echo.HTTPError{
			Code:     http.StatusInternalServerError,
			Message:  "internal error",
			Internal: err,
		}
	}

	// Convert to our format
	httpjson.Error(c.Response(), he.Code, rid, errorCodeFromStatus(he.Code), fmt.Sprintf("%v", he.Message))
}

func healthHandler(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"status":  "ok",
			"service": "sigardapanel-api",
			"time":    time.Now().UTC().Format(time.RFC3339),
		},
		RequestID: rid,
	})
}

func versionHandler(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"service": "sigardapanel-api",
			"version": version,
		},
		RequestID: rid,
	})
}

func notImplementedHandler(action string) echo.HandlerFunc {
	return func(c echo.Context) error {
		rid := c.Response().Header().Get(echo.HeaderXRequestID)
		return c.JSON(http.StatusNotImplemented, httpjson.ErrorBody{
			Error: httpjson.APIError{
				Code:    "not_implemented",
				Message: action + " not implemented yet",
			},
			RequestID: rid,
		})
	}
}

func errorCodeFromStatus(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return "unauthorized"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusBadRequest:
		return "bad_request"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusTooManyRequests:
		return "rate_limited"
	case http.StatusNotImplemented:
		return "not_implemented"
	default:
		return "internal_error"
	}
}
