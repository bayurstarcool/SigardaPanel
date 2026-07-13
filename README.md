<div align="center">

# SigardaPanel

**Open-source VPS management panel built with Go**

Lightweight, fast, and secure — manage your servers, sites, SSL, Docker, firewall, and deployments from a single binary.

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/bayurstarcool/SigardaPanel)](https://github.com/bayurstarcool/SigardaPanel/releases)
[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)](https://golang.org)
[![Svelte](https://img.shields.io/badge/Svelte-5-FF3E00?logo=svelte&logoColor=white)](https://svelte.dev)

</div>

---

## Features

### 🖥️ Server Management
- **Multi-Server** — Manage multiple VPS from one panel
- **Health Monitoring** — CPU, memory, disk, network metrics
- **Server Status** — Real-time health indicators
- **Agent Management** — Auto-registration, token management

### 🌐 Site Management
- **Site CRUD** — Create, update, delete websites
- **Git Deploy** — Branches, commit history, rollback, checkout
- **Docker Deploy** — Container-based deployments
- **Runtime Support** — Static, PHP, Node.js, Python, Go
- **Log Rotation** — Auto-generated on site creation

### 🔒 SSL & Security
- **SSL Automation** — Let's Encrypt auto-issue and renewal
- **SSL Dashboard** — Visual certificate status and expiry timeline
- **Auto-Renewal** — Cron-based renewal with expiry alerts
- **Firewall (UFW)** — Rules, presets, IP blocking, enable/disable
- **Fail2ban** — Ban/unban IPs, manage jails, enable/disable

### 🐳 Docker Management
- **Containers** — Start, stop, restart, remove, view logs
- **Images** — Pull, remove, list with sizes
- **Volumes** — Create, remove, list
- **Networks** — Create, remove, list
- **Compose** — Docker Compose up/down
- **System Info** — Version, driver, CPUs, memory, disk usage

### 💾 Databases
- **MySQL/MariaDB** — Full CRUD management
- **PostgreSQL** — Full CRUD management
- **Database Users** — Create, list, rotate password, delete
- **phpMyAdmin** — Auto-login integration

### 💿 Backups
- **Local Backups** — On-server backup storage
- **S3 Backups** — AWS S3, DigitalOcean Spaces, Wasabi, B2
- **Scheduled Backups** — Cron-based automation
- **Backup Restore** — One-click restore from any backup
- **Concurrency Limit** — Max 5 concurrent backups per server
- **Stuck Cleanup** — Auto-fail jobs stuck >30min

### 🖥️ WebSocket Terminal
- **Full TUI Support** — htop, vim, nano, tmux
- **Session Persistence** — Reconnect to same session
- **Sticky Shortcuts** — Ctrl+C/Z/D/L/U, Tab, Esc, arrows
- **Mobile Optimized** — No resize interference during typing
- **Pending Buffer** — 64KB buffer when no client attached

### 📊 Monitoring & Alerts
- **System Metrics** — CPU, memory, disk, network
- **Disk Usage Alerts** — 80% warning, 90% critical
- **Alert Channels** — Telegram, Discord, Email, Webhook
- **In-App Notifications** — Real-time notification center

### 🌍 Cloudflare Integration
- **DNS Management** — Create, update, delete DNS records
- **Zone Management** — List and search zones
- **Cache Purge** — Purge Cloudflare cache
- **Analytics** — Zone traffic analytics

### 🛡️ Additional Features
- **Redis Management** — Stats, flush, info
- **Memcached** — Install and management
- **Varnish Cache** — Config and purge
- **Cron Jobs** — Per-site cron management
- **File Manager** — Browse, edit, upload, download
- **Audit Logs** — Complete action audit trail
- **User Management** — RBAC with roles (super_admin, admin, user)
- **Impersonation** — Admin can impersonate users
- **Service Tokens** — API access tokens with scopes
- **Global Search** — Cmd+K search across all resources
- **Dark/Light Theme** — Toggle between themes
- **Mobile Responsive** — Works on all devices

---

## CLI Commands (55+)

```
sigardapanel
├── api / agent / dev / install / init / login / logout / doctor / version
│
├── server    add|list|update|remove|doctor
├── site      create|list|update|delete|deploy|config|setup-logrotate
├── ssl       status|issue|renew|renew-all
├── job       list|watch|cancel|logs
├── backup    create|list|delete|restore
├── db        create|list|update|delete
│   └── user  create|list|rotate-password|delete
│
├── docker    ps|start|stop|restart|rm|logs|images|compose
├── git       branches|log|rollback|checkout
├── firewall  status|enable|disable|allow|deny|rules|delete|reset
├── redis     stats|info|flush|flushdb
├── system    disk-usage|updates
│
├── channels  add|list|remove
├── alerts    list
├── metrics   latest|list
├── user      create|list|reset-password|delete
└── cloudpanel scan|import-sites|import-ssl|doctor
```

---

## API Endpoints (~160+)

| Domain | Endpoints | Description |
|--------|-----------|-------------|
| Auth | 5 | Login, logout, profile |
| Users | 12 | CRUD, roles, impersonation |
| Servers | 8 | CRUD, health, tokens |
| Sites | 15 | CRUD, deploy, config |
| App Runtime | 7 | Start, stop, restart, limits |
| Databases | 14 | MySQL/PostgreSQL CRUD |
| Backups | 16 | Local/S3, scheduled, restore |
| Backup Providers | 6 | S3, B2, Wasabi config |
| SSL | 4 | Issue, renew, auto-renew |
| Jobs | 3 | List, cancel, logs |
| Cloudflare | 16 | DNS, zones, analytics |
| Stack | 4 | Install/manage software |
| Docker | 19 | Containers, images, volumes, networks |
| Git | 4 | Branches, history, rollback |
| Firewall | 8 | UFW rules, presets |
| Fail2ban | 6 | Ban/unban, jail management |
| Redis | 4 | Stats, flush, info |
| WebSocket | 1 | Terminal (PTY) |
| License/Plans | 9 | License management |
| Notifications | 6 | Real-time notifications |
| Alerts | 4 | Alert channels and history |
| Service Tokens | 3 | API token management |

---

## Architecture

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
        │  • Fail2ban management      │
        │  • WebSocket PTY terminal   │
        │  • Redis/Memcached          │
        │  • System metrics           │
        └─────────────────────────────┘
```

---

## Quick Start

### Install

```bash
curl -sSL https://github.com/bayurstarcool/SigardaPanel/releases/latest/download/sigardapanel-linux-amd64 -o sigardapanel
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
# Build
CGO_ENABLED=0 go build -o sigardapanel ./cmd/sigardapanel

# Run API + Agent
./sigardapanel dev

# Or run separately
./sigardapanel api &
./sigardapanel agent &
```

---

## Downloads

| Platform | Architecture | Binary |
|----------|-------------|--------|
| Linux | amd64 | `sigardapanel-linux-amd64` |
| Linux | arm64 | `sigardapanel-linux-arm64` |
| macOS | amd64 | `sigardapanel-darwin-amd64` |
| macOS | arm64 | `sigardapanel-darwin-arm64` |
| Windows | amd64 | `sigardapanel-windows-amd64.exe` |

All binaries: https://github.com/bayurstarcool/SigardaPanel/releases

---

## Tech Stack

| Component | Technology |
|-----------|------------|
| Backend | Go 1.22+, Echo framework |
| Frontend | Svelte 5, SvelteKit, Tailwind CSS |
| Database | SQLite (WAL mode) |
| Auth | Argon2id password hashing |
| Agent | Go, system commands |
| Terminal | WebSocket PTY (gorilla/websocket, creack/pty, xterm.js) |

---

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| SIGARDAPANEL_API_ADDR | :7700 | API listen address |
| SIGARDAPANEL_AGENT_ADDR | :7710 | Agent listen address |
| SIGARDAPANEL_DB_PATH | sigardapanel.db | Database path |
| SIGARDAPANEL_AGENT_TOKEN | - | Agent auth token |

---

## Roadmap

### v0.6.0 — Process Management & Uptime
- [ ] Process resource limits (CPU, memory per app)
- [ ] Uptime monitoring (HTTP/TCP checks)
- [ ] Status page (public)
- [ ] Incident management

### v0.7.0 — Email & DNS
- [ ] Email hosting (Postfix/Dovecot)
- [ ] Webmail integration (Roundcube)
- [ ] Local DNS zone management
- [ ] Email forwarders and autoresponders

### v0.8.0 — Advanced Features
- [ ] Multi-server config sync
- [ ] Staging environments
- [ ] Deployment previews
- [ ] Team management and workspaces
- [ ] API documentation page (Swagger/OpenAPI)

### v1.0.0 — Production Ready
- [ ] High availability setup
- [ ] Database migration to PostgreSQL (optional)
- [ ] WebSocket real-time updates
- [ ] Mobile app (Flutter)
- [ ] Plugin system

---

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

---

## License

MIT License — see [LICENSE](LICENSE) for details.

---

## Links

- [GitHub](https://github.com/bayurstarcool/SigardaPanel)
- [Releases](https://github.com/bayurstarcool/SigardaPanel/releases)
- [Enterprise Edition](https://github.com/bayurstarcool/SigardaPanel-Enterprise)
- [Documentation](https://panel.sigarda.dev/docs)

---

<div align="center">

**Built with ❤️ by [bayurstarcool](https://github.com/bayurstarcool)**

</div>
