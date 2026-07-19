# Agent

This document defines the design and behavior of the SigardaPanel VPS agent.

## Purpose

The agent is a Go service that runs on target servers to execute operations safely, reliably, and with full audit capability.

## Principles

- The agent is not a policy authority
- The backend makes permission decisions
- The agent validates task schema, paths, and capabilities
- The agent executes operations with timeouts
- The agent never executes raw user input as shell commands

## Communication

### Current Implementation

- **Signed token requests** — agent authenticates with panel using registration token
- Panel communicates with agent via HTTP on port `:7710`

### Future

- **HTTPS/mTLS** between backend and agent (planned)
- **SSH mode** — only for bootstrap or fallback when requested

## Agent Registration

### Flow

1. Admin creates registration token via API (`POST /api/v1/agents/tokens`)
2. User runs installer on target VPS with token
3. Installer sends token to panel API (`POST /api/v1/agents/register`)
4. Panel validates token, registers agent
5. Agent begins heartbeat reporting
6. Panel marks server online after valid heartbeat

## Agent Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | `GET` | Health check |
| `/capabilities` | `GET` | Capability report |
| `/tasks/site/create` | `POST` | Create site |
| `/tasks/site/delete` | `POST` | Delete site |
| `/tasks/site/deploy` | `POST` | Deploy site |
| `/tasks/ssl/issue` | `POST` | Issue SSL certificate |
| `/tasks/service/reload` | `POST` | Reload service |
| `/tasks/fail2ban/*` | `POST` | Fail2ban management |
| `/tasks/firewall/*` | `POST` | Firewall management |
| `/tasks/docker/*` | `POST/GET` | Docker management |
| `/tasks/pm2/*` | `POST` | PM2 management |
| `/tasks/redis/*` | `GET/POST` | Redis management |
| `/tasks/bot-blocker/*` | `POST` | Bot blocker management |
| `/logs` | `GET` | Retrieve logs |
| `/metrics` | `POST` | Receive metrics from agent |
| `/ws/terminal` | `WS` | WebSocket PTY terminal |

All mutation endpoints require authenticated requests from the backend.

## Task Envelope

All tasks must include:

```json
{
  "task_id": "task_123",
  "job_id": "job_123",
  "requested_by": "user_123",
  "action": "site.create",
  "payload": {},
  "timeout_seconds": 300,
  "issued_at": "2026-06-24T00:00:00Z"
}
```

The agent must reject tasks without `task_id`, `job_id`, `action`, or valid authentication.

## Site Creation

### Required Input

- `site_id`
- `domain`
- `runtime`
- Document root / release root (generated)
- SSL enabled/disabled

### Steps

1. Validate domain and site_id
2. Create site directory under `/home/{system_user}/htdocs/{domain}`
3. Create user/group per site if isolation enabled
4. Generate runtime configuration
5. Generate reverse proxy configuration
6. Test proxy configuration
7. Reload proxy
8. Issue SSL if requested
9. Return status and safe logs

## Deployment

### Strategy

- Release directory per deploy
- Symlink `current` to active release
- Rollback by switching symlink to previous release
- Restart service after deploy if required

### Deploy Sources

- Git repository
- Artifact upload

### Build Commands

- Disabled by default or require allowlist/approval
- If enabled, run as site user, not root
- Timeout is mandatory

## Reverse Proxy

### Nginx

- Generate configuration in controlled path
- Files must include SigardaPanel markers
- Run `nginx -t` before reload
- Reload only if test succeeds

## Systemd Service

For Node.js, Go, and Python applications:

| Field | Value |
|-------|-------|
| Unit name | `sigardapanel-site-<site_id>.service` |
| Run user | Site user (if isolation enabled) |
| WorkingDirectory | Site current release |
| EnvironmentFile | Site environment file path |
| Restart policy | `on-failure` |

## WebSocket PTY Terminal

- Full TUI support (htop, vim, nano, tmux)
- Session persistence across reconnects
- 64KB pending buffer when no client attached
- Per-site terminal runs as `system_user` (not root)
- Sticky keyboard shortcuts (Ctrl+C/Z/D/L/U, Tab, Esc, arrows)
- Mobile-optimized input handling

## Docker Management

- Container lifecycle (list, create, start, stop, restart, remove)
- Container logs and exec
- Image management (list, search, pull, remove)
- Docker Compose (up/down)
- Volume management (list, create, remove)
- Network management (list, create, remove)
- Docker info and disk usage

## Firewall (UFW)

- Status check
- Enable/disable
- Allow/deny/delete rules
- List rules
- Reset

## Fail2ban

- Status and jail list
- Ban/unban IPs
- Enable/disable jails

## Redis

- Stats and info
- Flush all/DB keys

## PM2

- Process management (start, stop, restart, delete, list)

## Bot Blocker

- Manage bot blocking rules

## Logs

The agent must support reading:

- Access logs
- Error logs
- Application/service logs
- Agent operation logs

### Log Output Options

- Tail lines
- Follow stream
- Since timestamp
- Maximum bytes limit

## Metrics

### Implemented Metrics

- CPU usage
- RAM usage (used/total)
- Disk usage (used/total)
- Swap usage (used/total)
- Network I/O (bytes in/out)
- GPU metrics (when available)

Metrics must be lightweight. Agent pushes to panel via `POST /api/v1/servers/:id/metrics`.

## Safety Guards

The agent must reject:

- Paths outside allowed roots
- Invalid domains
- Invalid service names
- Expired tasks
- Unknown task actions
- Requests without valid authentication
- Delete operations on resources without SigardaPanel markers

## File Markers

Generated files must include header comments:

```
# Managed by SigardaPanel
# Do not edit manually unless you know what you are doing
# site_id=<site_id>
```

The agent must not overwrite files without markers unless the task explicitly includes import/adopt mode.

## Error Model

Agent error responses:

```json
{
  "error": {
    "code": "proxy_config_invalid",
    "message": "Reverse proxy configuration validation failed",
    "details": "safe short detail",
    "task_id": "task_123"
  }
}
```

Error details must never contain secrets.

## Agent Installer

The installer must:

- Detect OS and architecture
- Install agent binary
- Create agent configuration
- Register agent with panel
- Create systemd service
- Start and enable service
- Display final status

The installer must be idempotent and safe to run multiple times.
