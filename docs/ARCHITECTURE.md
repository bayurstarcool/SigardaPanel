# Architecture

SigardaPanel is a self-hosted VPS management panel built with Go. It consists of a single binary with multiple operational modes: API server, agent, CLI, and development runner.

## Overview

```
┌─────────────────────────────────────────────────────────┐
│                    Panel Server (:7700)                   │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐   │
│  │  API Server  │  │  Dashboard   │  │  Job Worker   │   │
│  │  (Go/Echo)   │  │ (SvelteKit)  │  │  (Async)      │   │
│  └──────┬───────┘  └──────────────┘  └──────┬───────┘   │
│         │                                    │           │
│         └──────────────┬─────────────────────┘           │
│                        │                                 │
│              ┌─────────▼─────────┐                       │
│              │    SQLite DB      │                       │
│              │   (WAL mode)      │                       │
│              └───────────────────┘                       │
└──────────────────────────┬──────────────────────────────┘
                           │
              ┌────────────▼────────────┐
              │    Agent (:7710) per VPS │
              │  ┌──────────────────┐   │
              │  │  Task Executor   │   │
              │  │  Nginx Manager   │   │
              │  │  SSL Handler     │   │
              │  │  Systemd Control │   │
              │  │  Docker Manager  │   │
              │  │  File Manager    │   │
              │  │  Terminal (PTY)  │   │
              │  │  Firewall (UFW)  │   │
              │  │  Fail2ban        │   │
              │  │  Redis/Memcached │   │
              │  └──────────────────┘   │
              └─────────────────────────┘
```

## Components

### Panel API

- **Language:** Go (Echo framework)
- **Port:** `:7700`
- **Responsibilities:**
  - Authentication and RBAC (roles: super_admin, admin, user)
  - 2FA/TOTP with recovery codes
  - User impersonation
  - Server and site management
  - Job orchestration and queue management
  - Audit logging
  - Agent communication and health checks
  - Metrics ingestion and history
  - Backup management with S3 support
  - Database management (MySQL/MariaDB)
  - Docker management
  - Firewall (UFW) management
  - Fail2ban management
  - Redis management
  - PM2 process management
  - Cloudflare DNS integration
  - License and feature gating
  - In-app notifications and alerts
  - REST API for dashboard and CLI

### Web Dashboard

- **Technology:** SvelteKit + Tailwind CSS
- **Port:** `:7720`
- **Responsibilities:**
  - Administrative interface
  - Job progress and log viewer
  - Site and server management
  - File manager
  - WebSocket terminal (PTY)
  - Docker management UI
  - Firewall/Fail2ban UI
  - Backup management UI
  - Database management UI
  - Monitoring and metrics dashboard
  - Cloudflare DNS management
  - Notifications center
  - License and billing management

### CLI / Unified Binary

- **Language:** Go
- **Binary:** `sigardapanel`
- **Modes:** `api`, `agent`, `dev`, and CLI commands
- **Responsibilities:**
  - Administrative automation
  - Installer helper
  - Site deployment
  - Log and job monitoring
  - System diagnostics (`doctor`)
  - All operations available via CLI

### Agent

- **Language:** Go
- **Port:** `:7710`
- **Responsibilities:**
  - Execute server operations
  - Manage reverse proxy configuration (Nginx)
  - Manage systemd services
  - Issue and renew SSL certificates (Let's Encrypt)
  - Docker container management
  - File manager operations
  - WebSocket PTY terminal (session persistence)
  - Firewall (UFW) management
  - Fail2ban management
  - Redis/Memcached management
  - PM2 process management
  - Bot blocker management
  - Stream logs and metrics
  - Report health and capabilities

### Database

- **Engine:** SQLite with WAL mode
- **Data:**
  - Users, sessions, service tokens
  - Servers, agents, registration tokens
  - Sites, user-site mappings
  - Jobs, job logs
  - Backups, backup configs, backup providers
  - Databases, database users, database servers
  - Server metrics
  - Alert channels, alerts, notifications
  - Audit logs
  - License, plans, orders
  - Cloudflare config
  - App settings

### Job Queue

- **Implementation:** Async processing within the backend process
- **Behavior:**
  - Worker polls jobs table and dispatches to agents
  - Real-time progress updates
  - Automatic retry with backoff (max 3 attempts)
  - Idempotent retries
  - Job logs captured per-line

---

## Workflows

### Create Site

1. User/CLI calls API to create site
2. Backend validates RBAC, domain, runtime, and server capability
3. Backend creates async job
4. Worker sends task to agent
5. Agent creates site directory and runtime configuration
6. Agent generates reverse proxy configuration
7. Agent tests proxy configuration
8. Agent reloads proxy
9. Agent issues SSL if requested
10. Backend updates job status and audit log

### Deploy Application

1. User triggers deploy from dashboard/CLI
2. Backend validates permissions and site ownership
3. Backend creates deployment record and job
4. Agent checks out/uploads artifact to release directory
5. Agent executes allowed build commands
6. Agent updates symlink to active release
7. Agent restarts/reloads service
8. Agent streams logs and progress to backend

### Agent Registration

1. Admin creates registration token via API
2. User runs installer on target VPS with token
3. Installer sends token to panel API
4. Panel validates token, registers agent
5. Agent begins heartbeat reporting
6. Panel marks server online after valid heartbeat

### Backup

1. User triggers backup (manual or scheduled)
2. Backend creates backup job
3. Agent creates tar archive of site files
4. Agent uploads to S3/storage provider (if configured)
5. Backend updates backup record with paths and size
6. Retention policy auto-expires old backups

---

## Trust Boundary

| Component | Trust Level |
|-----------|-------------|
| Browser/CLI | Not trusted without valid token |
| Panel API | Policy authority — validates all requests |
| Agent | Limited executor — scoped per server |
| Database | Source of truth — WAL mode for consistency |
| Server OS | Most sensitive boundary |

## Generated File Policy

- All generated files must include SigardaPanel markers
- Never overwrite manual files without markers
- Backup existing configuration before replacement
- Test configuration before service reload
- Use `BEGIN/END SIGARDAPANEL MANAGED BLOCK` markers

## Failure Handling

- Jobs must fail cleanly with safe error messages
- Partial resources must be cleaned up on best-effort basis
- Database state must not indicate success before agent confirms
- Retries must be idempotent
- All operations must have appropriate timeouts
- Backup jobs stuck >30min auto-marked as failed

## Observability

- Request ID in all API responses
- Job ID in all async operations
- Audit log for critical operations
- Agent logs separated from application logs
- Server metrics (CPU, RAM, disk, network, swap)
- GPU metrics
- In-app notifications per user
- Alert channels (Telegram, Discord, email, webhook)
