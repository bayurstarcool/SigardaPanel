# MVP Scope

This document defines the initial release scope to keep development focused, secure, and incrementally deliverable.

## MVP Goals

- Self-hosted VPS management panel built with Go
- Manage multiple servers through agents
- Create and manage basic sites
- Deploy static, Node.js, and Go applications
- Automatic SSL certificate management
- Usable via dashboard and CLI
- All server operations logged in audit trail

## Non-Goals

- Email/mailbox features
- Billing system
- Plugin system
- Support for all Linux distributions
- Full cPanel clone in first release
- Authoritative DNS server
- Full Docker management in MVP

## Target Operating Systems

| OS | Priority |
|----|----------|
| Ubuntu 22.04 LTS | Primary |
| Ubuntu 24.04 LTS | Primary |
| Debian 12 | Secondary |

## MVP Components

| Component | Technology |
|-----------|------------|
| Backend API | Go (Echo) |
| Agent | Go |
| CLI | Go |
| Dashboard | SvelteKit + Tailwind |
| Database | SQLite (WAL mode) |
| Job Queue | Async processing |
| Reverse Proxy | Nginx |

## MVP Features

### Authentication & Users

- Login/logout
- Secure password hashing
- Basic roles: super_admin, admin, user
- Session management
- Scoped API tokens

### Server Management

- Register servers
- Install agents
- Check agent health
- List server capabilities
- View basic CPU, RAM, disk metrics
- View basic service status

### Site Management

- Create site with domain
- List/detail site
- Delete site with confirmation guard
- Enable/disable site
- Configure reverse proxy
- View access/error logs

### Runtime Support (MVP)

- Static sites
- Node.js applications via systemd
- Go binaries via systemd

### SSL

- Issue Let's Encrypt SSL certificates
- Automatic SSL renewal
- Check SSL expiry
- Force HTTPS toggle

### Deployment

- Simple artifact upload/deploy
- Basic Git deploy: repository URL, branch, deploy key/token via secret
- Optional build commands with allowlist/approval
- Service restart after deployment

### Job Queue

- Async operations for site creation, deployment, SSL, and deletion
- Job statuses: queued, running, succeeded, failed, canceled
- Job logs
- Limited retry support

### Audit Log

- Record actor, action, target, server, status, timestamp
- Mask secrets in payloads
- Basic search and filtering

### CLI (MVP)

```bash
sigardapanel init
sigardapanel login
sigardapanel server add|list|doctor
sigardapanel agent install|status|logs
sigardapanel site create|list|delete|deploy
sigardapanel logs tail
sigardapanel job list|watch
```

## Data Model (MVP)

- `users`
- `roles`
- `sessions`
- `api_tokens`
- `servers`
- `agents`
- `sites`
- `site_domains`
- `deployments`
- `jobs`
- `audit_logs`
- `secrets`

## Definition of Done

- [ ] Install panel on a single VPS
- [ ] Add the same or another server via agent
- [ ] Create a static site with a domain
- [ ] Issue a valid SSL certificate
- [ ] Deploy a Node.js application
- [ ] Deploy a Go application
- [ ] View site/service logs
- [ ] Async operations visible in job logs
- [ ] Audit log recorded for critical operations
- [ ] CLI can execute basic workflows without dashboard

## Primary Risks

- Command injection from domain, path, or build commands
- Agent compromise due to root privileges
- Data loss during delete/restore operations
- Reverse proxy configuration errors causing downtime
- Secret leakage through logs, UI, or CLI

## Mitigation Strategy

- Strict validation of domains, paths, and service names
- Command wrappers with argument arrays, not shell strings
- Configuration testing before proxy reload
- Dry-run and confirmation for delete operations
- Secret masking in all outputs
- Mandatory audit logging
