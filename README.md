<div align="center">

# SigardaPanel

**Open-source VPS management panel built with Go**

Lightweight, fast, and secure — manage your servers, sites, SSL, and deployments from a single binary.

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/bayurstarcool/SigardaPanel)](https://github.com/bayurstarcool/SigardaPanel/releases)

</div>

---

## Features

- **Multi-Server Management** — Manage multiple VPS instances from one panel
- **Site Management** — Create, configure, and deploy websites with ease
- **SSL Automation** — Automatic Let's Encrypt certificate issuance and renewal
- **Nginx Reverse Proxy** — Dynamic vhost configuration with managed blocks
- **Job Queue** — Async task processing with real-time status tracking
- **User & RBAC** — Role-based access control (super_admin, admin, user)
- **Audit Logging** — Complete audit trail for all administrative actions
- **Health Monitoring** — Automatic agent health checks with alerting
- **Single Binary** — No dependencies, no runtime, just one executable

## Architecture

```
┌─────────────────────────────────────────────────────┐
│                  Panel Server                       │
│  ┌──────────────┐  ┌──────────────┐                │
│  │  API Server  │  │  Dashboard   │                │
│  │   (Go/Echo)  │  │ (SvelteKit)  │                │
│  └──────┬───────┘  └──────────────┘                │
│         │                                          │
│  ┌──────▼───────┐                                  │
│  │  SQLite DB   │                                  │
│  └──────────────┘                                  │
└────────────────────────┬────────────────────────────┘
                         │
        ┌────────────────▼────────────────┐
        │     Agent (per VPS)             │
        │  ┌──────────────────────────┐   │
        │  │  Task Executor           │   │
        │  │  Nginx / SSL / Systemd   │   │
        │  └──────────────────────────┘   │
        └─────────────────────────────────┘
```

| Component | Description |
|-----------|-------------|
| **Panel API** | REST API for all operations |
| **Dashboard** | Web UI for management (Enterprise) |
| **Agent** | Executes tasks on target VPS |
| **Database** | SQLite with WAL mode |

## Installation

### Panel Server

```bash
# Download
curl -sSL https://github.com/bayurstarcool/SigardaPanel/releases/latest/download/sigardapanel-linux-amd64 -o sigardapanel
chmod +x sigardapanel
sudo mv sigardapanel /usr/local/bin/

# Setup
sigardapanel init
```

Or use the installer:

```bash
PANEL_DOMAIN=panel.yourdomain.com bash <(curl -sSL https://raw.githubusercontent.com/bayurstarcool/SigardaPanel/main/deploy/install-panel.sh)
```

### Agent (Target VPS)

```bash
# From panel, get registration token
sigardapanel server add

# On target VPS
curl -sSL http://panel.yourdomain.com:7700/api/v1/agents/install?token=YOUR_TOKEN | bash
```

## Usage

### Run Agent

```bash
# Start agent
sigardapanel agent

# Custom address
SIGARDAPANEL_AGENT_ADDR=:19090 sigardapanel agent

# Systemd
sudo systemctl start sigardapanel-agent
sudo systemctl enable sigardapanel-agent
```

### Run Panel

```bash
# Start API server
sigardapanel api

# Custom address
SIGARDAPANEL_API_ADDR=:18080 sigardapanel api
```

### CLI Commands

```bash
# Server
sigardapanel server list
sigardapanel server add

# Sites
sigardapanel site list
sigardapanel site create --domain example.com --server 1

# SSL
sigardapanel ssl issue --domain example.com
sigardapanel ssl renew --domain example.com

# Jobs
sigardapanel job list
sigardapanel job watch JOB_ID

# Backups
sigardapanel backup create --site example.com
sigardapanel backup list

# System
sigardapanel doctor
sigardapanel version
```

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `SIGARDAPANEL_API_ADDR` | `:7700` | API server address |
| `SIGARDAPANEL_AGENT_ADDR` | `:7790` | Agent address |
| `SIGARDAPANEL_AGENT_TOKEN` | - | Agent auth token |
| `SIGARDAPANEL_DB_PATH` | `./sigardapanel.db` | Database path |

## Security

- **Token-based Auth** — Bearer tokens for agent communication
- **Role-Based Access** — super_admin > admin > user
- **Audit Logging** — All actions logged
- **Path Validation** — Prevents traversal attacks
- **Config Validation** — Nginx validated before reload

## Enterprise

Full-featured panel with dashboard, API, and advanced management:

**[SigardaPanel-Enterprise](https://github.com/bayurstarcool/SigardaPanel-Enterprise)**

- Web Dashboard (SvelteKit + Tailwind)
- Panel API Server
- Job Worker & Scheduler
- Backup Management
- Cloudflare DNS Integration
- Notifications (Telegram, Discord, Email)
- License & Plan Management

## License

MIT License — see [LICENSE](LICENSE)

---

<div align="center">

**Built with ❤️ using Go**

</div>
