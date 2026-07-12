# Architecture

SigardaPanel v0.5.0 — Self-hosted VPS management panel built with Go.

## Overview

```
┌──────────────────────────────────────────────────┐
│               Panel Server                       │
│  ┌──────────────┐    ┌──────────────────┐        │
│  │  API (:7700) │    │  Frontend (:7720)│        │
│  │  (Go/Echo)   │    │  (SvelteKit)     │        │
│  └──────┬───────┘    └────────┬─────────┘        │
│         │                     │                   │
│  ┌──────▼─────────────────────▼─────────┐         │
│  │         SQLite DB (WAL mode)         │         │
│  └──────────────────────────────────────┘         │
└──────────────────────┬───────────────────────────┘
                       │
        ┌──────────────▼──────────────┐
        │   Agent (:7710) per VPS     │
        │  • Site management          │
        │  • SSL certificates         │
        │  • Docker management        │
        │  • Firewall (UFW)           │
        │  • Redis/Memcached          │
        │  • System metrics           │
        └─────────────────────────────┘
```

## Components

### API Server (Go/Echo)
- REST API with ~150+ endpoints
- JWT-like token authentication
- Role-based access control (super_admin, admin, user)
- Audit logging for all admin actions
- Background job scheduler
- Metrics collector

### Frontend (SvelteKit)
- Svelte 5 with runes
- Tailwind CSS
- Dark/Light theme
- Global Search (Cmd+K)
- Mobile responsive

### Agent (Go)
- Runs on each managed VPS
- Executes tasks from API server
- Manages nginx, SSL, Docker, UFW, Redis
- Reports system metrics

### Database (SQLite)
- WAL mode for concurrent access
- 42 incremental migrations
- Auto-upgrade on startup

## Data Flow

1. User → Frontend → API (auth check)
2. API → Store (SQLite) → Response
3. API → Agent (proxy) → Task execution
4. Agent → System (nginx, docker, ufw, etc.)

## Security

- Argon2id password hashing
- Token-based authentication
- Role-based access control
- Audit logging
- Executor allowlist for shell commands
- Agent token authentication

## Tech Stack

| Component | Technology |
|-----------|------------|
| Backend | Go 1.22+, Echo framework |
| Frontend | Svelte 5, SvelteKit, Tailwind CSS |
| Database | SQLite (WAL mode) |
| Auth | Argon2id, token-based |
| Agent | Go, system commands |
