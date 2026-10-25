# API Reference

Base URL: `http://127.0.0.1:7700/api/v1` (via nginx: `https://panel.example.com/api/v1`)

## Authentication

All endpoints (except public ones) require authentication. Browsers use the
HttpOnly `sp_session` cookie (set at login, SameSite=Strict); API clients use a
Bearer token:

```bash
curl -H "Authorization: Bearer $TOKEN" http://127.0.0.1:7700/api/v1/...
```

Get a token:
```bash
sigardapanel login --user admin --password 'xxx'
# or POST /api/v1/auth/login with {"username","password"}
```

Service tokens (machine-to-machine) use the same header with scoped permissions.

## Public Endpoints

| Method | Path | Description |
|---|---|---|
| GET | `/health` | API health |
| GET | `/version` | API version |
| POST | `/auth/login` | Login, get session token |
| POST | `/auth/register` | Public signup (disabled by default) |
| GET | `/agents/status-dashboard` | Public agent status |
| GET | `/agents/install` | Get install script (requires registration token header) |
| POST | `/webhooks/deploy/:site_id` | Git provider webhook |

## Auth Endpoints

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/auth/me` | user | Current user info |
| PUT | `/auth/profile` | user | Update profile |
| POST | `/auth/logout` | user | Revoke session |
| POST | `/auth/stop-impersonation` | user | Stop impersonating |
| GET | `/auth/2fa/status` | user | 2FA enabled? |
| POST | `/auth/2fa/setup` | user | Generate TOTP secret + QR |
| POST | `/auth/2fa/enable` | user | Enable 2FA (verify code) |
| POST | `/auth/2fa/disable` | user | Disable 2FA |
| POST | `/auth/2fa/verify` | user | Verify TOTP code |
| POST | `/auth/2fa/recovery-codes` | user | Generate recovery codes |

## Users

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/users` | admin | List users (admin sees own; super sees all) |
| POST | `/users` | admin | Create user |
| PATCH | `/users/:id` | admin | Update user (role, password, limits) |
| DELETE | `/users/:id` | admin | Delete user |
| GET | `/users/:id/usage` | user | Disk/sites usage |
| GET | `/users/:id/sites` | admin | List sites assigned |
| POST | `/users/:id/sites` | admin | Assign site to user |
| DELETE | `/users/:id/sites/:siteId` | admin | Unassign |
| PATCH | `/users/:id/sites/:siteId` | admin | Update permission |
| GET | `/sites/:id/users` | admin | List users on a site |
| POST | `/users/:id/impersonate` | admin | Impersonate (requires `RequireUserSession`) |

## Servers

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/servers` | user | List all servers |
| POST | `/servers` | admin | Register server |
| GET | `/servers/:id` | user | Get server |
| PATCH | `/servers/:id` | admin | Update |
| DELETE | `/servers/:id` | admin | Remove |
| GET | `/servers/health` | user | All servers health |
| GET | `/servers/:id/health` | user | One server health |
| POST | `/servers/:id/token` | admin | **Regenerate agent token** (sync DB + env) |
| GET | `/servers/:id/specs` | user | CPU, RAM, disk specs |
| GET | `/servers/:id/stack` | user | Installed software (nginx, PHP, etc.) |
| GET | `/servers/:id/benchmark` | user | Run benchmark |
| GET | `/servers/:id/metrics` | user | Time-series metrics |

## Stack (per-server)

| Method | Path | Auth | Description |
|---|---|---|---|
| POST | `/servers/:id/stack/action` | user | Install/restart/stop/update package, or manage versions (`list-versions`, `list-available`) |
| POST | `/servers/:id/stack/restart` | user | Restart stack component |

Supported packages: `nginx`, `php` (with `version` field), `node`, `python`, `go`, `docker`, `certbot`, `mariadb`/`mysql`, `redis`, `memcached`, `supervisor`

Actions: `install`, `update`, `uninstall`, `restart`, `start`, `stop`, `switch-version`, `list-versions`, `list-available`

## Sites

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/sites` | user | List (filtered by owner if not admin) |
| POST | `/sites` | user | Create site (supports `runtime`: `static`, `php` with `php_version`, `node`, `python`, `go`, `proxy`, `docker`) |
| GET | `/sites/:id` | owner | Get site |
| PATCH | `/sites/:id/config` | owner | Update site config |
| DELETE | `/sites/:id` | owner | Delete |
| POST | `/sites/:id/deploy` | owner | Trigger deploy |
| POST | `/sites/:id/reload-nginx` | owner | Reload nginx |
| POST | `/sites/:id/transfer` | owner | Transfer to another user |
| GET | `/sites/:id/ssl` | user | SSL status |
| POST | `/sites/:id/ssl/issue` | owner | Issue Let's Encrypt cert |
| POST | `/sites/:id/ssl/renew` | owner | Renew cert |
| GET | `/sites/:id/files` | owner | List files |
| GET | `/sites/:id/files/read` | owner | Read file |
| POST | `/sites/:id/files/write` | owner | Write file |
| POST | `/sites/:id/files/create` | owner | Create file/dir |
| DELETE | `/sites/:id/files` | owner | Delete |
| POST | `/sites/:id/files/rename` | owner | Rename/move |
| GET | `/sites/:id/logs` | owner | Tail access/error logs |
| GET | `/sites/:id/usage` | owner | Disk/quota usage |

## Databases

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/databases` | user | List DBs for a site |
| POST | `/databases` | user | Create DB |
| PATCH | `/databases/:id` | user | Update (charset, etc.) |
| DELETE | `/databases/:id` | user | Delete |
| GET | `/db-users` | user | List DB users |
| POST | `/databases/:id/users` | user | Create DB user |
| POST | `/databases/:id/users/:userId/rotate-password` | user | Rotate password |
| GET | `/databases/:id/pma-credentials` | user | Get phpMyAdmin login |
| POST | `/pma/ticket` | user | Generate phpMyAdmin auth ticket |
| GET | `/pma/t/:ticket` | (no auth) | phpMyAdmin entry |

## Backups

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/backup-providers` | user | List B2/S3 providers |
| POST | `/backup-providers` | admin | Add provider |
| PATCH | `/backup-providers/:id` | admin | Update |
| DELETE | `/backup-providers/:id` | admin | Remove |
| GET | `/backup-providers/:id/usage` | user | Cloud storage usage |
| POST | `/backup-providers/:id/test` | admin | Test connection |
| GET | `/sites/:id/backups` | user | List backups |
| POST | `/sites/:id/backups` | user | Create backup |
| DELETE | `/backups/:id` | user | Delete |
| POST | `/backups/:id/restore` | user | Restore |

## Docker (per-server, agent proxies)

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/servers/:id/docker/containers` | user | List |
| POST | `/servers/:id/docker/container/start` | user | Start |
| POST | `/servers/:id/docker/container/stop` | user | Stop |
| POST | `/servers/:id/docker/container/restart` | user | Restart |
| POST | `/servers/:id/docker/container/remove` | user | Remove |
| POST | `/servers/:id/docker/container/create` | user | Create |
| GET | `/servers/:id/docker/container/logs` | user | Logs |
| POST | `/servers/:id/docker/container/exec` | user | Exec command |
| GET | `/servers/:id/docker/images` | user | List |
| GET | `/servers/:id/docker/volumes` | user | List |
| GET | `/servers/:id/docker/networks` | user | List |

## Firewall (per-server)

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/servers/:id/firewall/status` | user | UFW status |
| POST | `/servers/:id/firewall/enable` | user | Enable UFW |
| POST | `/servers/:id/firewall/disable` | user | Disable UFW |
| POST | `/servers/:id/firewall/allow` | user | Add allow rule |
| POST | `/servers/:id/firewall/deny` | user | Add deny rule |
| POST | `/servers/:id/firewall/delete` | user | Delete rule by number |
| GET | `/servers/:id/firewall/rules` | user | List rules |
| POST | `/servers/:id/firewall/reset` | user | Reset all rules |

## Fail2ban & Security (per-server)

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/servers/:id/security/overview` | user | Live security status (UFW, DDoS protection, banned attackers, DB/file integrity) |
| GET | `/servers/:id/fail2ban/status` | user | Status |
| GET | `/servers/:id/fail2ban/jail` | user | Jail status (query: `?name=sshd`) |
| POST | `/servers/:id/fail2ban/unban` | user | Unban IP |
| POST | `/servers/:id/fail2ban/ban` | user | Ban IP |
| POST | `/servers/:id/fail2ban/enable` | user | Enable |
| POST | `/servers/:id/fail2ban/disable` | user | Disable |

## Redis (per-server)

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/servers/:id/redis/stats` | user | Stats |
| GET | `/servers/:id/redis/info` | user | Info |
| POST | `/servers/:id/redis/flush` | user | Flush all |
| POST | `/servers/:id/redis/flushdb` | user | Flush current DB |

## Other

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/jobs` | user | List background jobs |
| GET | `/jobs/:id` | user | Job status |
| POST | `/jobs/:id/cancel` | user | Cancel |
| GET | `/jobs/:id/logs` | user | Job logs |
| GET | `/license` | user | License info |
| GET | `/license/features` | user | Feature flags |
| GET | `/plans` | user | List plans |
| GET | `/alerts` | user | List alerts |
| GET | `/metrics/server` | user | Server metrics |
| GET | `/settings/retention` | user | Backup retention settings |
| POST | `/cloudflare/zones` | user | List CF zones (if configured) |
| GET | `/agents/install` | (token header) | Get install script |

## Error Format

All errors:
```json
{
  "error": {"code": "invalid_credentials", "message": "Invalid credentials"},
  "request_id": "req_abc123"
}
```

Common error codes:
- `unauthorized` (401)
- `forbidden` (403)
- `not_found` (404)
- `bad_request` (400)
- `too_many_requests` (429)
- `internal_error` (500)
- `invalid_credentials` (401)
- `2fa_required` (401)
- `service_token_lacks_scope` (403)
