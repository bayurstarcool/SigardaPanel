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
- **S3 Backups** — AWS S3, DigitalOcean Spaces, Wasabi
- **Scheduled Backups** — Cron-based automation
- **Backup Restore** — One-click restore from any backup

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
- **Site DNS Provisioning** — Optional A-record creation during site creation
- **Per-User Credentials** — Each panel user can own an independent Cloudflare token

### 🛡️ Additional Features
- **Redis Management** — Stats, flush, info
- **Memcached** — Install and management
- **Varnish Cache** — Config and purge
- **Cron Jobs** — Per-site cron management
- **File Manager** — Browse, edit, upload, download
- **Web Terminal** — Browser-based SSH terminal
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

## API Endpoints (~150+)

| Domain | Endpoints | Description |
|--------|-----------|-------------|
| Auth | 5 | Login, logout, profile |
| Users | 12 | CRUD, roles, impersonation |
| Servers | 8 | CRUD, health, tokens |
| Sites | 15 | CRUD, deploy, config |
| App Runtime | 7 | Start, stop, restart, limits |
| Databases | 14 | MySQL/PostgreSQL CRUD |
| Backups | 16 | Local/S3, scheduled, restore |
| SSL | 4 | Issue, renew, auto-renew |
| Jobs | 3 | List, cancel, logs |
| Cloudflare | 17 | DNS, zones, analytics, cache purge |
| Stack | 4 | Install/manage software |
| Docker | 19 | Containers, images, volumes, networks |
| Git | 4 | Branches, history, rollback |
| Firewall | 8 | UFW rules, presets |
| Redis | 4 | Stats, flush, info |
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
        │  • Redis/Memcached          │
        │  • System metrics           │
        └─────────────────────────────┘
```

---

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
# Build
CGO_ENABLED=0 go build -o sigardapanel ./cmd/sigardapanel

# Run API + Agent
./sigardapanel dev

# Or run separately
./sigardapanel api &
./sigardapanel agent &
```

### Deploy a Site Through SigardaPanel

SigardaPanel is panel-first: site creation is handled by the API, queued as a
job, and executed by the Agent. Do not edit generated Nginx files manually
unless recovering from a documented failure.

```text
Sites → Create Site
  → API validates and stores site
  → Worker queues site.create
  → Agent creates root/configuration
  → Nginx validates and reloads
  → optional Cloudflare DNS provisioning
  → optional SSL issue
```

Runtime selection:

| Runtime | Use | Required configuration |
|---|---|---|
| `static` | HTML/CSS/JS | `root_path` |
| `php` | PHP/Laravel/WordPress | `root_path`, PHP configuration |
| `node` | Node.js app | app process on assigned port |
| `python` | Python app | app process on assigned port |
| `go` | Go app | app process on assigned port |
| `proxy` | Existing service | `proxy_pass_url`, e.g. `http://127.0.0.1:18080` |
| `docker` | Container app | Docker image and container settings |

External repository deployment pattern:

1. Clone and audit repository.
2. Install dependencies and run syntax/build checks.
3. Bind backend to a dedicated loopback port.
4. Use same-origin browser API paths; never ship `localhost` as production API origin.
5. Create the site in **Sites** with `runtime=proxy`.
6. Enable **Cloudflare DNS** only after the token is configured for the creating user.
7. Verify job result, Nginx, DNS, backend, and public URL in that order.

Example same-origin FastAPI site:

```json
{
  "server_id": 1,
  "name": "pdf-editor",
  "domain": "pdfedit.sigarda.com",
  "runtime": "proxy",
  "proxy_pass_url": "http://127.0.0.1:18080",
  "cloudflare_provision": true
}
```

### Cloudflare DNS Setup

Cloudflare credentials are scoped to the logged-in panel user. Configure the
token first under **Cloudflare**, then create sites from that same user when
automatic DNS provisioning is required.

Panel API flow:

```text
GET  /api/v1/cloudflare/config
GET  /api/v1/cloudflare/zones
GET  /api/v1/cloudflare/zones/:id/dns
POST /api/v1/cloudflare/zones/:id/dns
POST /api/v1/sites
```

`DNS_PROBE_FINISHED_NXDOMAIN` means the hostname has no visible DNS record.
Check authoritative DNS before debugging Nginx or the application:

```bash
dig pdfedit.sigarda.com SOA +noall +answer
dig +short pdfedit.sigarda.com @1.1.1.1
```

If a per-user Cloudflare token was migrated from an older installation, run
the latest database migrations before saving it again. Migration 049 removes
the legacy single-row `id = 1` constraint while preserving existing tokens.

### Verification Checklist

```bash
# Panel and Agent
curl -fsS http://127.0.0.1:7700/health
curl -fsS http://127.0.0.1:7710/health

# Site job and generated config
sigardapanel site list
sigardapanel job list
nginx -t

# Origin routing
curl -sS -H 'Host: example.com' http://127.0.0.1/

# Public DNS and HTTPS
dig +short example.com @1.1.1.1
curl -skI https://example.com/
```

For the full external-repository workflow, see
[`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md). For endpoint details, see
[`docs/API.md`](docs/API.md).

---

## Tech Stack

| Component | Technology |
|-----------|------------|
| Backend | Go 1.22+, Echo framework |
| Frontend | Svelte 5, SvelteKit, Tailwind CSS |
| Database | SQLite (WAL mode) |
| Auth | Argon2id password hashing |
| Agent | Go, system commands |

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
