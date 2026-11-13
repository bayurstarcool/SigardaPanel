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
| GET | `/users/:id/usage` | user | Disk/sites usage: owned sites, measured on their servers (`disk_unmeasured_sites` = not in the total) |
| GET | `/users/:id/sites` | admin | List sites assigned |
| POST | `/users/:id/sites` | admin | Assign site to user |
| DELETE | `/users/:id/sites/:siteId` | admin | Unassign |
| PATCH | `/users/:id/sites/:siteId` | admin | Update permission |
| GET | `/sites/:id/users` | admin | List users on a site |

Disk limits (`disk_limit`, GB; 0 = unlimited) apply to the sites a user owns.
At or over the limit, actions that add data to those sites return 403: site
create, file write/create/upload/compress, deploy (also by webhook), git
checkout/rollback, backup restore, and transfer to that user. Usage is checked
every 5 minutes; the user and admins are notified at 80% and 100%. Writes
outside the panel (SSH/FTP, cron, PHP) are not blocked, only reported.
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
| GET | `/servers/:id/stack` | user | Installed software, PHP versions, distro, running stack operation |
| GET | `/servers/:id/benchmark` | user | Run benchmark |
| GET | `/servers/:id/metrics` | user | Time-series metrics |

## Stack (per-server)

| Method | Path | Auth | Description |
|---|---|---|---|
| POST | `/servers/:id/stack/action` | admin | `{package, action, version?, extension?}` — see below |
| GET | `/servers/:id/stack/ops/:op` | admin | Progress/result of a background package operation |
| POST | `/servers/:id/stack/restart` | admin | `{package, version?}` — same as action `restart` |
| GET | `/servers/:id/php-versions` | user | PHP-FPM versions installed on the server (`versions`, `details`) — used by the site forms |
| POST | `/servers/:id/command` | admin | `{command, timeout?}` (1–300 s, default 60); super_admin on the panel's own server |

Packages: `nginx`, `php`, `node`, `python`, `go`, `docker`, `certbot`, `mariadb` (alias `mysql`), `redis`, `memcached`, `supervisor`.

Actions:

- `install`, `uninstall`, `update`, `install-extension`, `uninstall-extension` run in the background
  on the agent: the answer is **202** with `{op_id, status: "running", …}`; poll
  `GET /servers/:id/stack/ops/:op_id` until `done` is true. `ok`/`status` (`succeeded` | `failed`)
  is set only after the agent verified the result (binary present, service running, PHP-FPM socket);
  `message` explains, `output` is the log tail (apt/systemd errors), `info` the component's detected state.
  One package operation per server at a time (409 otherwise). `version`: PHP `7.4`–`8.5` (required),
  Node.js major (`24`, `22`, `20`), Go release (e.g. `1.25.4`; default latest stable); other packages take
  no version. `extension` (PHP only): the suffix of `php<version>-<ext>`, e.g. `redis`.
- `start`, `stop`, `restart`, `reload` (synchronous, verified; PHP without `version` = every FPM, `stop`
  needs a version; nginx is config-tested first and a failing `nginx -t` is a 409), `set-default` (PHP CLI
  default via update-alternatives).
- `list-versions`, `list-available`, `list-extensions` (PHP, with `version`) — read-only.

Refusals (409, nothing changed): PHP version used by sites or by an nginx vhost; nginx while the server has
sites; node/docker while sites of that runtime exist; docker with running containers; certbot while sites
have certificates; mariadb while the panel manages databases there; any removal that would also remove
dependent packages. On the panel's own server (loopback agent URL) nginx cannot be stopped/removed and
certbot cannot be removed, and `uninstall`/`stop` need super_admin (403). Every mutating action is audited
(`stack_<action>`, refusals `stack_<action>_refused`, background results `stack_<action>_result`).

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
