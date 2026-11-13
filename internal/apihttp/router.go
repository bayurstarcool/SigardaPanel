package apihttp

import (
	"errors"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"sigardapanel/internal/audit"
	"sigardapanel/internal/auth"
	"sigardapanel/internal/buildinfo"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/member"
	"sigardapanel/internal/store"
)

// NewRouter returns a new Echo router wired with the given store and database.
func NewRouter(s *store.Store, db *sql.DB, memberDB *sql.DB) *echo.Echo {
	e := echo.New()
	// Only trust X-Real-IP when it comes from the local nginx proxy; a client
	// must not be able to spoof its IP via X-Forwarded-For to dodge rate limits
	// or poison audit logs.
	e.IPExtractor = echo.ExtractIPFromRealIPHeader(
		echo.TrustLoopback(true),
		echo.TrustLinkLocal(false),
		echo.TrustPrivateNet(false),
	)
	// A panic in any handler must turn into a 500 for that request, not take
	// down the API process (and the job worker and scheduler running in it).
	// The panic comes back as *middleware.PanicStackError; customErrorHandler
	// logs it with the stack and answers 500.
	e.Use(middleware.Recover())
	e.Use(middleware.BodyLimit(32 << 20))

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

	// Cookie-authenticated state changes must come from this site.
	e.Use(cookieCSRFGuard)

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
	// Public, unauthenticated switches the web needs before anyone signs in:
	// whether the /register page can offer a sign-up form (member sign-up
	// needs the member database AND SIGARDAPANEL_ALLOW_MEMBER_SIGNUP) and
	// whether / and /pricing show the SaaS landing (SIGARDAPANEL_LANDING_PAGE)
	// or redirect to the login. Same body on both paths.
	publicConfig := func(c *echo.Context) error {
		c.Response().Header().Set("Cache-Control", "no-store")
		return c.JSON(http.StatusOK, httpjson.Success{
			Data:      publicConfigData(memberDB != nil),
			RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
		})
	}
	e.GET("/api/v1/auth/signup", publicConfig)
	e.GET("/api/v1/public/config", publicConfig)

	// Two-Factor Authentication (2FA) endpoints
	tfaH := newTwoFactorHandler(s)
	e.POST("/api/v1/auth/2fa/setup", tfaH.setup2FA, RequireAuth(s), RequireUserSession, rejectImpersonation)
	e.POST("/api/v1/auth/2fa/enable", tfaH.enable2FA, RequireAuth(s), RequireUserSession, rejectImpersonation)
	e.POST("/api/v1/auth/2fa/disable", tfaH.disable2FA, RequireAuth(s), RequireUserSession, rejectImpersonation)
	e.POST("/api/v1/auth/2fa/verify", tfaH.verify2FA, RequireAuth(s))
	e.GET("/api/v1/auth/2fa/status", tfaH.status2FA, RequireAuth(s))
	e.POST("/api/v1/auth/2fa/recovery-codes", tfaH.generateRecoveryCodes, RequireAuth(s), RequireUserSession, rejectImpersonation)

	// Member routes (separate database)
	if memberDB != nil {
		member.RegisterRoutes(e, memberDB)
	}

	e.GET("/api/v1/auth/me", ah.me, RequireAuth(s))
	// Email/password changes: the account holder's own interactive session
	// only (not an API key, not an impersonating admin).
	e.PUT("/api/v1/auth/profile", ah.updateProfile, RequireAuth(s), RequireUserSession, rejectImpersonation)
	// Logout revokes the session row by ID, so service tokens and API keys
	// (which carry no session row of their own) are rejected.
	e.POST("/api/v1/auth/logout", ah.logout, RequireAuth(s), RequireUserSession)
	e.POST("/api/v1/auth/stop-impersonation", ah.stopImpersonation, RequireAuth(s), RequireUserSession)

	// User management (admin only). Real user sessions only: a service token
	// carries no user, so it must not be able to manage (super) admins.
	e.GET("/api/v1/users", uh.list, RequireAuth(s), RequireUserSession, RequireAdmin(s))
	e.POST("/api/v1/users", uh.create, RequireAuth(s), RequireUserSession, RequireAdmin(s))
	e.DELETE("/api/v1/users/:id", uh.delete, RequireAuth(s), RequireUserSession, RequireAdmin(s))
	e.PATCH("/api/v1/users/:id", uh.update, RequireAuth(s), RequireUserSession, RequireAdmin(s))

	// User usage/quota (authenticated users can check their own)
	e.GET("/api/v1/users/:id/usage", uh.usage, RequireAuth(s), RequireUserSession)

	// User ↔ Site access mappings (admin only; assign/unassign/permission
	// changes additionally require site owner or super_admin)
	e.GET("/api/v1/users/:id/sites", uh.listUserSites, RequireAuth(s), RequireUserSession, RequireAdmin(s))
	e.POST("/api/v1/users/:id/sites", uh.assignUserSite, RequireAuth(s), RequireUserSession, RequireAdmin(s))
	e.DELETE("/api/v1/users/:id/sites/:siteId", uh.unassignUserSite, RequireAuth(s), RequireUserSession, RequireAdmin(s))
	e.PATCH("/api/v1/users/:id/sites/:siteId", uh.updateSitePermission, RequireAuth(s), RequireUserSession, RequireAdmin(s))
	e.GET("/api/v1/sites/:id/users", uh.listSiteUsers, RequireAuth(s), RequireUserSession, RequireAdmin(s))

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
	termH := newTerminalHandler(s, auditRec, sitesH)
	// Web terminal: ticket (authenticated) -> websocket (ticket-authenticated)
	e.POST("/api/v1/terminal/tickets", termH.createTicket, RequireAuth(s), RequireUserSession)
	e.GET("/api/v1/ws/terminal", termH.serve)
	e.GET("/api/v1/sites", sitesH.list, RequireAuth(s))
	e.POST("/api/v1/sites", sitesH.create, RequireAuth(s))
	e.GET("/api/v1/sites/:id", sitesH.get, RequireAuth(s))
	e.DELETE("/api/v1/sites/:id", sitesH.delete, RequireAuth(s))
	e.POST("/api/v1/sites/:id/deploy", sitesH.deploy, RequireAuth(s))
	e.POST("/api/v1/sites/:id/config", sitesH.updateConfig, RequireAuth(s))
	e.GET("/api/v1/sites/:id/webhook", sitesH.webhookSecret, RequireAuth(s))
	e.POST("/api/v1/sites/:id/webhook/rotate", sitesH.webhookSecret, RequireAuth(s))
	e.POST("/api/v1/sites/:id/reload-nginx", sitesH.reloadNginx, RequireAuth(s))
	e.POST("/api/v1/sites/:id/suspend", sitesH.suspend, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/sites/:id/resume", sitesH.resume, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/sites/:id/transfer", sitesH.transfer, RequireAuth(s))

	// Public webhook endpoints (no auth)
	wh := newWebhookHandler(s)
	e.POST("/api/v1/webhooks/deploy/:site_id", wh.deploy)

	jh := newJobHandler(s, auditRec)
	e.GET("/api/v1/jobs", jh.list, RequireAuth(s))
	e.GET("/api/v1/jobs/:id", jh.get, RequireAuth(s))
	e.POST("/api/v1/jobs/:id/cancel", jh.cancel, RequireAuth(s))

	jobLogH := newJobLogHandler(s)
	// Appending log lines is for the worker side (admin-scoped service
	// tokens) and admins only.
	e.POST("/api/v1/jobs/:id/logs", jobLogH.append, RequireAuth(s), RequireAdmin(s))
	e.GET("/api/v1/jobs/:id/logs", jobLogH.getSince, RequireAuth(s))

	sslH := newSSLHandler(s, auditRec)
	e.GET("/api/v1/sites/:id/ssl", sslH.status, RequireAuth(s))
	e.POST("/api/v1/sites/:id/ssl/issue", sslH.issue, RequireAuth(s))
	e.POST("/api/v1/sites/:id/ssl/renew", sslH.renew, RequireAuth(s))
	e.POST("/api/v1/ssl/renew-all", sslH.renewAll, RequireAuth(s), RequireUserSession, RequireRole(s, auth.RoleSuperAdmin)) // acts on every tenant's sites

	// App process management (node/go/python runtimes)
	e.GET("/api/v1/sites/:id/app/status", sitesH.appStatus, RequireAuth(s))
	e.POST("/api/v1/sites/:id/app/start", sitesH.appStart, RequireAuth(s))
	e.POST("/api/v1/sites/:id/app/stop", sitesH.appStop, RequireAuth(s))
	e.POST("/api/v1/sites/:id/app/restart", sitesH.appRestart, RequireAuth(s))
	e.PUT("/api/v1/sites/:id/app/config", sitesH.appUpdateConfig, RequireAuth(s))
	e.GET("/api/v1/sites/:id/app/resource-usage", sitesH.appResourceUsage, RequireAuth(s))
	e.PUT("/api/v1/sites/:id/app/resource-limits", sitesH.appUpdateResourceLimits, RequireAuth(s))

	// PHP runtime config (per-site version + php.ini)
	e.GET("/api/v1/sites/:id/php/config", sitesH.getPHPConfig, RequireAuth(s))
	e.PUT("/api/v1/sites/:id/php/config", sitesH.updatePHPConfig, RequireAuth(s))

	// SSH info (read-only, displays connection details)
	e.GET("/api/v1/sites/:id/ssh/info", sitesH.sshInfo, RequireAuth(s))

	logsH := newLogsHandler(s)

	// Site management advanced tabs
	e.GET("/api/v1/sites/:id/vhost", sitesH.getVhost, RequireAuth(s))
	// Raw vhost editing can read/write arbitrary files via nginx directives, so it is admin-only.
	e.PUT("/api/v1/sites/:id/vhost", sitesH.updateVhost, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/sites/:id/vhost/validate", sitesH.validateVhost, RequireAuth(s), RequireAdmin(s))
	e.GET("/api/v1/sites/:id/varnish", sitesH.getVarnish, RequireAuth(s))
	e.POST("/api/v1/sites/:id/varnish/purge", sitesH.purgeVarnish, RequireAuth(s))
	e.GET("/api/v1/sites/:id/security", sitesH.getSecurity, RequireAuth(s))
	e.PUT("/api/v1/sites/:id/security", sitesH.updateSecurity, RequireAuth(s))

	// IP restrictions per site
	ipH := newIPRestrictionHandler(s)
	e.GET("/api/v1/sites/:id/ip-restrictions", ipH.list, RequireAuth(s))
	e.POST("/api/v1/sites/:id/ip-restrictions", ipH.create, RequireAuth(s))
	e.DELETE("/api/v1/sites/:id/ip-restrictions/:restriction_id", ipH.delete, RequireAuth(s))
	// SSH/FTP accounts: the handlers require read (list) or write permission
	// on the site, so owners and site writers manage their own accounts.
	e.GET("/api/v1/sites/:id/ssh-ftp", sitesH.getSshFtp, RequireAuth(s))
	e.POST("/api/v1/sites/:id/ssh-ftp", sitesH.createSshFtp, RequireAuth(s))
	e.DELETE("/api/v1/sites/:id/ssh-ftp/:ssh_id", sitesH.deleteSshFtp, RequireAuth(s))
	e.GET("/api/v1/sites/:id/ssh-ftp/:ssh_id/key", sitesH.getSSHPrivateKey, RequireAuth(s))
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
	e.POST("/api/v1/sites/:id/files/extract", fmH.extractArchive, RequireAuth(s))
	e.POST("/api/v1/sites/:id/files/chmod", fmH.chmodEntry, RequireAuth(s))
	e.POST("/api/v1/sites/:id/files/upload", fmH.uploadFile, RequireAuth(s))
	e.GET("/api/v1/sites/:id/files/download", fmH.downloadFile, RequireAuth(s))

	// ─── Backup Storage Providers ─────────────────────────
	// Providers are global (the default one receives every tenant's
	// backups): only a super_admin may change them.
	bpH := newBackupProviderHandler(s, auditRec)
	e.GET("/api/v1/backup-providers", bpH.list, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/backup-providers", bpH.create, RequireAuth(s), RequireUserSession, RequireRole(s, auth.RoleSuperAdmin))
	e.PATCH("/api/v1/backup-providers/:id", bpH.update, RequireAuth(s), RequireUserSession, RequireRole(s, auth.RoleSuperAdmin))
	e.DELETE("/api/v1/backup-providers/:id", bpH.delete, RequireAuth(s), RequireUserSession, RequireRole(s, auth.RoleSuperAdmin))
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
	e.GET("/api/v1/backup-configs", bcfgH.list, RequireAuth(s), RequireUserSession, RequireRole(s, auth.RoleSuperAdmin))
	e.POST("/api/v1/backup-configs", bcfgH.create, RequireAuth(s), RequireUserSession, RequireRole(s, auth.RoleSuperAdmin))
	e.PATCH("/api/v1/backup-configs/:id", bcfgH.update, RequireAuth(s), RequireUserSession, RequireRole(s, auth.RoleSuperAdmin))
	e.DELETE("/api/v1/backup-configs/:id", bcfgH.delete, RequireAuth(s), RequireUserSession, RequireRole(s, auth.RoleSuperAdmin))
	e.POST("/api/v1/backup-configs/:id/run", bcfgH.run, RequireAuth(s), RequireUserSession, RequireRole(s, auth.RoleSuperAdmin))

	// ─── Retention Settings ─────────────────────────────────
	// Panel self-update (super_admin, interactive session)
	puH := &panelUpdateHandler{store: s, db: db, audit: auditRec}
	e.GET("/api/v1/system/panel-update", puH.status, RequireAuth(s), RequireUserSession, RequireRole(s, auth.RoleSuperAdmin))
	e.POST("/api/v1/system/panel-update", puH.start, RequireAuth(s), RequireUserSession, RequireRole(s, auth.RoleSuperAdmin))
	e.GET("/api/v1/system/panel-update/summary", puH.summary, RequireAuth(s), RequireUserSession, RequireRole(s, auth.RoleSuperAdmin))
	e.GET("/api/v1/system/panel-update/backups", puH.backups, RequireAuth(s), RequireUserSession, RequireRole(s, auth.RoleSuperAdmin))
	e.POST("/api/v1/system/panel-update/rollback", puH.rollback, RequireAuth(s), RequireUserSession, RequireRole(s, auth.RoleSuperAdmin))
	// Remote agent updates (super_admin, interactive session)
	auH := &agentUpdateHandler{store: s, db: db, audit: auditRec}
	e.GET("/api/v1/system/agent-updates", auH.list, RequireAuth(s), RequireUserSession, RequireRole(s, auth.RoleSuperAdmin))
	e.POST("/api/v1/system/agent-updates", auH.startMany, RequireAuth(s), RequireUserSession, RequireRole(s, auth.RoleSuperAdmin))
	e.PUT("/api/v1/system/agent-updates/settings", auH.settings, RequireAuth(s), RequireUserSession, RequireRole(s, auth.RoleSuperAdmin))
	e.POST("/api/v1/servers/:id/agent-update", auH.startOne, RequireAuth(s), RequireUserSession, RequireRole(s, auth.RoleSuperAdmin))
	e.PUT("/api/v1/system/panel-update/settings", puH.settings, RequireAuth(s), RequireUserSession, RequireRole(s, auth.RoleSuperAdmin))

	e.GET("/api/v1/settings/retention", getRetention(db), RequireAuth(s), RequireAdmin(s))
	// Global retention deletes every tenant's backups: super_admin only.
	e.PUT("/api/v1/settings/retention", setRetention(db), RequireAuth(s), RequireUserSession, RequireRole(s, auth.RoleSuperAdmin))

	metricH := newMetricHandler(s)
	// POST metrics — admins and admin-scoped service tokens only
	e.POST("/api/v1/servers/:id/metrics", metricH.ingest, RequireAuth(s), RequireAdmin(s))
	// GET metrics = user-facing dashboard
	e.GET("/api/v1/servers/:id/metrics/latest", metricH.latest, RequireAuth(s))
	e.GET("/api/v1/servers/:id/metrics", metricH.range_, RequireAuth(s))
	e.GET("/api/v1/servers/:id/gpu", metricH.gpu, RequireAuth(s))
	// Service Tokens (admin only)
	stH := newServiceTokenHandler(s)
	e.GET("/api/v1/service-tokens", stH.list, RequireAuth(s), RequireUserSession, RequireRole(s, auth.RoleSuperAdmin))
	e.POST("/api/v1/service-tokens", stH.create, RequireAuth(s), RequireUserSession, RequireRole(s, auth.RoleSuperAdmin))
	e.DELETE("/api/v1/service-tokens/:id", stH.revoke, RequireAuth(s), RequireUserSession, RequireRole(s, auth.RoleSuperAdmin))

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
	e.GET("/api/v1/databases/:id/viewer/tables", dbH.viewerTables, RequireAuth(s))
	e.GET("/api/v1/databases/:id/viewer/columns", dbH.viewerColumns, RequireAuth(s))
	e.GET("/api/v1/databases/:id/viewer/rows", dbH.viewerRows, RequireAuth(s))
	e.POST("/api/v1/databases/exec-sql", dbH.execSQL, RequireAuth(s), RequireUserSession, RequireRole(s, auth.RoleSuperAdmin))

	// Reset passwords for imported users (admin only)
	e.POST("/api/v1/databases/phpmyadmin-reset-passwords", dbH.phpMyAdminResetPasswords, RequireAuth(s), RequireUserSession, RequireRole(s, auth.RoleSuperAdmin))

	// Database server management
	dbServerH := newDBServerHandler(s, auditRec)
	// Server-wide MySQL management: super_admin only.
	e.GET("/api/v1/database-server", dbServerH.getDBServer, RequireAuth(s), RequireRole(s, auth.RoleSuperAdmin))
	e.POST("/api/v1/database-server/import", dbServerH.importDatabases, RequireAuth(s), RequireRole(s, auth.RoleSuperAdmin))

	// Audit log endpoint (admin only)
	auditH := audit.NewHandler(s.AuditLogs, scopeAuditLogs(s))
	auditGroup := e.Group("/api/v1", RequireAuth(s))
	auditH.Register(auditGroup)
	// Purging old audit entries is global and destructive: super_admin with
	// an interactive session only.
	e.DELETE("/api/v1/audit-logs", purgeAuditLogs(s, auditRec), RequireAuth(s), RequireUserSession, RequireRole(s, auth.RoleSuperAdmin))

	// ─── Personal API keys (scoped to the calling user) ─────
	// Managed from interactive sessions only, so a leaked key cannot mint
	// more keys or revoke the owner's others.
	akH := newAPIKeyHandler(s, auditRec)
	e.GET("/api/v1/api-keys", akH.list, RequireAuth(s), RequireUserSession)
	e.POST("/api/v1/api-keys", akH.create, RequireAuth(s), RequireUserSession)
	e.DELETE("/api/v1/api-keys/:id", akH.revoke, RequireAuth(s), RequireUserSession)

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
	e.GET("/api/v1/servers/:id/stack/ops/:op", stackH.stackOp, RequireAuth(s), RequireAdmin(s))
	e.GET("/api/v1/servers/:id/php-versions", stackH.phpVersions, RequireAuth(s))
	e.POST("/api/v1/servers/:id/command", stackH.commandExec, RequireAuth(s), RequireAdmin(s))

	// ─── Docker Container Management ──
	e.GET("/api/v1/servers/:id/docker/containers", stackH.dockerContainers, RequireAuth(s))
	e.POST("/api/v1/servers/:id/docker/container/start", stackH.dockerContainerStart, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/servers/:id/docker/container/stop", stackH.dockerContainerStop, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/servers/:id/docker/container/restart", stackH.dockerContainerRestart, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/servers/:id/docker/container/remove", stackH.dockerContainerRemove, RequireAuth(s), RequireAdmin(s), requireSuperAdminOnPanelHost(s), auditServerAction(auditRec))
	e.POST("/api/v1/servers/:id/docker/container/create", stackH.dockerContainerCreate, RequireAuth(s), RequireAdmin(s), requireSuperAdminOnPanelHost(s), auditServerAction(auditRec))
	e.GET("/api/v1/servers/:id/docker/container/logs", stackH.dockerContainerLogs, RequireAuth(s))
	e.POST("/api/v1/servers/:id/docker/container/exec", stackH.dockerContainerExec, RequireAuth(s), RequireAdmin(s), requireSuperAdminOnPanelHost(s), auditServerAction(auditRec))
	e.GET("/api/v1/servers/:id/docker/images", stackH.dockerImages, RequireAuth(s), RequireAdmin(s))
	e.GET("/api/v1/servers/:id/docker/images/search", stackH.dockerSearchImages, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/servers/:id/docker/compose/up", stackH.dockerComposeUp, RequireAuth(s), RequireAdmin(s), requireSuperAdminOnPanelHost(s), auditServerAction(auditRec))
	e.POST("/api/v1/servers/:id/docker/compose/down", stackH.dockerComposeDown, RequireAuth(s), RequireAdmin(s), requireSuperAdminOnPanelHost(s), auditServerAction(auditRec))
	e.POST("/api/v1/servers/:id/docker/image/pull", stackH.dockerImagePull, RequireAuth(s), RequireAdmin(s), requireSuperAdminOnPanelHost(s), auditServerAction(auditRec))
	e.POST("/api/v1/servers/:id/docker/image/remove", stackH.dockerImageRemove, RequireAuth(s), RequireAdmin(s), requireSuperAdminOnPanelHost(s), auditServerAction(auditRec))
	e.GET("/api/v1/servers/:id/docker/volumes", stackH.dockerVolumes, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/servers/:id/docker/volume/create", stackH.dockerVolumeCreate, RequireAuth(s), RequireAdmin(s), requireSuperAdminOnPanelHost(s), auditServerAction(auditRec))
	e.POST("/api/v1/servers/:id/docker/volume/remove", stackH.dockerVolumeRemove, RequireAuth(s), RequireAdmin(s), requireSuperAdminOnPanelHost(s), auditServerAction(auditRec))
	e.GET("/api/v1/servers/:id/docker/networks", stackH.dockerNetworks, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/servers/:id/docker/network/create", stackH.dockerNetworkCreate, RequireAuth(s), RequireAdmin(s), requireSuperAdminOnPanelHost(s), auditServerAction(auditRec))
	e.POST("/api/v1/servers/:id/docker/network/remove", stackH.dockerNetworkRemove, RequireAuth(s), RequireAdmin(s), requireSuperAdminOnPanelHost(s), auditServerAction(auditRec))
	e.GET("/api/v1/servers/:id/docker/info", stackH.dockerInfo, RequireAuth(s), RequireAdmin(s))
	e.GET("/api/v1/servers/:id/docker/disk-usage", stackH.dockerDiskUsage, RequireAuth(s), RequireAdmin(s))

	// ─── Git Deploy v2 ──
	e.GET("/api/v1/sites/:id/git/branches", sitesH.gitBranches, RequireAuth(s))
	e.GET("/api/v1/sites/:id/git/log", sitesH.gitLog, RequireAuth(s))
	e.POST("/api/v1/sites/:id/git/rollback", sitesH.gitRollback, RequireAuth(s))
	e.POST("/api/v1/sites/:id/git/checkout", sitesH.gitCheckout, RequireAuth(s))

	// ─── Firewall (UFW) Management ──
	e.GET("/api/v1/servers/:id/firewall/status", stackH.firewallStatus, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/servers/:id/firewall/enable", stackH.firewallEnable, RequireAuth(s), RequireAdmin(s), requireSuperAdminOnPanelHost(s), auditServerAction(auditRec))
	e.POST("/api/v1/servers/:id/firewall/disable", stackH.firewallDisable, RequireAuth(s), RequireAdmin(s), requireSuperAdminOnPanelHost(s), auditServerAction(auditRec))
	e.POST("/api/v1/servers/:id/firewall/allow", stackH.firewallAllow, RequireAuth(s), RequireAdmin(s), requireSuperAdminOnPanelHost(s), auditServerAction(auditRec))
	e.POST("/api/v1/servers/:id/firewall/deny", stackH.firewallDeny, RequireAuth(s), RequireAdmin(s), requireSuperAdminOnPanelHost(s), auditServerAction(auditRec))
	e.POST("/api/v1/servers/:id/firewall/delete", stackH.firewallDelete, RequireAuth(s), RequireAdmin(s), requireSuperAdminOnPanelHost(s), auditServerAction(auditRec))
	e.GET("/api/v1/servers/:id/firewall/rules", stackH.firewallRules, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/servers/:id/firewall/reset", stackH.firewallReset, RequireAuth(s), RequireAdmin(s), requireSuperAdminOnPanelHost(s), auditServerAction(auditRec))

	// ─── PM2 Process Management ──
	e.POST("/api/v1/servers/:id/pm2/manage", stackH.pm2Manage, RequireAuth(s), RequireAdmin(s), requireSuperAdminOnPanelHost(s), auditServerAction(auditRec))

	// ─── Mail server (proxied to the agent) ──
	e.POST("/api/v1/servers/:id/email/manage", stackH.emailManage, RequireAuth(s), RequireAdmin(s), requireSuperAdminOnPanelHost(s), auditServerAction(auditRec))

	// ─── Redis Management ──
	e.GET("/api/v1/servers/:id/redis/stats", stackH.redisStats, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/servers/:id/redis/flush", stackH.redisFlush, RequireAuth(s), RequireAdmin(s), requireSuperAdminOnPanelHost(s), auditServerAction(auditRec))
	e.GET("/api/v1/servers/:id/redis/info", stackH.redisInfo, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/servers/:id/redis/flushdb", stackH.redisFlushDB, RequireAuth(s), RequireAdmin(s), requireSuperAdminOnPanelHost(s), auditServerAction(auditRec))

	// ─── Threat Monitor (admin only) ──
	smH := newSecurityMonitorHandler(s, auditRec)
	e.GET("/api/v1/servers/:id/security/monitor", smH.monitor, RequireAuth(s), RequireAdmin(s))
	e.GET("/api/v1/servers/:id/security/baseline", smH.getBaseline, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/servers/:id/security/baseline", smH.saveBaseline, RequireAuth(s), RequireAdmin(s))

	// ─── Fail2ban Management ──
	e.GET("/api/v1/servers/:id/fail2ban/status", stackH.fail2banStatus, RequireAuth(s), RequireAdmin(s))
	e.GET("/api/v1/servers/:id/security/overview", stackH.securityOverview, RequireAuth(s), RequireAdmin(s))
	e.GET("/api/v1/servers/:id/fail2ban/jail", stackH.fail2banJail, RequireAuth(s), RequireAdmin(s))
	e.POST("/api/v1/servers/:id/fail2ban/unban", stackH.fail2banUnban, RequireAuth(s), RequireAdmin(s), requireSuperAdminOnPanelHost(s), auditServerAction(auditRec))
	e.POST("/api/v1/servers/:id/fail2ban/ban", stackH.fail2banBan, RequireAuth(s), RequireAdmin(s), requireSuperAdminOnPanelHost(s), auditServerAction(auditRec))
	e.POST("/api/v1/servers/:id/fail2ban/enable", stackH.fail2banEnable, RequireAuth(s), RequireAdmin(s), requireSuperAdminOnPanelHost(s), auditServerAction(auditRec))
	e.POST("/api/v1/servers/:id/fail2ban/disable", stackH.fail2banDisable, RequireAuth(s), RequireAdmin(s), requireSuperAdminOnPanelHost(s), auditServerAction(auditRec))

	// ─── Bot Blocker Management ──
	e.POST("/api/v1/servers/:id/bot-blocker/manage", stackH.botBlockerManage, RequireAuth(s), RequireAdmin(s), requireSuperAdminOnPanelHost(s), auditServerAction(auditRec))

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
	// Approving or rejecting orders (plans are global): super_admin only.
	e.PATCH("/api/v1/orders/:id", orderH.updateOrderStatus, RequireAuth(s), RequireUserSession, RequireRole(s, auth.RoleSuperAdmin))

	// ─── Assign Plan to User ──────────────────────────────────
	e.POST("/api/v1/users/:id/assign-plan", uh.assignPlan, RequireAuth(s), RequireUserSession, RequireAdmin(s))

	// Example: feature-gated route (uncomment when ready to gate specific endpoints)
	// e.GET("/api/v1/servers", sh.list, RequireAuth(s), RequireFeature(s, "multi_server"))

	return e
}

func customErrorHandler(c *echo.Context, err error) {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	if responseCommitted(c) {
		return
	}
	var pse *middleware.PanicStackError
	if errors.As(err, &pse) {
		slog.Error("api: recovered from handler panic", "route", c.Path(), "error", pse.Err, "stack", string(pse.Stack))
	}

	// *echo.HTTPError carries our message; echo's own errors (404/405 from the
	// router, 413 from BodyLimit) only carry a status; anything else is a 500
	// whose details are not shown.
	code, msg := http.StatusInternalServerError, "internal error"
	var he *echo.HTTPError
	if errors.As(err, &he) {
		code, msg = he.Code, he.Message
	} else if sc := echo.StatusCode(err); sc != 0 {
		code, msg = sc, http.StatusText(sc)
	}
	httpjson.Error(c.Response(), code, rid, errorCodeFromStatus(code), msg)
}

// responseStatus is the status written (or about to be written) for c.
func responseStatus(c *echo.Context) int {
	if r, err := echo.UnwrapResponse(c.Response()); err == nil {
		return r.Status
	}
	return 0
}

// responseCommitted reports whether c's response headers were already sent.
func responseCommitted(c *echo.Context) bool {
	r, err := echo.UnwrapResponse(c.Response())
	return err == nil && r.Committed
}

func healthHandler(c *echo.Context) error {
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

func versionHandler(c *echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"service": "sigardapanel-api",
			"version": buildinfo.Version,
		},
		RequestID: rid,
	})
}

func notImplementedHandler(action string) echo.HandlerFunc {
	return func(c *echo.Context) error {
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
	case http.StatusMethodNotAllowed:
		return "method_not_allowed"
	case http.StatusConflict:
		return "conflict"
	case http.StatusRequestEntityTooLarge:
		return "payload_too_large"
	case http.StatusBadGateway:
		return "bad_gateway"
	case http.StatusServiceUnavailable:
		return "unavailable"
	case http.StatusTooManyRequests:
		return "rate_limited"
	case http.StatusNotImplemented:
		return "not_implemented"
	default:
		return "internal_error"
	}
}
