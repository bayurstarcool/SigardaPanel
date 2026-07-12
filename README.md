<div align="center">

# SigardaPanel

**Open-source VPS management panel built with Go**

Lightweight, fast, and secure — manage your servers, sites, SSL, and deployments from a single binary.

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/bayurstarcool/SigardaPanel)](https://github.com/bayurstarcool/SigardaPanel/releases)

</div>

---

## Features

### Core
- **Multi-Server Management** — Manage multiple VPS instances from one panel
- **Site Management** — Create, configure, and deploy websites with ease
- **SSL Automation** — Automatic Let's Encrypt certificate issuance and renewal
- **Database Management** — MySQL/PostgreSQL create, manage, users
- **Backups** — Local + S3 storage, scheduled backups, restore

### DevOps
- **Docker Management** — Containers, images, volumes, networks, compose
- **Git Deploy** — Branches, commit history, rollback, checkout
- **Firewall (UFW)** — Rules, presets, IP blocking, enable/disable
- **Redis/Memcached** — Stats, flush, info, management
- **Stack Manager** — Install/manage nginx, php, node, docker, etc.

### Monitoring and Security
- **System Monitoring** — CPU, memory, disk, network metrics
- **SSL Dashboard** — Visual certificate status and expiry timeline
- **Disk Usage Alerts** — 80% warning, 90% critical notifications
- **Audit Logs** — Track all administrative actions
- **Global Search** — Cmd+K search across servers, sites, databases

## Quick Start

### Install

```bash
curl -sSL https://github.com/bayurstarcool/SigardaPanel/releases/latest/download/sigardapanel -o sigardapanel
chmod +x sigardapanel
./sigardapanel install
```

### Development

```bash
git clone https://github.com/bayurstarcool/SigardaPanel.git
cd SigardaPanel
go run ./cmd/sigardapanel dev
```

### Production

```bash
CGO_ENABLED=0 go build -o sigardapanel ./cmd/sigardapanel
./sigardapanel dev
```

## CLI Commands

```
sigardapanel
  api / agent / dev / install / init / login / logout / doctor / version
  server    add|list|update|remove|doctor
  site      create|list|update|delete|deploy|config|setup-logrotate
  ssl       status|issue|renew|renew-all
  job       list|watch|cancel|logs
  backup    create|list|delete|restore
  db        create|list|update|delete
  db user   create|list|rotate-password|delete
  docker    ps|start|stop|restart|rm|logs|images|compose
  git       branches|log|rollback|checkout
  firewall  status|enable|disable|allow|deny|rules|delete|reset
  redis     stats|info|flush|flushdb
  system    disk-usage|updates
  channels  add|list|remove
  alerts    list
  metrics   latest|list
  user      create|list|reset-password|delete
```

## Architecture

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

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| SIGARDAPANEL_API_ADDR | :8080 | API listen address |
| SIGARDAPANEL_AGENT_ADDR | :9090 | Agent listen address |
| SIGARDAPANEL_DB_PATH | sigardapanel.db | Database path |
| SIGARDAPANEL_AGENT_TOKEN | - | Agent auth token |

## Tech Stack

- **Backend**: Go 1.22+, Echo framework
- **Frontend**: Svelte 5, SvelteKit, Tailwind CSS
- **Database**: SQLite (WAL mode)
- **Auth**: Argon2id password hashing

## License

MIT License — see [LICENSE](LICENSE) for details.

## Links

- [GitHub](https://github.com/bayurstarcool/SigardaPanel)
- [Releases](https://github.com/bayurstarcool/SigardaPanel/releases)
- [Enterprise Edition](https://github.com/bayurstarcool/SigardaPanel-Enterprise)
