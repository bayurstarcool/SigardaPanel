# SigardaPanel v0.5.0

SigardaPanel adalah panel VPS berbasis Go dengan satu binary utama: `sigardapanel`.

## Features

### Core
- **Server Management** — Add, update, delete, health monitoring
- **Site Management** — Create, update, delete, deploy, git integration
- **SSL Certificates** — Auto-issue, auto-renew, expiry alerts
- **Databases** — MySQL/PostgreSQL create, manage, users
- **Backups** — Local + S3, scheduled, restore

### DevOps
- **Docker Management** — Containers, images, volumes, networks, compose
- **Git Deploy v2** — Branches, commit history, rollback, checkout
- **Firewall (UFW)** — Rules, presets, IP blocking, enable/disable
- **Redis/Memcached** — Stats, flush, info, management
- **Stack Manager** — Install/manage nginx, php, node, docker, etc.

### Monitoring & Security
- **System Monitoring** — CPU, memory, disk, network metrics
- **SSL Dashboard** — Visual certificate status & expiry timeline
- **Disk Usage Alerts** — 80% warning, 90% critical notifications
- **Audit Logs** — Track all admin actions

### UX
- **Global Search** — Cmd+K search across servers, sites, databases
- **Server Health Overview** — Dashboard with real-time metrics
- **Dark/Light Theme** — Toggle between themes
- **Mobile Responsive** — Works on all devices

## CLI Commands

```
sigardapanel
├── api / agent / dev / install / init / login / logout / doctor / version
├── server    add|list|update|remove|doctor
├── site      create|list|update|delete|deploy|config|setup-logrotate
├── ssl       status|issue|renew|renew-all
├── job       list|watch|cancel|logs
├── backup    create|list|delete|restore
├── db        create|list|update|delete
│   └── user  create|list|rotate-password|delete
├── docker    ps|start|stop|restart|rm|logs|images|compose
├── git       branches|log|rollback|checkout
├── firewall  status|enable|disable|allow|deny|rules|delete|reset
├── redis     stats|info|flush|flushdb
├── system    disk-usage|updates
├── channels  add|list|remove
├── alerts    list
├── metrics   latest|list
├── user      create|list|reset-password|delete
└── cloudpanel scan|import-sites|import-ssl|doctor
```

## API Endpoints (~150+)

| Domain | Endpoints |
|---|---|
| Auth | 5 |
| Users | 12 |
| Servers | 8 |
| Sites | 15 |
| App Runtime | 7 |
| Databases | 14 |
| Backups | 16 |
| SSL | 4 |
| Jobs | 3 |
| Cloudflare | 14 |
| Stack | 4 |
| Docker | 19 |
| Git | 4 |
| Firewall | 8 |
| Redis | 4 |
| License/Plans | 9 |
| Notifications | 6 |
| Alerts | 4 |
| Service Tokens | 3 |

## License

MIT License — see [LICENSE](LICENSE) for details.

## Enterprise

Panel API, dashboard, dan licensing tersedia di repo private:
[SigardaPanel-Enterprise](https://github.com/bayurstarcool/SigardaPanel-Enterprise)

## Quick Start

### Development

```bash
git clone https://github.com/bayurstarcool/SigardaPanel-Enterprise.git
cd SigardaPanel-Enterprise
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

### Default Credentials

- Username: `admin`
- Password: `admin123`

### Environment Variables

| Variable | Default | Description |
|---|---|---|
| SIGARDAPANEL_API_ADDR | :8080 | API listen address |
| SIGARDAPANEL_AGENT_ADDR | :9090 | Agent listen address |
| SIGARDAPANEL_DB_PATH | sigardapanel.db | Database path |
| SIGARDAPANEL_AGENT_TOKEN | - | Agent auth token |

## Architecture

```
┌──────────────────────────────────────────────────┐
│               Panel Server                       │
│  ┌──────────────┐    ┌──────────────────┐        │
│  │  API (:8080) │    │  Frontend (:4001)│        │
│  │  (Go/Echo)   │    │  (SvelteKit)     │        │
│  └──────┬───────┘    └────────┬─────────┘        │
│         │                     │                   │
│  ┌──────▼─────────────────────▼─────────┐         │
│  │         SQLite DB (WAL)              │         │
│  └──────────────────────────────────────┘         │
└──────────────────────┬───────────────────────────┘
                       │
        ┌──────────────▼──────────────┐
        │   Agent (:9090) per VPS     │
        │  • Site management          │
        │  • SSL certificates         │
        │  • Docker management        │
        │  • Firewall (UFW)           │
        │  • System metrics           │
        └─────────────────────────────┘
```

## Tech Stack

- **Backend**: Go 1.22+, Echo framework
- **Frontend**: Svelte 5, SvelteKit, Tailwind CSS
- **Database**: SQLite (WAL mode)
- **Auth**: Argon2id password hashing, JWT-like tokens

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## Links

- [GitHub](https://github.com/bayurstarcool/SigardaPanel)
- [Releases](https://github.com/bayurstarcool/SigardaPanel/releases)
- [Documentation](https://panel.sigarda.dev/docs)
