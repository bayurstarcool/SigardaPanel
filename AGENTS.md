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
  - Fail2ban management
  - Redis/Memcached
  - WebSocket PTY Terminal
  - System metrics
```

## Services

| Service  | Port  | Description                          |
|----------|-------|--------------------------------------|
| API      | 7700  | Panel API server                     |
| Frontend | 7720  | SvelteKit web dashboard              |
| Agent    | 7710  | Agent HTTP server (per VPS)          |

## Quick Start (Dev Mode)

```bash
go build -o sigardapanel ./cmd/sigardapanel
SIGARDAPANEL_DEV=true ./sigardapanel init --user admin   # first account (prompts for a password)
SIGARDAPANEL_DEV=true ./sigardapanel dev
```

Frontend: `cd web && npx vite dev --host 0.0.0.0 --port 7720`

Full dev setup: [docs/GUIDE.md](docs/GUIDE.md). Production installs: [docs/INSTALL.md](docs/INSTALL.md).

## Credentials

No account is seeded. In development create one with `sigardapanel init`
(local dev databases conventionally use `admin` / `admin123`). Production
installs get a random password from `install.sh` (or `ADMIN_PASS`), and the
first account is `super_admin`. Never document or ship a default password.

`users.password_changed_at IS NULL` means "password chosen by someone else"
(install.sh-generated, admin-created, `init --must-change-password`). While it is
NULL the API (`RequireAuth`) answers `403 password_change_required` except for
`GET /auth/me`, `GET /auth/2fa/status`, `PUT /auth/profile`, `POST /auth/logout`;
the account's API keys are refused; impersonation sessions are exempt. A password
supplied by the operator (`ADMIN_PASS`, `init` without the flag, self sign-up)
is final. CLI way out: `sigardapanel passwd`.

## User-Site Isolation Model

SigardaPanel uses **ownership-based site isolation**:

```
┌─────────────────────────────────────────────────────┐
│                   Roles Hierarchy                    │
├─────────────────────────────────────────────────────┤
│  super_admin → sees ALL sites & users               │
│  admin       → sees only OWN sites + ASSIGNED sites  │
│  user        → sees only OWN sites + ASSIGNED sites  │
└─────────────────────────────────────────────────────┘
```

### How It Works

1. **User creates site** → `owner_id` = creator's user ID
2. **Site belongs to creator** — only creator (and super_admin) can access it
3. **Multi-user access** — owner can assign site to other users via `user_sites` table
4. **Permission levels** — `read`, `write`, `admin` per assignment
5. **Linux user per panel user** — each panel user gets a Linux system user

### Linux User Model

Every panel user has a matching Linux system user:

```
Panel User       Linux User      Home Directory
────────────────────────────────────────────────
admin            admin           /home/admin/htdocs/{domain}
bayur            bayur           /home/bayur/htdocs/{domain}
hanyawahyu       hanyawahyu      /home/hanyawahyu/htdocs/{domain}
```

- **User created** → `useradd -m -s /bin/bash <username>` runs automatically
- **Site created** → `system_user` set to owner's Linux username; the `site.create`
  job carries it and the agent (panel host or remote server) creates the account
  with `useradd -m -s /bin/bash` if it is missing (validated name, never root or
  service accounts; existing UID < 1000 accounts are not used as owner) and
  `chown -R <user>:` the site root it just created (an existing root keeps its
  ownership). PHP sites also fail fast (`422`, permanent job error) when the
  server lacks the PHP-FPM socket/port the vhost would use.
- **Root path** → `/home/{system_user}/htdocs/{domain}`
- **File ownership** → `{system_user}:{system_user}` (not www-data!)
- **PHP-FPM** → pool runs as `www-data` (shared, for now)
- **SSH/FTP** → optional per-site user for site-level access
- **Per-site terminal** → SSH tab in site detail runs shell as `system_user` via `?user=<system_user>`

### Assignment API

```
POST /api/v1/users/:id/sites          → Assign user to site
GET  /api/v1/users/:id/sites          → List user's assigned sites
DELETE /api/v1/users/:id/sites/:siteId → Unassign
PATCH /api/v1/users/:id/sites/:siteId → Update permission
GET  /api/v1/sites/:id/users          → List site's assigned users
```

### Permission Matrix

| Action | super_admin | Owner | admin (assigned) | write (assigned) | read (assigned) |
|--------|:-----------:|:-----:|:----------------:|:----------------:|:---------------:|
| View site | ✅ | ✅ | ✅ | ✅ | ✅ |
| Edit config | ✅ | ✅ | ✅ | ✅ | ❌ |
| Deploy | ✅ | ✅ | ✅ | ✅ | ❌ |
| Delete site | ✅ | ✅ | ❌ | ❌ | ❌ |
| Manage users | ✅ | ✅ | ❌ | ❌ | ❌ |

### Example

```
1. hanyawahyu (super_admin) creates "tirtamedia.co.id"
   → owner_id=3, system_user="hanyawahyu"

2. hanyawahyu assigns bayur (admin) to "tirtamedia.co.id" with "write" permission
   → bayur can view & edit, but cannot delete

3. bayur sees: [tirtamedia.co.id] (1 site)
   hanyawahyu sees: [all 16 sites]

4. bayur creates "bayurnews.com"
   → owner_id=2
   → bayur owns it, hanyawahyu (super_admin) can also see it
```

## CLI Commands

Full reference: [docs/CLI.md](docs/CLI.md). Help: `sigardapanel help`, `sigardapanel <cmd> [sub] --help`.

```
sigardapanel
├── api / agent / dev / install / init        services & bootstrap (local)
├── login / logout / whoami / version / doctor
├── server       list|get|health|add|update|remove|token|doctor
├── site         list|get|create|update|delete|deploy|suspend|resume|reload-nginx|setup-logrotate
├── ssl          status|issue|renew|renew-all
├── job          list|get|watch|cancel|logs
├── backup       list|create|restore|delete
├── db           list|create|update|delete
│   └── user     list|create|rotate-password|delete
├── logs         tail
├── metrics      latest|list|gpu
├── system       disk-usage|updates
├── panel-update status|check|apply|auto     (alias self-update; super_admin)
├── user         list|create|update|reset-password|delete
├── apikey       list|create|revoke
├── audit        list|purge
├── notification list|count|read|delete
├── channels     list|add|remove
├── alerts       list
├── docker       ps|start|stop|restart|rm|logs|images|compose
├── git          branches|log|rollback|checkout
├── firewall     status|enable|disable|allow|deny|rules|delete|reset
├── redis        stats|info|flush|flushdb
└── cloudpanel   scan|import-sites|import-site|import-ssl|doctor
```

CLI conventions: `-o table|json|yaml` on list/get commands, `--yes` on destructive
ones, exit codes 0 ok / 1 error / 2 usage. CLI code: `internal/commands` (shared
helpers in `helpers.go`), `internal/cli` (client, config, output); tests use
`setupMockAPI` and never touch the real `~/.config/sigardapanel` or `localhost:7700`.

## API Endpoints (~223)

### Auth (5 endpoints)
- `POST /api/v1/auth/login` — Login
- `GET /api/v1/auth/me` — Current user
- `PUT /api/v1/auth/profile` — Update profile
- `POST /api/v1/auth/logout` — Logout
- `POST /api/v1/auth/stop-impersonation` — Stop impersonation
- `GET /api/v1/public/config` (alias `GET /api/v1/auth/signup`) — public: `landing_page`, `member_signup`, `panel_signup`

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
- `GET /api/v1/jobs/:id/logs` — Get job logs

### Metrics (3 endpoints)
- `POST /api/v1/servers/:id/metrics` — Ingest metrics
- `GET /api/v1/servers/:id/metrics/latest` — Latest metrics
- `GET /api/v1/servers/:id/metrics` — Metrics range

### Service Tokens (3 endpoints)
- `GET /api/v1/service-tokens` — List tokens
- `POST /api/v1/service-tokens` — Create token
- `DELETE /api/v1/service-tokens/:id` — Revoke token

### Personal API Keys (3 endpoints)
- `GET /api/v1/api-keys` — List the caller's keys (name, prefix, created/last used/expiry; never the key)
- `POST /api/v1/api-keys` — Create `{name, expires_in_days}`; raw key returned once, stored as SHA-256 hash; max 20 per user
- `DELETE /api/v1/api-keys/:id` — Revoke one of the caller's keys (another user's key is 404)
- A key authenticates as its owner (Bearer). Interactive-session-only endpoints (`RequireUserSession`: users, impersonation, service tokens, terminal tickets, api-keys, logout) reject it.

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

### Cloudflare (16 endpoints)
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

### Stack (6 endpoints)
- `GET /api/v1/servers/:id/stack` — Stack info (packages, `php_versions`, distro, `is_local`, running `operation`)
- `POST /api/v1/servers/:id/stack/action` — Install/manage (admin). Package operations
  (`install`/`uninstall`/`update`/`install-extension`/`uninstall-extension`) answer 202 `{op_id}`
  and run in the background on the agent (one at a time per agent, 45 min cap, verified result)
- `GET /api/v1/servers/:id/stack/ops/:op` — Operation progress/result (log tail)
- `POST /api/v1/servers/:id/stack/restart` — Restart services
- `GET /api/v1/servers/:id/php-versions` — Installed PHP-FPM versions (site forms)
- `POST /api/v1/servers/:id/command` — Execute command (`timeout` 1–300 s; super_admin on the panel host)

Stack code: agent `internal/agenthttp/stack.go` (handlers, operations), `stack_pkgs.go`
(components), `stack_sys.go` (host access + apt helpers). All host access goes through
`stackSystem`; routers built with `RouterOptions` get `denyStackSystem` unless a test injects
a fake (`stack_test.go`), so tests never run apt/systemctl. Guards (sites using a PHP version,
nginx/node/docker/certbot/mariadb in use, panel-host protections, super_admin for destructive
actions on the panel host) live in `internal/apihttp/stack_manage.go`; the agent independently
refuses removing a PHP version referenced by an nginx vhost and any removal that `apt-get -s`
shows would take other packages with it.

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

### Fail2ban (6 endpoints)
- `GET /api/v1/servers/:id/fail2ban/status` — Fail2ban status
- `GET /api/v1/servers/:id/fail2ban/jail` — List jails
- `POST /api/v1/servers/:id/fail2ban/ban` — Ban IP
- `POST /api/v1/servers/:id/fail2ban/unban` — Unban IP
- `POST /api/v1/servers/:id/fail2ban/enable` — Enable jail
- `POST /api/v1/servers/:id/fail2ban/disable` — Disable jail

### Redis (4 endpoints)
- `GET /api/v1/servers/:id/redis/stats` — Redis stats
- `POST /api/v1/servers/:id/redis/flush` — Flush all
- `GET /api/v1/servers/:id/redis/info` — Redis info
- `POST /api/v1/servers/:id/redis/flushdb` — Flush database

### WebSocket Terminal (1 endpoint)
- `GET /api/ws/terminal` — WebSocket PTY terminal
  - Full TUI support (htop, vim, nano)
  - Session persistence across reconnections
  - Sticky keyboard shortcuts (Ctrl+C/Z/D/L/U, Tab, Esc, arrows)
  - Mobile-optimized (no resize interference during typing)

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
- `GET /api/v1/system/panel-update` — Installed/latest release, update status (`?refresh=1` re-checks GitHub; super_admin)
- `POST /api/v1/system/panel-update` — Install the latest release (202; super_admin, interactive session)
- `GET /api/v1/system/panel-update/summary` — Cached `{current_version, latest_tag, update_available}` for the header badge (super_admin)
- `PUT /api/v1/system/panel-update/settings` — `{auto_update, auto_update_window: {start,end}|null}`, partial (super_admin)
- `POST /api/v1/databases/phpmyadmin-credentials` — phpMyAdmin login
- `POST /api/v1/databases/phpmyadmin-reset-passwords` — Reset passwords
- `GET /api/v1/database-server` — DB server info
- `POST /api/v1/database-server/import` — Import databases
- `GET /api/v1/audit-logs` — Audit logs
- `DELETE /api/v1/audit-logs?older_than_days=N` — Purge entries older than N days (super_admin, interactive session; the purge is itself audited)

## WebSocket Terminal

### Architecture
```
Browser (xterm.js) ←WebSocket→ Nginx ←WebSocket→ Agent (:7710/ws/terminal)
                                         ↕
                                    PTY (bash)
```

### Features
- **Full TUI support**: htop, vim, nano, tmux
- **Session persistence**: reconnect to same PTY session after disconnect
- **Sticky keyboard shortcuts**: Ctrl+C/Z/D/L/U, Tab, Esc, arrows
- **Mobile-optimized**: no resize interference during typing
- **Pending buffer**: up to 64KB buffered when no client attached
- **Per-site terminal**: `?user=<system_user>` runs shell as site owner (not root)

### Session Persistence Flow
1. Agent sends `{"type":"session","id":"pty_xxx"}` on connect
2. Frontend stores session ID in memory
3. On reconnect, frontend sends `?session=pty_xxx` query param
4. Agent swaps WebSocket reference (single goroutine per session)
5. Agent flushes pending buffer + sends prompt redraw

### Nginx Proxy Config
```nginx
location /api/ws/ {
    proxy_pass http://127.0.0.1:7710/ws/;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header Authorization "Bearer <agent_token>";
    proxy_read_timeout 3600s;
    proxy_send_timeout 3600s;
}
```

## Panel Self-Update

Settings → **Panel update** (super_admin). `internal/selfupdate` looks up the latest
GitHub release (cached; tags must be `vX.Y.Z`) and compares versions. The API
(`internal/apihttp/panel_update.go`) asks the panel host's own agent
(`SIGARDAPANEL_AGENT_URL`, must be loopback) to install it; the agent
(`internal/agenthttp/panel_update.go`, `GET|POST /tasks/panel/update`) writes the
installer embedded in the binary (`installer/embed.go` → `installer/install.sh`) to
`/var/lib/sigardapanel-update/` and runs `install.sh update` with
`SIGARDA_VERSION=<tag>` in the transient unit `sigardapanel-update.service`
(`systemd-run --collect`), so it survives the agent/API/web restarts. Signature
check, backup and rollback are the installer's. The agent refuses dev builds,
hosts without `/etc/sigardapanel/panel.env`, non-newer tags, a second run and a
failed preflight (systemd-run, free space for backup/download/binary). A new start
archives the previous run to `history/` (last 10, shown in the card).
`StartPanelUpdateChecker` (1 min after start, then hourly) announces the result of
the latest run once (`panel_update_reported_run`), notifies super admins once per new
release and, with `app_settings.panel_auto_update=true` and inside
`panel_auto_update_window` ("start-end" local hours), starts one update per release
(`panel_update_auto_tag`). `watchPanelUpdate` follows a started update and announces
its result without waiting for the hourly tick. `selfupdate.Checker` shares one lookup
between callers, retries a failed lookup after 5 min and never caches a lookup the
caller cancelled. CLI: `internal/commands/panelupdate.go`. Injected agent routers get a
runner that executes nothing (`RouterOptions.PanelUpdate*`). `build.sh` must copy `installer/`.

## Fail2ban Management

### Features
- View fail2ban status and jail list
- Ban/unban IPs per jail
- Enable/disable jails
- Real-time status updates

### Agent Endpoints
- `GET /tasks/fail2ban/status` — Overall status
- `GET /tasks/fail2ban/jail` — Jail list with stats
- `POST /tasks/fail2ban/ban` — Ban IP `{jail, ip}`
- `POST /tasks/fail2ban/unban` — Unban IP `{jail, ip}`
- `POST /tasks/fail2ban/enable` — Enable jail `{jail}`
- `POST /tasks/fail2ban/disable` — Disable jail `{jail}`

## Backup System

### Job Types
- `backup.create` — File backup
- `backup.database` — Database backup
- `backup.restore` — Restore backup

### Features
- **S3 upload**: backup to S3-compatible storage (B2, Wasabi, etc.)
- **Local fallback**: if S3 fails, keep local backup
- **Concurrency limit**: max 1 concurrent backup per server (enforced atomically when a job is claimed)
- **Stuck cleanup**: jobs stuck >30min auto-marked as failed
- **Deduplication**: skip if backup job already active for site
- **Retention**: auto-expire old backups based on config

### Worker Flow
The worker runs a bounded pool (default 3, `SIGARDAPANEL_WORKER_CONCURRENCY` 1-16).
Jobs of the same site never run concurrently. Jobs left `running` by a previous
process are marked failed when the worker starts.

1. Claim next queued job
2. Set backup status to "running"
3. Execute on agent (tar + optional S3 upload)
4. On success: update backup record with paths
5. On failure: retry with backoff, then mark as failed
6. On permanent failure: create failover job (local only)

## Env Variables

| Variable | Default | Description |
|----------|---------|-------------|
| SIGARDAPANEL_API_ADDR | :7700 | API listen address |
| SIGARDAPANEL_AGENT_ADDR | :7710 | Agent listen address |
| SIGARDAPANEL_DB_PATH | sigardapanel.db | Database path |
| SIGARDAPANEL_AGENT_TOKEN | - | Agent auth token |
| SIGARDAPANEL_API_URL | http://localhost:7700 | CLI: API base URL (overrides the URL saved by `login`) |
| SIGARDAPANEL_TOKEN | - | CLI: bearer token, e.g. a personal API key (overrides the saved login token) |
| SIGARDAPANEL_OUTPUT | table | CLI: default output format (table\|json\|yaml) |
| SIGARDAPANEL_DEV | false | Dev mode (enables hot reload, disables rate limiters) |
| SIGARDAPANEL_ENCRYPTION_KEY | - | 32-byte hex AES key for secrets at rest (`openssl rand -hex 32`) |
| SIGARDAPANEL_ENCRYPTION_KEY_FILE | `<db dir>/encryption.key` | Key file used when the env key is unset |
| SIGARDAPANEL_WORKER_CONCURRENCY | 3 | Job worker pool size (1-16) |
| SIGARDAPANEL_ALLOW_REGISTRATION | false | Panel self-registration (`POST /api/v1/auth/register`) |
| SIGARDAPANEL_LANDING_PAGE | false | `true`: `/` and `/pricing` show the SaaS landing; otherwise they redirect to `/login` (`/dashboard` when signed in). Read by the web via `GET /api/v1/public/config` |
| SIGARDAPANEL_ALLOW_MEMBER_SIGNUP | false | `true`: `POST /api/v1/member/register` open (needs `MEMBER_DB_*`); otherwise 403 `member_signup_disabled` |

### Encryption key resolution

1. `SIGARDAPANEL_ENCRYPTION_KEY` (if valid).
2. The key file (`SIGARDAPANEL_ENCRYPTION_KEY_FILE`, default `encryption.key` next to the DB).
3. Legacy derived fallback `sha256(db path + ":" + agent token)` — weak.

On API start, if neither 1 nor 2 exists **and nothing is encrypted yet** (new
install), a random key is generated into the key file (mode 0600). Back it up
with the database. If data is already encrypted with the fallback, the panel
keeps using the fallback (so it still decrypts) and logs a loud warning;
switching requires re-encrypting those values.

## Rate Limiting

- **Login endpoint**: 10 attempts per 15 minutes per IP (`StrictLimiter`)
- **API endpoints**: 100 requests per minute per IP (`APIlimiter`)
- **Dev mode**: rate limiters are **bypassed** when `SIGARDAPANEL_DEV=true`
- Rate limit state is in-memory (resets on process restart)

## Bug Fixes Applied

### Health Check Status Sync (`internal/jobs/healthcheck.go`)
- Server status now syncs correctly: `offline` → `online` when agent responds
- Server status demotes: `degraded` → `offline` when agent unreachable
- Previously only promoted from `pending` → `online`

### Backup Access Control (`internal/apihttp/backups.go`)
- `GET /api/v1/sites/:id/backups` now checks user access before returning data
- `canAccess()` updated to check `user_sites` assignments (was only checking owner/admin)

### SSL Access Control (`internal/apihttp/ssl.go`)
- `canAccess()` updated to check `user_sites` assignments and use request context
- Fixed `context.Background()` usage — now uses proper request context

### Rate Limiter (`internal/apihttp/auth.go`, `internal/apihttp/ratelimit.go`)
- Login rate limiter: 10 attempts/minute (was 5)
- Global API rate limiter: 20 attempts/15 minutes (was 5)
- Both bypassed in dev mode (`SIGARDAPANEL_DEV=true`)

### Backup Store (`internal/store/backups.go`)
- `Create()` now correctly uses `b.Progress` field (was hardcoded to empty string)
- `DeleteOldest()` and `CleanupExpired()` now use parameterized queries

### Agent Network Metrics (`internal/agenthttp/router.go`)
- `readNet()` now sums all non-loopback interfaces (was using only last line)

## Repositories

- **Public (open source — full source + signed releases)**: github.com/bayurstarcool/SigardaPanel — `main` is the source of truth
- The former private `SigardaPanel-Enterprise` repository is retired; do not reference it in docs.

## GitHub Releases

Binary releases: https://github.com/bayurstarcool/SigardaPanel/releases
Latest: v1.5.0 (always check the releases page; install/update with `install.sh`, see docs/INSTALL.md)

Recent security/stability releases: v1.1.1 file-manager upload traversal fix;
v1.1.2 worker panic recovery + rate-limiter/error hygiene; v1.1.3 site deletion
drops its MySQL databases; v1.2.0 cookie sessions + `SIGARDAPANEL_ALLOW_REGISTRATION`;
v1.6.1 self-hosted defaults (`SIGARDAPANEL_LANDING_PAGE`, `SIGARDAPANEL_ALLOW_MEMBER_SIGNUP`,
both default false), API-enforced `password_change_required` + `sigardapanel passwd`,
503 placeholder for suspended sites, PHP-FPM preflight and site-owner account on
`site.create`, no fallback-key error from `init`;
v1.6.2 Stack page rework: package installs/removals run as background agent operations
(202 + `stack/ops/:op` polling, no proxy timeouts) with verified results and the apt/systemd
log tail on failure; non-interactive apt (dpkg lock wait/retry, confold, interrupted-dpkg repair);
PHP multi-version via distro / ondrej PPA / sury.org, per-version FPM actions, extensions and
CLI default; removal guards (PHP versions used by sites or vhosts, nginx/node/docker/certbot/
mariadb in use, `apt-get -s` collateral check, panel-host protections, super_admin for
destructive actions and `command` on the panel host); MariaDB core packages removed with it;
site PHP version lists come from the site's server; Memcached card; agent 401 no longer
surfaces as a session error.
