# MVP Scope & Current Status

This document tracks the original MVP scope and current implementation status.

## Current Version: v0.5.x

The project has evolved beyond the original MVP scope. This document now serves as a reference for what was planned vs what is implemented.

## Architecture

| Component | Technology |
|-----------|------------|
| Backend API | Go (Echo framework) |
| Agent | Go |
| CLI | Go (unified binary) |
| Dashboard | SvelteKit + Tailwind CSS |
| Database | SQLite (WAL mode) |
| Reverse Proxy | Nginx |

## Services

| Service  | Port  | Description |
|----------|-------|-------------|
| API      | 7700  | Panel REST API |
| Agent    | 7710  | Agent per VPS |
| Frontend | 7720  | SvelteKit dashboard |

---

## Feature Status

### ✅ Implemented (Beyond MVP)

#### Authentication & Users
- Login/logout with session management
- Password hashing (Argon2id/bcrypt)
- Roles: `super_admin`, `admin`, `user`
- 2FA/TOTP with recovery codes
- User impersonation (admin)
- User registration
- Profile update

#### Server Management
- Register servers
- Agent registration via token
- Agent health checks (all servers + single)
- Server status sync (online/offline/degraded)
- Server specs and benchmarks
- Agent token regeneration
- Server update/delete

#### Site Management
- Create site with domain
- List/detail site
- Delete site
- Update site configuration
- Site transfer between servers
- Reload nginx config
- Site usage stats
- Site isolation (owner-based)

#### User-Site Access Control
- Assign/unassign users to sites
- Permission levels: `read`, `write`, `admin`
- Linux system user per panel user
- Per-site terminal (runs as system_user)

#### Runtime Support
- Static sites
- Node.js applications (systemd + PM2)
- Go binaries (systemd)
- Python applications (systemd)
- PHP applications (per-site version + php.ini)
- Reverse proxy passthrough
- Docker applications

#### SSL
- Issue Let's Encrypt certificates
- Renew certificates
- Renew all certificates (bulk)
- SSL status and expiry check
- Cloudflare DNS integration

#### Deployment
- Git deploy (repo URL, branch, checkout, rollback)
- Webhook deploy (signature validated)
- Git branch management
- Git commit log
- Deploy key management

#### App Process Management
- Start/stop/restart app
- App status check
- App config update
- Resource usage monitoring
- Resource limits configuration

#### Job Queue
- Async operations for site creation, deployment, SSL, deletion
- Job statuses: queued, running, succeeded, failed, canceled
- Job logs (per-line, stdout/stderr)
- Cancel jobs
- Automatic retry with backoff (max 3 attempts)

#### File Manager
- List files/directories
- Read/write file content
- Create file/directory
- Delete files
- Rename/move files
- Compress files
- Upload/download files

#### Backup System
- File backups (per-site)
- Database backups
- Bulk backup creation
- Backup restore
- Backup download
- Backup storage providers (S3/B2/R2/Wasabi/DigitalOcean)
- Backup schedules (deprecated)
- Backup configs (global, multi-site)
- Retention settings
- Deduplication (skip if backup already running)

#### Database Management
- Create/list/delete databases
- Create/list DB users
- Multiple database server connections (MariaDB/MySQL)

#### Server Stack Management
- Install/uninstall software (Nginx, PHP, Node, etc.)
- Restart stack components
- Execute commands on server

#### Docker Management
- Container management (list, create, start, stop, restart, remove)
- Container logs and exec
- Image management (list, search, pull, remove)
- Docker Compose (up/down)
- Volume management (list, create, remove)
- Network management (list, create, remove)
- Docker info and disk usage

#### Firewall (UFW)
- Status check
- Enable/disable firewall
- Allow/deny/delete rules
- List rules
- Reset firewall

#### Fail2ban Management
- Status check
- Jail list with stats
- Ban/unban IPs
- Enable/disable jails

#### Redis Management
- Stats and info
- Flush all/DB keys

#### PM2 Process Management
- Process management

#### Bot Blocker
- Bot blocker management

#### Monitoring
- Server metrics (CPU, RAM, disk, network, swap)
- Metrics history with range queries
- GPU metrics
- Latest metrics endpoint
- Metrics ingestion from agent

#### Notifications
- In-app notifications (per-user)
- Unread count
- Mark read/all read
- Delete notifications

#### Alert System
- Alert channels (Telegram, Discord, email, webhook)
- Alert events with severity

#### Cloudflare Integration
- API config management
- Zone management
- DNS record CRUD
- Bulk DNS operations
- DNS import
- Cache purge
- Analytics

#### Vhost Management
- Custom Nginx vhost config
- Config validation

#### Varnish Cache
- Config per site
- Cache purge

#### Site Security
- Basic auth per site
- Blocked IPs per site

#### IP Restrictions
- Per-site IP restrictions (CRUD)

#### SSH/FTP Users
- Create/list/delete SSH/FTP users per site
- SSH key generation

#### Cron Jobs
- Per-site cron job management

#### CLI
- Full CLI for all operations
- Multiple output formats (table, json, yaml)

#### License & Billing
- License activation/deactivation
- Feature gating per tier
- Plans management
- License orders

#### Audit Logging
- All critical operations logged
- Actor, action, resource, details

### 🚫 Non-Goals (Not Implemented)

- Email/mailbox features (planned for future)
- Plugin system
- Full cPanel clone
- Authoritative DNS server
- 2FA was originally planned for Beta — now shipped in v0.5.x

## Target Operating Systems

| OS | Priority |
|----|----------|
| Ubuntu 22.04 LTS | Primary |
| Ubuntu 24.04 LTS | Primary |
| Debian 12 | Secondary |

## Definition of Done (MVP)

- [x] Install panel on a single VPS
- [x] Add another server via agent
- [x] Create a static site with a domain
- [x] Issue a valid SSL certificate
- [x] Deploy a Node.js application
- [x] Deploy a Go application
- [x] View site/service logs
- [x] Async operations visible in job logs
- [x] Audit log recorded for critical operations
- [x] CLI can execute basic workflows without dashboard

## Primary Risks

- Command injection from domain, path, or build commands
- Agent compromise due to root privileges
- Data loss during delete/restore operations
- Reverse proxy configuration errors causing downtime
- Secret leakage through logs, UI, or CLI output

## Mitigation Strategy

- Strict validation of domains, paths, and service names
- Command wrappers with argument arrays, not shell strings
- Configuration testing before proxy reload
- Dry-run and confirmation for delete operations
- Secret masking in all outputs
- Mandatory audit logging
