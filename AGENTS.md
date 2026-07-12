# SigardaPanel — Agent & Developer Guide

## Architecture

SigardaPanel adalah panel VPS berbasis Go dengan arsitektur client-server.

```
Panel Server
  API (:7700) + Frontend (:7720)
  SQLite DB (WAL mode)
      |
Agent (:7710) per VPS
  - Site management
  - SSL certificates
  - Docker management
  - Firewall (UFW)
  - Redis/Memcached
  - System metrics
```

## Services

| Service  | Port  | Description                          |
|----------|-------|--------------------------------------|
| API      | 8080  | Panel API server                     |
| Frontend | 4001  | SvelteKit web dashboard              |
| Agent    | 9090  | Agent HTTP server (per VPS)          |

## Quick Start (Dev Mode)

```bash
cd SigardaPanel-Enterprise
./sigardapanel dev
```

Frontend: `cd web && npx vite dev --host 0.0.0.0 --port 4000`

## Default Credentials

- Username: `admin`
- Password: `admin123`

## CLI Commands

```
sigardapanel
├── api / agent / dev / install / init / login / logout / doctor / version
├── server    add|list|update|remove|doctor
├── site      create|list|update|delete|deploy|config|setup-logrotate
├── ssl       status|issue|renew|renew-all
├── job       list|watch|cancel|logs
├── backup    create|list|delete|restore
├── db        create|list|update|delete
│   └── user  create|list|rotate-password|delete
├── docker    ps|start|stop|restart|rm|logs|images|compose
├── git       branches|log|rollback|checkout
├── firewall  status|enable|disable|allow|deny|rules|delete|reset
├── redis     stats|info|flush|flushdb
├── system    disk-usage|updates
├── channels  add|list|remove
├── alerts    list
├── metrics   latest|list
├── user      create|list|reset-password|delete
└── cloudpanel scan|import-sites|import-ssl|doctor
```

## API Endpoints (~150+)

### Auth (5 endpoints)
- `POST /api/v1/auth/login` — Login
- `GET /api/v1/auth/me` — Current user
- `PUT /api/v1/auth/profile` — Update profile
- `POST /api/v1/auth/logout` — Logout
- `POST /api/v1/auth/stop-impersonation` — Stop impersonation

### Servers (8 endpoints)
- `GET /api/v1/servers` — List servers
- `POST /api/v1/servers` — Create server
- `GET /api/v1/servers/:id` — Server detail
- `PATCH /api/v1/servers/:id` — Update server
- `DELETE /api/v1/servers/:id` — Delete server
- `GET /api/v1/servers/health` — All servers health
- `GET /api/v1/servers/:id/health` — Server health
- `POST /api/v1/servers/:id/token` — Regenerate agent token

### Sites (15 endpoints)
- `GET /api/v1/sites` — List sites
- `POST /api/v1/sites` — Create site
- `GET /api/v1/sites/:id` — Site detail
- `DELETE /api/v1/sites/:id` — Delete site
- `POST /api/v1/sites/:id/deploy` — Deploy site
- `POST /api/v1/sites/:id/config` — Update config
- `POST /api/v1/sites/:id/reload-nginx` — Reload nginx
- `POST /api/v1/sites/:id/transfer` — Transfer site
- `GET /api/v1/sites/:id/vhost` — Get vhost
- `PUT /api/v1/sites/:id/vhost` — Update vhost
- `POST /api/v1/sites/:id/vhost/validate` — Validate vhost
- `GET /api/v1/sites/:id/usage` — Disk usage
- `GET /api/v1/sites/:id/logs` — Tail logs
- `POST /api/v1/webhooks/deploy/:site_id` — Git webhook
- `GET /api/v1/sites/:id/ssh/info` — SSH info

### App Runtime (7 endpoints)
- `GET /api/v1/sites/:id/app/status` — App status
- `POST /api/v1/sites/:id/app/start` — Start app
- `POST /api/v1/sites/:id/app/stop` — Stop app
- `POST /api/v1/sites/:id/app/restart` — Restart app
- `PUT /api/v1/sites/:id/app/config` — Update config
- `GET /api/v1/sites/:id/app/resource-usage` — Resource usage
- `PUT /api/v1/sites/:id/app/resource-limits` — Update limits

### PHP (2 endpoints)
- `GET /api/v1/sites/:id/php/config` — Get PHP config
- `PUT /api/v1/sites/:id/php/config` — Update PHP config

### SSH/FTP (4 endpoints)
- `GET /api/v1/sites/:id/ssh-ftp` — List SSH accounts
- `POST /api/v1/sites/:id/ssh-ftp` — Create SSH account
- `DELETE /api/v1/sites/:id/ssh-ftp/:ssh_id` — Delete SSH account
- `GET /api/v1/sites/:id/ssh-ftp/:ssh_id/key` — Get private key

### Cron (3 endpoints)
- `GET /api/v1/sites/:id/cron` — List cron jobs
- `POST /api/v1/sites/:id/cron` — Create cron job
- `DELETE /api/v1/sites/:id/cron/:cron_id` — Delete cron job

### Security (2 endpoints)
- `GET /api/v1/sites/:id/security` — Get security settings
- `PUT /api/v1/sites/:id/security` — Update security

### Varnish (2 endpoints)
- `GET /api/v1/sites/:id/varnish` — Get Varnish config
- `POST /api/v1/sites/:id/varnish/purge` — Purge cache

### File Manager (10 endpoints)
- `GET /api/v1/sites/:id/files` — List files
- `GET /api/v1/sites/:id/files/read` — Read file
- `POST /api/v1/sites/:id/files/write` — Write file
- `POST /api/v1/sites/:id/files/create` — Create entry
- `DELETE /api/v1/sites/:id/files` — Delete entry
- `POST /api/v1/sites/:id/files/rename` — Rename entry
- `POST /api/v1/sites/:id/files/move` — Move entries
- `POST /api/v1/sites/:id/files/compress` — Compress entries
- `POST /api/v1/sites/:id/files/upload` — Upload file
- `GET /api/v1/sites/:id/files/download` — Download file

### SSL (4 endpoints)
- `GET /api/v1/sites/:id/ssl` — SSL status
- `POST /api/v1/sites/:id/ssl/issue` — Issue cert
- `POST /api/v1/sites/:id/ssl/renew` — Renew cert
- `POST /api/v1/ssl/renew-all` — Batch renew all

### Databases (14 endpoints)
- `GET /api/v1/databases` — List all databases
- `POST /api/v1/sites/:id/databases` — Create database
- `GET /api/v1/sites/:id/databases` — List site databases
- `GET /api/v1/databases/:id` — Database detail
- `PATCH /api/v1/databases/:id` — Update database
- `DELETE /api/v1/databases/:id` — Delete database
- `POST /api/v1/databases/:id/users` — Create DB user
- `GET /api/v1/databases/:id/users` — List DB users
- `DELETE /api/v1/databases/:id/users/:user_id` — Delete DB user
- `POST /api/v1/databases/:id/users/:user_id/rotate-password` — Rotate password
- `POST /api/v1/databases/:id/grant-user` — Grant user
- `GET /api/v1/databases/:id/available-users` — Available users
- `GET /api/v1/db-users` — List all DB users
- `PATCH /api/v1/db-users/:id` — Update DB user

### Backups (16 endpoints)
- `POST /api/v1/backups/bulk` — Bulk create
- `POST /api/v1/sites/:id/backups` — Create backup
- `POST /api/v1/sites/:id/backups/database` — Database backup
- `GET /api/v1/backups` — List backups
- `GET /api/v1/backups/:id/download` — Download backup
- `DELETE /api/v1/backups/:id` — Delete backup
- `POST /api/v1/backups/:id/restore` — Restore backup
- `POST /api/v1/sites/:id/schedules` — Create schedule
- `GET /api/v1/sites/:id/schedules` — List schedules
- `PATCH /api/v1/schedules/:id` — Update schedule
- `DELETE /api/v1/schedules/:id` — Delete schedule
- `GET /api/v1/backup-configs` — List backup configs
- `POST /api/v1/backup-configs` — Create backup config
- `PATCH /api/v1/backup-configs/:id` — Update backup config
- `DELETE /api/v1/backup-configs/:id` — Delete backup config
- `POST /api/v1/backup-configs/:id/run` — Run backup config

### Backup Providers (6 endpoints)
- `GET /api/v1/backup-providers` — List providers
- `POST /api/v1/backup-providers` — Create provider
- `PATCH /api/v1/backup-providers/:id` — Update provider
- `DELETE /api/v1/backup-providers/:id` — Delete provider
- `GET /api/v1/backup-providers/:id/usage` — Provider usage
- `POST /api/v1/backup-providers/:id/test` — Test connection

### Jobs (3 endpoints)
- `GET /api/v1/jobs` — List jobs
- `GET /api/v1/jobs/:id` — Job detail
- `POST /api/v1/jobs/:id/cancel` — Cancel job

### Job Logs (2 endpoints)
- `POST /api/v1/jobs/:id/logs` — Append logs
- `GET /api/v1/jobs/:id/logs` — Get logs

### Metrics (3 endpoints)
- `POST /api/v1/servers/:id/metrics` — Ingest metrics
- `GET /api/v1/servers/:id/metrics/latest` — Latest metrics
- `GET /api/v1/servers/:id/metrics` — Metrics range

### Service Tokens (3 endpoints)
- `GET /api/v1/service-tokens` — List tokens
- `POST /api/v1/service-tokens` — Create token
- `DELETE /api/v1/service-tokens/:id` — Revoke token

### Alerts (4 endpoints)
- `POST /api/v1/alert-channels` — Create channel
- `GET /api/v1/alert-channels` — List channels
- `DELETE /api/v1/alert-channels/:id` — Delete channel
- `GET /api/v1/alerts` — List alerts

### Notifications (6 endpoints)
- `GET /api/v1/notifications` — List notifications
- `GET /api/v1/notifications/unread-count` — Unread count
- `POST /api/v1/notifications/:id/read` — Mark read
- `POST /api/v1/notifications/read-all` — Mark all read
- `DELETE /api/v1/notifications/:id` — Delete notification
- `DELETE /api/v1/notifications/read` — Delete all read

### Cloudflare (14 endpoints)
- `GET /api/v1/cloudflare/config` — Get config
- `PUT /api/v1/cloudflare/config` — Save config
- `DELETE /api/v1/cloudflare/config` — Delete config
- `GET /api/v1/cloudflare/verify` — Verify token
- `GET /api/v1/cloudflare/zones` — List zones
- `GET /api/v1/cloudflare/zones/search` — Search zones
- `GET /api/v1/cloudflare/zones/:id` — Zone detail
- `GET /api/v1/cloudflare/zones/:id/dns` — List DNS records
- `POST /api/v1/cloudflare/zones/:id/dns` — Create DNS record
- `DELETE /api/v1/cloudflare/zones/:id/dns/:recordId` — Delete DNS record
- `PUT /api/v1/cloudflare/zones/:id/dns/:recordId` — Update DNS record
- `PATCH /api/v1/cloudflare/zones/:id/dns/:recordId` — Patch DNS record
- `POST /api/v1/cloudflare/zones/:id/dns/bulk-delete` — Bulk delete DNS
- `POST /api/v1/cloudflare/zones/:id/dns/import` — Import DNS records
- `POST /api/v1/cloudflare/zones/:id/purge-cache` — Purge cache
- `GET /api/v1/cloudflare/zones/:id/analytics` — Zone analytics
- `GET /api/v1/cloudflare/zones/:id/analytics/range` — Analytics range

### Stack (4 endpoints)
- `GET /api/v1/servers/:id/stack` — Stack info
- `POST /api/v1/servers/:id/stack/action` — Install/manage
- `POST /api/v1/servers/:id/stack/restart` — Restart services
- `POST /api/v1/servers/:id/command` — Execute command

### Docker (19 endpoints)
- `GET /api/v1/servers/:id/docker/containers` — List containers
- `POST /api/v1/servers/:id/docker/container/start` — Start container
- `POST /api/v1/servers/:id/docker/container/stop` — Stop container
- `POST /api/v1/servers/:id/docker/container/restart` — Restart container
- `POST /api/v1/servers/:id/docker/container/remove` — Remove container
- `GET /api/v1/servers/:id/docker/container/logs` — Container logs
- `GET /api/v1/servers/:id/docker/images` — List images
- `POST /api/v1/servers/:id/docker/image/pull` — Pull image
- `POST /api/v1/servers/:id/docker/image/remove` — Remove image
- `GET /api/v1/servers/:id/docker/volumes` — List volumes
- `POST /api/v1/servers/:id/docker/volume/create` — Create volume
- `POST /api/v1/servers/:id/docker/volume/remove` — Remove volume
- `GET /api/v1/servers/:id/docker/networks` — List networks
- `POST /api/v1/servers/:id/docker/network/create` — Create network
- `POST /api/v1/servers/:id/docker/network/remove` — Remove network
- `GET /api/v1/servers/:id/docker/info` — Docker info
- `GET /api/v1/servers/:id/docker/disk-usage` — Disk usage
- `POST /api/v1/servers/:id/docker/compose/up` — Compose up
- `POST /api/v1/servers/:id/docker/compose/down` — Compose down

### Git (4 endpoints)
- `GET /api/v1/sites/:id/git/branches` — List branches
- `GET /api/v1/sites/:id/git/log` — Commit history
- `POST /api/v1/sites/:id/git/rollback` — Rollback to commit
- `POST /api/v1/sites/:id/git/checkout` — Checkout branch

### Firewall (8 endpoints)
- `GET /api/v1/servers/:id/firewall/status` — Firewall status
- `POST /api/v1/servers/:id/firewall/enable` — Enable firewall
- `POST /api/v1/servers/:id/firewall/disable` — Disable firewall
- `POST /api/v1/servers/:id/firewall/allow` — Allow port
- `POST /api/v1/servers/:id/firewall/deny` — Deny port
- `POST /api/v1/servers/:id/firewall/delete` — Delete rule
- `GET /api/v1/servers/:id/firewall/rules` — List rules
- `POST /api/v1/servers/:id/firewall/reset` — Reset firewall

### Redis (4 endpoints)
- `GET /api/v1/servers/:id/redis/stats` — Redis stats
- `POST /api/v1/servers/:id/redis/flush` — Flush all
- `GET /api/v1/servers/:id/redis/info` — Redis info
- `POST /api/v1/servers/:id/redis/flushdb` — Flush database

### License & Plans (9 endpoints)
- `GET /api/v1/license` — Get license
- `POST /api/v1/license/activate` — Activate license
- `DELETE /api/v1/license` — Deactivate license
- `GET /api/v1/license/features` — Get features
- `GET /api/v1/plans` — List plans
- `POST /api/v1/plans` — Create plan
- `GET /api/v1/plans/:id` — Plan detail
- `PATCH /api/v1/plans/:id` — Update plan
- `DELETE /api/v1/plans/:id` — Delete plan

### Orders (4 endpoints)
- `GET /api/v1/orders` — List orders
- `POST /api/v1/orders` — Create order
- `GET /api/v1/orders/:id` — Order detail
- `PATCH /api/v1/orders/:id` — Update order

### Misc (5 endpoints)
- `GET /health` — Health check
- `GET /api/v1/health` — Health check
- `GET /api/v1/version` — API version
- `POST /api/v1/agents/register` — Agent registration
- `GET /api/v1/agents/install` — Agent install script
- `POST /api/v1/agents/tokens` — Create registration token
- `GET/PUT /api/v1/settings/retention` — Retention settings
- `POST /api/v1/databases/phpmyadmin-credentials` — phpMyAdmin login
- `POST /api/v1/databases/phpmyadmin-reset-passwords` — Reset passwords
- `GET /api/v1/database-server` — DB server info
- `POST /api/v1/database-server/import` — Import databases
- `GET /api/v1/audit-logs` — Audit logs

## Env Variables

| Variable | Default | Description |
|----------|---------|-------------|
| SIGARDAPANEL_API_ADDR | :7700 | API listen address |
| SIGARDAPANEL_AGENT_ADDR | :7710 | Agent listen address |
| SIGARDAPANEL_DB_PATH | sigardapanel.db | Database path |
| SIGARDAPANEL_AGENT_TOKEN | - | Agent auth token |
| SIGARDAPANEL_API_URL | - | API base URL for CLI |
| SIGARDAPANEL_TOKEN | - | API token from login |
| SIGARDAPANEL_OUTPUT | table | Output format (table|json|yaml) |

## Repositories

- **Public (binary distribution)**: github.com/bayurstarcool/SigardaPanel
- **Enterprise (full source)**: github.com/bayurstarcool/SigardaPanel-Enterprise

## GitHub Releases

Binary releases: https://github.com/bayurstarcool/SigardaPanel/releases
Latest: v0.5.0 — Full Docker/Firewall/Redis management, Global Search, SSL Dashboard
