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

### Priority Order

1. **HTTPS/mTLS** — between backend and agent (preferred)
2. **Signed token requests** — with rotation plan
3. **SSH mode** — only for bootstrap or fallback when requested

## Agent Registration

### Flow

1. Backend creates bootstrap token
2. User runs installer on target VPS
3. Installer sends token to agent setup
4. Agent registers with backend
5. Backend stores agent fingerprint
6. Agent sends capability report
7. Backend marks server online after valid heartbeat

## Capability Report

The agent must report:

| Capability | Description |
|------------|-------------|
| OS name/version | Operating system information |
| Architecture | CPU architecture |
| Agent version | Current agent version |
| Hostname | Server hostname |
| CPU/RAM/disk | Resource summary |
| Reverse proxy | Available: nginx, caddy |
| Runtimes | Available: node, go, php, python, docker |
| Systemd | Systemd availability |
| ACME/SSL | SSL certificate capability |
| Allowed roots | Permitted directory roots |

### Example Response

```json
{
  "agent_version": "0.1.0",
  "os": "ubuntu",
  "os_version": "24.04",
  "arch": "amd64",
  "capabilities": {
    "systemd": true,
    "nginx": true,
    "caddy": false,
    "node": true,
    "go": true,
    "php": false,
    "python": true,
    "docker": false
  }
}
```

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
| `/logs` | `GET` | Retrieve logs |
| `/metrics` | `GET` | Retrieve metrics |

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
2. Create site directory in allowed root
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

### Caddy

- Generate Caddyfile or JSON configuration
- Validate configuration before reload if available

## Systemd Service

For Node.js, Go, and Python applications:

| Field | Value |
|-------|-------|
| Unit name | `sigardapanel-site-<site_id>.service` |
| Run user | Site user (if isolation enabled) |
| WorkingDirectory | Site current release |
| EnvironmentFile | Site environment file path |
| Restart policy | `on-failure` |

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

### MVP Metrics

- CPU usage
- RAM usage
- Disk usage
- Load average
- Service status
- Site process status

Metrics must be lightweight. Avoid excessive polling frequency.

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
