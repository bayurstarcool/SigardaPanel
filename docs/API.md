# API Reference

This document defines the actual API for SigardaPanel v0.5.x. The dashboard and CLI use the same API endpoints.

## API Principles

- All public endpoints use the `/api/v1` prefix
- Consistent JSON response format
- All responses include a `request_id` for tracing
- All mutation endpoints perform RBAC checks and audit logging
- All list endpoints support pagination via `limit` and `offset`
- Secrets are never returned in full after creation

## Authentication

### Methods

- **Cookie session** — for dashboard access
- **Bearer token** — for CLI and API access

### Headers

```http
Authorization: Bearer <token>
Content-Type: application/json
```

## Response Format

### Success (Single Resource)

```json
{
  "data": {},
  "request_id": "req_123"
}
```

### Success (List)

```json
{
  "data": [],
  "pagination": {
    "limit": 25,
    "offset": 0,
    "total": 100
  },
  "request_id": "req_123"
}
```

### Error

```json
{
  "error": {
    "code": "unauthorized",
    "message": "Invalid credentials"
  },
  "request_id": "req_123"
}
```

## HTTP Status Codes

| Code | Description |
|------|-------------|
| `200` | Success |
| `201` | Resource created |
| `202` | Job accepted (async) |
| `400` | Invalid input |
| `401` | Not authenticated |
| `403` | Permission denied |
| `404` | Resource not found |
| `409` | Conflict / invalid state |
| `422` | Validation failed |
| `429` | Rate limited |
| `500` | Server error |
| `502` | Agent or target server error |
| `504` | Agent or job timeout |
| `501` | Not implemented |

## Rate Limiting

| Endpoint | Limit |
|----------|-------|
| Login (`/auth/login`) | 10 attempts per 15 minutes per IP |
| API endpoints | 100 requests per minute per IP |
| Dev mode | Rate limiters bypassed when `SIGARDAPANEL_DEV=true` |

---

## System Endpoints

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/health` | No | API health check |
| `GET` | `/api/v1/health` | No | API health check (alias) |
| `GET` | `/api/v1/version` | No | API version info |

## Authentication Endpoints

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `POST` | `/api/v1/auth/login` | No | Login (rate-limited: 10/15min) |
| `POST` | `/api/v1/auth/register` | No | Register new user |
| `GET` | `/api/v1/auth/me` | Yes | Current user info |
| `PUT` | `/api/v1/auth/profile` | Yes | Update own profile |
| `POST` | `/api/v1/auth/logout` | Yes | Revoke current session |
| `POST` | `/api/v1/auth/stop-impersonation` | Yes | Stop impersonating user |

## Two-Factor Authentication

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `POST` | `/api/v1/auth/2fa/setup` | Yes | Setup 2FA (generate secret) |
| `POST` | `/api/v1/auth/2fa/enable` | Yes | Enable 2FA after setup |
| `POST` | `/api/v1/auth/2fa/disable` | Yes | Disable 2FA |
| `POST` | `/api/v1/auth/2fa/verify` | Yes | Verify 2FA code |
| `GET` | `/api/v1/auth/2fa/status` | Yes | 2FA status check |
| `POST` | `/api/v1/auth/2fa/recovery-codes` | Yes | Generate recovery codes |

## User Management

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/api/v1/users` | Admin | List users |
| `POST` | `/api/v1/users` | Admin | Create user |
| `PATCH` | `/api/v1/users/:id` | Admin | Update user |
| `DELETE` | `/api/v1/users/:id` | Admin | Delete user |
| `GET` | `/api/v1/users/:id/usage` | Auth | Check user usage/quota |
| `POST` | `/api/v1/users/:id/impersonate` | Admin | Login as user |
| `POST` | `/api/v1/users/:id/assign-plan` | Admin | Assign plan to user |

## User ↔ Site Mapping

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/api/v1/users/:id/sites` | Admin | List user's assigned sites |
| `POST` | `/api/v1/users/:id/sites` | Admin | Assign user to site |
| `DELETE` | `/api/v1/users/:id/sites/:siteId` | Admin | Unassign user from site |
| `PATCH` | `/api/v1/users/:id/sites/:siteId` | Admin | Update permission level |
| `GET` | `/api/v1/sites/:id/users` | Admin | List site's assigned users |

**Permission levels:** `read`, `write`, `admin`

## Server Management

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/api/v1/servers` | Auth | List servers |
| `POST` | `/api/v1/servers` | Admin | Create server |
| `GET` | `/api/v1/servers/:id` | Auth | Server details |
| `PATCH` | `/api/v1/servers/:id` | Admin | Update server |
| `DELETE` | `/api/v1/servers/:id` | Admin | Remove server |
| `GET` | `/api/v1/servers/health` | Auth | Health status (all servers) |
| `GET` | `/api/v1/servers/:id/health` | Auth | Health status (single server) |
| `POST` | `/api/v1/servers/:id/token` | Admin | Regenerate agent token |
| `GET` | `/api/v1/servers/:id/specs` | Auth | Server hardware specs |
| `GET` | `/api/v1/servers/:id/benchmark` | Admin | Server benchmark |

## Agent Registration

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `POST` | `/api/v1/agents/register` | Token | Register agent with registration token |
| `GET` | `/api/v1/agents/install` | No | Get agent install script |
| `GET` | `/api/v1/agents/status-dashboard` | No | Status dashboard script |
| `POST` | `/api/v1/agents/tokens` | Admin | Create registration token |

## Site Management

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/api/v1/sites` | Auth | List sites (filters: `server_id`, `owner_id`, `runtime`, `status`, `search`) |
| `POST` | `/api/v1/sites` | Auth | Create site |
| `GET` | `/api/v1/sites/:id` | Auth | Site details |
| `DELETE` | `/api/v1/sites/:id` | Auth | Delete site |
| `POST` | `/api/v1/sites/:id/config` | Auth | Update site configuration |
| `POST` | `/api/v1/sites/:id/deploy` | Auth | Deploy site |
| `POST` | `/api/v1/sites/:id/reload-nginx` | Auth | Reload nginx config |
| `POST` | `/api/v1/sites/:id/transfer` | Auth | Transfer site to another server |
| `GET` | `/api/v1/sites/:id/usage` | Auth | Site resource usage |
| `GET` | `/api/v1/sites/:id/logs` | Auth | Tail site logs |

## App Process Management

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/api/v1/sites/:id/app/status` | Auth | App process status |
| `POST` | `/api/v1/sites/:id/app/start` | Auth | Start app |
| `POST` | `/api/v1/sites/:id/app/stop` | Auth | Stop app |
| `POST` | `/api/v1/sites/:id/app/restart` | Auth | Restart app |
| `PUT` | `/api/v1/sites/:id/app/config` | Auth | Update app config |
| `GET` | `/api/v1/sites/:id/app/resource-usage` | Auth | App resource usage |
| `PUT` | `/api/v1/sites/:id/app/resource-limits` | Admin | Update resource limits |

## PHP Configuration

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/api/v1/sites/:id/php/config` | Auth | Get PHP version + php.ini |
| `PUT` | `/api/v1/sites/:id/php/config` | Auth | Update PHP config |

## Vhost Management

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/api/v1/sites/:id/vhost` | Auth | Get vhost config |
| `PUT` | `/api/v1/sites/:id/vhost` | Auth | Update vhost config |
| `POST` | `/api/v1/sites/:id/vhost/validate` | Auth | Validate vhost config |

## Varnish Cache

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/api/v1/sites/:id/varnish` | Auth | Get varnish config |
| `POST` | `/api/v1/sites/:id/varnish/purge` | Auth | Purge varnish cache |

## Site Security

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/api/v1/sites/:id/security` | Auth | Get security config (basic auth, blocked IPs) |
| `PUT` | `/api/v1/sites/:id/security` | Auth | Update security config |

## IP Restrictions

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/api/v1/sites/:id/ip-restrictions` | Auth | List IP restrictions |
| `POST` | `/api/v1/sites/:id/ip-restrictions` | Auth | Create IP restriction |
| `DELETE` | `/api/v1/sites/:id/ip-restrictions/:restriction_id` | Auth | Delete IP restriction |

## SSH/FTP Users

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/api/v1/sites/:id/ssh-ftp` | Admin | List SSH/FTP users |
| `POST` | `/api/v1/sites/:id/ssh-ftp` | Admin | Create SSH/FTP user |
| `DELETE` | `/api/v1/sites/:id/ssh-ftp/:ssh_id` | Admin | Delete SSH/FTP user |
| `GET` | `/api/v1/sites/:id/ssh-ftp/:ssh_id/key` | Admin | Get SSH private key |

## SSH Info

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/api/v1/sites/:id/ssh/info` | Auth | Get SSH connection details |

## Cron Jobs

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/api/v1/sites/:id/cron` | Auth | List cron jobs |
| `POST` | `/api/v1/sites/:id/cron` | Admin | Create cron job |
| `DELETE` | `/api/v1/sites/:id/cron/:cron_id` | Admin | Delete cron job |

## SSL

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/api/v1/sites/:id/ssl` | Auth | SSL status and expiry |
| `POST` | `/api/v1/sites/:id/ssl/issue` | Auth | Issue SSL certificate |
| `POST` | `/api/v1/sites/:id/ssl/renew` | Auth | Renew SSL certificate |
| `POST` | `/api/v1/ssl/renew-all` | Admin | Renew all SSL certificates |

## File Manager

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/api/v1/sites/:id/files` | Auth | List files/directories |
| `GET` | `/api/v1/sites/:id/files/read` | Auth | Read file content |
| `POST` | `/api/v1/sites/:id/files/write` | Auth | Write file content |
| `POST` | `/api/v1/sites/:id/files/create` | Auth | Create file/directory |
| `DELETE` | `/api/v1/sites/:id/files` | Auth | Delete file/directory |
| `POST` | `/api/v1/sites/:id/files/rename` | Auth | Rename file/directory |
| `POST` | `/api/v1/sites/:id/files/move` | Auth | Move files |
| `POST` | `/api/v1/sites/:id/files/compress` | Auth | Compress files |
| `POST` | `/api/v1/sites/:id/files/upload` | Auth | Upload file |
| `GET` | `/api/v1/sites/:id/files/download` | Auth | Download file |

## Git Deploy

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/api/v1/sites/:id/git/branches` | Auth | List git branches |
| `GET` | `/api/v1/sites/:id/git/log` | Auth | Git commit log |
| `POST` | `/api/v1/sites/:id/git/rollback` | Admin | Rollback to commit |
| `POST` | `/api/v1/sites/:id/git/checkout` | Admin | Checkout branch |

## Deployment Endpoints

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `POST` | `/api/v1/sites/:id/deploy` | Auth | Deploy site (git or artifact) |
| `POST` | `/api/v1/webhooks/deploy/:site_id` | Webhook | Git deploy webhook (signature validated) |

## Backup Management

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `POST` | `/api/v1/sites/:id/backups` | Auth | Create site backup |
| `GET` | `/api/v1/sites/:id/backups` | Auth | List site backups |
| `POST` | `/api/v1/sites/:id/backups/database` | Auth | Create database backup |
| `GET` | `/api/v1/backups` | Auth | List all backups |
| `GET` | `/api/v1/backups/:id/download` | Auth | Download backup |
| `DELETE` | `/api/v1/backups/:id` | Auth | Delete backup |
| `POST` | `/api/v1/backups/:id/restore` | Auth | Restore backup |
| `POST` | `/api/v1/backups/bulk` | Admin | Bulk create backups |

## Backup Storage Providers

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/api/v1/backup-providers` | Admin | List providers |
| `POST` | `/api/v1/backup-providers` | Admin | Create provider (S3/B2/R2/Wasabi) |
| `PATCH` | `/api/v1/backup-providers/:id` | Admin | Update provider |
| `DELETE` | `/api/v1/backup-providers/:id` | Admin | Delete provider |
| `GET` | `/api/v1/backup-providers/:id/usage` | Admin | Provider usage stats |
| `POST` | `/api/v1/backup-providers/:id/test` | Admin | Test connection |

## Backup Schedules (Deprecated)

> Deprecated: use backup-configs instead.

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `POST` | `/api/v1/sites/:id/schedules` | Auth | Create schedule |
| `GET` | `/api/v1/sites/:id/schedules` | Auth | List schedules |
| `PATCH` | `/api/v1/schedules/:id` | Auth | Update schedule |
| `DELETE` | `/api/v1/schedules/:id` | Auth | Delete schedule |

## Backup Configs

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/api/v1/backup-configs` | Admin | List backup configs |
| `POST` | `/api/v1/backup-configs` | Admin | Create backup config |
| `PATCH` | `/api/v1/backup-configs/:id` | Admin | Update backup config |
| `DELETE` | `/api/v1/backup-configs/:id` | Admin | Delete backup config |
| `POST` | `/api/v1/backup-configs/:id/run` | Admin | Run backup config now |

## Retention Settings

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/api/v1/settings/retention` | Admin | Get retention settings |
| `PUT` | `/api/v1/settings/retention` | Admin | Set retention settings |

## Database Management

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/api/v1/databases` | Auth | List all databases |
| `POST` | `/api/v1/sites/:id/databases` | Auth | Create database |
| `GET` | `/api/v1/sites/:id/databases` | Auth | List site databases |
| `GET` | `/api/v1/databases/:id` | Auth | Get database details |
| `PATCH` | `/api/v1/databases/:id` | Auth | Update database |
| `DELETE` | `/api/v1/databases/:id` | Auth | Delete database |
| `POST` | `/api/v1/databases/:id/users` | Auth | Create DB user |
| `GET` | `/api/v1/databases/:id/users` | Auth | List DB users |
| `DELETE` | `/api/v1/databases/:id/users/:user_id` | Auth | Delete DB user |
| `POST` | `/api/v1/databases/:id/users/:user_id/rotate-password` | Auth | Rotate DB user password |
| `POST` | `/api/v1/databases/:id/grant-user` | Auth | Grant existing user to database |
| `GET` | `/api/v1/databases/:id/available-users` | Auth | List available MySQL users |
| `GET` | `/api/v1/databases/:id/pma-credentials` | Auth | Get phpMyAdmin credentials |
| `POST` | `/api/v1/databases/exec-sql` | Admin | Execute SQL query |
| `POST` | `/api/v1/databases/phpmyadmin-reset-passwords` | Admin | Reset phpMyAdmin passwords |

## Database Servers

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/api/v1/db-servers` | Admin | List database servers |
| `POST` | `/api/v1/db-servers` | Admin | Add database server |
| `PUT` | `/api/v1/db-servers/:id` | Admin | Update database server |
| `DELETE` | `/api/v1/db-servers/:id` | Admin | Remove database server |

## Job Management

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/api/v1/jobs` | Auth | List jobs |
| `GET` | `/api/v1/jobs/:id` | Auth | Job details |
| `POST` | `/api/v1/jobs/:id/cancel` | Auth | Cancel job |
| `POST` | `/api/v1/jobs/:id/logs` | Auth | Append job log |
| `GET` | `/api/v1/jobs/:id/logs` | Auth | Get job logs (since line) |

## Metrics

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `POST` | `/api/v1/servers/:id/metrics` | Auth | Ingest metrics (agent reports) |
| `GET` | `/api/v1/servers/:id/metrics/latest` | Auth | Latest metrics |
| `GET` | `/api/v1/servers/:id/metrics` | Auth | Metrics history (range query) |
| `GET` | `/api/v1/servers/:id/gpu` | Auth | GPU metrics |

## Service Tokens

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/api/v1/service-tokens` | Admin | List service tokens |
| `POST` | `/api/v1/service-tokens` | Admin | Create service token |
| `DELETE` | `/api/v1/service-tokens/:id` | Admin | Revoke service token |

## Alert Channels & Alerts

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/api/v1/alert-channels` | Admin | List alert channels |
| `POST` | `/api/v1/alert-channels` | Admin | Create alert channel |
| `DELETE` | `/api/v1/alert-channels/:id` | Admin | Delete alert channel |
| `GET` | `/api/v1/alerts` | Admin | List alerts |

## Notifications

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/api/v1/notifications` | Auth | List notifications |
| `GET` | `/api/v1/notifications/unread-count` | Auth | Unread count |
| `POST` | `/api/v1/notifications/:id/read` | Auth | Mark as read |
| `POST` | `/api/v1/notifications/read-all` | Auth | Mark all as read |
| `DELETE` | `/api/v1/notifications/:id` | Auth | Delete notification |
| `DELETE` | `/api/v1/notifications/read` | Auth | Delete all read |

## Docker Management

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/api/v1/servers/:id/docker/containers` | Auth | List containers |
| `POST` | `/api/v1/servers/:id/docker/container/create` | Admin | Create container |
| `POST` | `/api/v1/servers/:id/docker/container/start` | Admin | Start container |
| `POST` | `/api/v1/servers/:id/docker/container/stop` | Admin | Stop container |
| `POST` | `/api/v1/servers/:id/docker/container/restart` | Admin | Restart container |
| `POST` | `/api/v1/servers/:id/docker/container/remove` | Admin | Remove container |
| `GET` | `/api/v1/servers/:id/docker/container/logs` | Auth | Container logs |
| `POST` | `/api/v1/servers/:id/docker/container/exec` | Admin | Exec into container |
| `GET` | `/api/v1/servers/:id/docker/images` | Auth | List images |
| `GET` | `/api/v1/servers/:id/docker/images/search` | Auth | Search images |
| `POST` | `/api/v1/servers/:id/docker/image/pull` | Admin | Pull image |
| `POST` | `/api/v1/servers/:id/docker/image/remove` | Admin | Remove image |
| `POST` | `/api/v1/servers/:id/docker/compose/up` | Admin | Docker compose up |
| `POST` | `/api/v1/servers/:id/docker/compose/down` | Admin | Docker compose down |
| `GET` | `/api/v1/servers/:id/docker/volumes` | Auth | List volumes |
| `POST` | `/api/v1/servers/:id/docker/volume/create` | Admin | Create volume |
| `POST` | `/api/v1/servers/:id/docker/volume/remove` | Admin | Remove volume |
| `GET` | `/api/v1/servers/:id/docker/networks` | Auth | List networks |
| `POST` | `/api/v1/servers/:id/docker/network/create` | Admin | Create network |
| `POST` | `/api/v1/servers/:id/docker/network/remove` | Admin | Remove network |
| `GET` | `/api/v1/servers/:id/docker/info` | Auth | Docker info |
| `GET` | `/api/v1/servers/:id/docker/disk-usage` | Auth | Docker disk usage |

## Stack Management

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/api/v1/servers/:id/stack` | Auth | Stack info (installed software) |
| `POST` | `/api/v1/servers/:id/stack/action` | Admin | Install/uninstall stack component |
| `POST` | `/api/v1/servers/:id/stack/restart` | Admin | Restart stack component |
| `POST` | `/api/v1/servers/:id/command` | Admin | Execute command on server |

## Firewall (UFW)

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/api/v1/servers/:id/firewall/status` | Auth | Firewall status |
| `POST` | `/api/v1/servers/:id/firewall/enable` | Admin | Enable firewall |
| `POST` | `/api/v1/servers/:id/firewall/disable` | Admin | Disable firewall |
| `POST` | `/api/v1/servers/:id/firewall/allow` | Admin | Allow port/protocol |
| `POST` | `/api/v1/servers/:id/firewall/deny` | Admin | Deny port/protocol |
| `POST` | `/api/v1/servers/:id/firewall/delete` | Admin | Delete rule |
| `GET` | `/api/v1/servers/:id/firewall/rules` | Auth | List firewall rules |
| `POST` | `/api/v1/servers/:id/firewall/reset` | Admin | Reset firewall |

## Fail2ban Management

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/api/v1/servers/:id/fail2ban/status` | Auth | Fail2ban status |
| `GET` | `/api/v1/servers/:id/fail2ban/jail` | Auth | Jail list with stats |
| `POST` | `/api/v1/servers/:id/fail2ban/ban` | Admin | Ban IP |
| `POST` | `/api/v1/servers/:id/fail2ban/unban` | Admin | Unban IP |
| `POST` | `/api/v1/servers/:id/fail2ban/enable` | Admin | Enable jail |
| `POST` | `/api/v1/servers/:id/fail2ban/disable` | Admin | Disable jail |

## PM2 Process Management

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `POST` | `/api/v1/servers/:id/pm2/manage` | Admin | PM2 process management |

## Redis Management

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/api/v1/servers/:id/redis/stats` | Auth | Redis stats |
| `GET` | `/api/v1/servers/:id/redis/info` | Auth | Redis info |
| `POST` | `/api/v1/servers/:id/redis/flush` | Admin | Flush all Redis keys |
| `POST` | `/api/v1/servers/:id/redis/flushdb` | Admin | Flush current DB |

## Bot Blocker

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `POST` | `/api/v1/servers/:id/bot-blocker/manage` | Admin | Manage bot blocker |

## Cloudflare Integration

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/api/v1/cloudflare/config` | Auth | Get Cloudflare config |
| `PUT` | `/api/v1/cloudflare/config` | Auth | Save Cloudflare config |
| `DELETE` | `/api/v1/cloudflare/config` | Auth | Delete Cloudflare config |
| `GET` | `/api/v1/cloudflare/verify` | Auth | Verify API token |
| `GET` | `/api/v1/cloudflare/zones` | Auth | List zones |
| `GET` | `/api/v1/cloudflare/zones/search` | Auth | Search zones |
| `GET` | `/api/v1/cloudflare/zones/:id` | Auth | Get zone details |
| `GET` | `/api/v1/cloudflare/zones/:id/dns` | Auth | List DNS records |
| `POST` | `/api/v1/cloudflare/zones/:id/dns` | Auth | Create DNS record |
| `PUT` | `/api/v1/cloudflare/zones/:id/dns/:recordId` | Auth | Update DNS record |
| `PATCH` | `/api/v1/cloudflare/zones/:id/dns/:recordId` | Auth | Patch DNS record |
| `DELETE` | `/api/v1/cloudflare/zones/:id/dns/:recordId` | Auth | Delete DNS record |
| `POST` | `/api/v1/cloudflare/zones/:id/dns/bulk-delete` | Auth | Bulk delete DNS records |
| `POST` | `/api/v1/cloudflare/zones/:id/dns/import` | Auth | Import DNS records |
| `POST` | `/api/v1/cloudflare/zones/:id/purge-cache` | Auth | Purge cache |
| `GET` | `/api/v1/cloudflare/zones/:id/analytics` | Auth | Zone analytics |
| `GET` | `/api/v1/cloudflare/zones/:id/analytics/range` | Auth | Analytics over range |

## License & Feature Gating

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/api/v1/license` | Auth | Get license info |
| `POST` | `/api/v1/license/activate` | Admin | Activate license |
| `DELETE` | `/api/v1/license` | Admin | Deactivate license |
| `GET` | `/api/v1/license/features` | Auth | Get enabled features |

## Plans

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/api/v1/plans` | Auth | List plans |
| `POST` | `/api/v1/plans` | Admin | Create plan |
| `GET` | `/api/v1/plans/:id` | Auth | Get plan details |
| `PATCH` | `/api/v1/plans/:id` | Admin | Update plan |
| `DELETE` | `/api/v1/plans/:id` | Admin | Delete plan |

## License Orders

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/api/v1/orders` | Auth | List orders |
| `POST` | `/api/v1/orders` | Auth | Create order |
| `GET` | `/api/v1/orders/:id` | Auth | Get order details |
| `PATCH` | `/api/v1/orders/:id` | Admin | Update order status |

## WebSocket Endpoints

| Endpoint | Auth | Description |
|----------|------|-------------|
| `/api/ws/terminal/:id?user=<system_user>` | Agent token | PTY terminal (SSH session) |
| `/api/ws/terminal/:id?session=<session_id>` | Agent token | Reconnect to existing PTY |

**Terminal Features:**
- Full TUI support (htop, vim, nano, tmux)
- Session persistence across reconnects
- 64KB pending buffer when no client attached
- Per-site terminal runs as `system_user` (not root)
- Sticky keyboard shortcuts (Ctrl+C/Z/D/L/U, Tab, Esc, arrows)
- Mobile-optimized input handling
