# Architecture

SigardaPanel is a self-hosted VPS management panel built with Go. It consists of a single binary with multiple operational modes: API server, agent, CLI, and development runner.

## Overview

```
┌─────────────────────────────────────────────────────────────┐
│                      Panel Server                           │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │  API Server  │  │  Dashboard   │  │  Job Worker  │      │
│  │   (Go/Echo)  │  │ (SvelteKit)  │  │  (Async)     │      │
│  └──────┬───────┘  └──────────────┘  └──────┬───────┘      │
│         │                                    │              │
│         └──────────────┬─────────────────────┘              │
│                        │                                    │
│              ┌─────────▼─────────┐                          │
│              │    SQLite DB      │                          │
│              │   (WAL mode)      │                          │
│              └───────────────────┘                          │
└──────────────────────────┬──────────────────────────────────┘
                           │
              ┌────────────▼────────────┐
              │    Agent (per VPS)      │
              │  ┌──────────────────┐   │
              │  │  Task Executor   │   │
              │  │  Nginx Manager   │   │
              │  │  SSL Handler     │   │
              │  │  Systemd Control │   │
              │  └──────────────────┘   │
              └─────────────────────────┘
```

## Components

### Panel API

- **Language:** Go (Echo framework)
- **Responsibilities:**
  - Authentication and RBAC
  - Server and site management
  - Job orchestration and queue management
  - Audit logging
  - Agent communication and health checks
  - REST API for dashboard and CLI

### Web Dashboard

- **Technology:** SvelteKit + Tailwind CSS
- **Responsibilities:**
  - Administrative interface
  - Job progress and log viewer
  - Site and server management
  - Secure credential input with masking

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

### Agent

- **Language:** Go
- **Responsibilities:**
  - Execute server operations
  - Manage reverse proxy configuration
  - Manage systemd services
  - Issue and renew SSL certificates
  - Stream logs
  - Report health and capabilities

### Database

- **Engine:** SQLite with WAL mode
- **Data:**
  - Users, roles, sessions
  - Servers, agents, sites
  - Jobs, deployments
  - Audit logs
  - Encrypted secrets

### Job Queue

- **Implementation:** Async processing within the backend process
- **Behavior:**
  - Worker polls jobs table and dispatches to agents
  - Real-time progress updates
  - Automatic retry with idempotency

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

### Agent Installation

1. User runs agent installer on target VPS
2. Installer detects OS, architecture, and dependencies
3. Installer creates agent user and systemd service
4. Agent registers with panel using bootstrap token
5. Panel stores agent fingerprint and capabilities
6. Agent begins heartbeat reporting

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

## Observability

- Request ID in all API responses
- Job ID in all async operations
- Audit log for critical operations
- Agent logs separated from application logs
- Basic metrics per server and site
