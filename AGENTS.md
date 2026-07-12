# SigardaPanel — Agent & Developer Guide

## Architecture

SigardaPanel is a VPS management panel with client-server architecture.

```
Panel Server
  API (:8080) + Frontend (:4001)
  SQLite DB (WAL mode)
      |
Agent (:9090) per VPS
  - Site management
  - SSL certificates
  - Docker management
  - Firewall (UFW)
  - System metrics
```

## Services

| Service  | Port  | Description                          |
|----------|-------|--------------------------------------|
| API      | 8080  | Panel API server                     |
| Frontend | 4001  | SvelteKit web dashboard              |
| Agent    | 9090  | Agent HTTP server (per VPS)          |

## Quick Start (Dev Mode)

```bash
cd SigardaPanel-Enterprise
./sigardapanel dev
```

Frontend: `cd web && npx vite dev --host 0.0.0.0 --port 4000`

## Endpoints

### Panel API (port 8080)

| Method | Path                        | Description              |
|--------|-----------------------------|--------------------------|
| POST   | /api/v1/auth/login          | Login                    |
| GET    | /api/v1/auth/me             | Current user             |
| GET    | /api/v1/servers             | List servers             |
| POST   | /api/v1/servers             | Create server            |
| GET    | /api/v1/servers/:id         | Server detail + token    |
| PATCH  | /api/v1/servers/:id         | Update server            |
| DELETE | /api/v1/servers/:id         | Delete server            |
| GET    | /api/v1/sites               | List sites               |
| POST   | /api/v1/sites               | Create site              |
| GET    | /api/v1/databases           | List databases           |
| POST   | /api/v1/sites/:id/databases | Create database          |
| GET    | /api/v1/jobs                | List jobs                |
| GET    | /api/v1/users               | List users               |
| POST   | /api/v1/users               | Create user              |

### Agent (port 9090)

| Method | Path                        | Description              |
|--------|-----------------------------|--------------------------|
| GET    | /health                     | Health check             |
| GET    | /metrics                    | System metrics           |
| GET    | /capabilities               | Agent capabilities       |

## Agent Installation

```bash
curl -sSL https://github.com/bayurstarcool/SigardaPanel/releases/latest/download/sigardapanel -o /usr/local/bin/sigardapanel
chmod +x /usr/local/bin/sigardapanel

SIGARDAPANEL_AGENT_TOKEN=<token> sigardapanel agent
```

## Env Variables

| Variable | Default | Description |
|----------|---------|-------------|
| SIGARDAPANEL_API_ADDR | :8080 | API listen address |
| SIGARDAPANEL_AGENT_ADDR | :9090 | Agent listen address |
| SIGARDAPANEL_DB_PATH | sigardapanel.db | Database path |
| SIGARDAPANEL_AGENT_TOKEN | - | Agent auth token |

## Repositories

- **Public (binary distribution)**: github.com/bayurstarcool/SigardaPanel
- **Enterprise (full source)**: github.com/bayurstarcool/SigardaPanel-Enterprise
