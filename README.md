# SigardaPanel

**VPS control plane** — self-hosted, multi-server, open source.
Manage nginx, PHP, Node, databases, SSL, backups, firewall from one web UI.

**Version:** 1.0.0
**License:** MIT (recommended)
**Stack:** Go 1.25 (Echo + SQLite) · SvelteKit · 12k+ LOC backend · 18k+ LOC frontend

---

## Features

| Domain | Capability |
|---|---|
| **Multi-Server** | Per-VPS agent (port 7710) — manage fleet from one panel |
| **Sites** | Create, deploy, Git, SSL auto, nginx vhost management |
| **Databases** | MySQL/MariaDB, PostgreSQL, phpMyAdmin integration |
| **Backups** | B2/S3 cloud, scheduled, restore |
| **SSL** | Let's Encrypt DNS-01 (Cloudflare), self-signed, custom |
| **Monitoring** | CPU, RAM, disk, processes (live + 30-day history) |
| **Docker** | Container management per-server |
| **Firewall** | UFW rules per-server, fail2ban integration |
| **Redis** | Stats, info, flush, keyspace |
| **Stack** | Detect/install/restart nginx, PHP (8.0-8.5), Node, Go, Docker, MariaDB, Redis — per-service version manager |
| **Security Shield** | Anti-DDoS, SYN-cookie defense, Fail2ban monitor, web malware & DB exposure scanner |
| **Alerts** | Channel notifications, history |
| **Audit** | Per-user action log |
| **2FA** | TOTP with QR provisioning, recovery codes |
| **Service Tokens** | Machine-to-machine auth with scope-based access control |
| **CLI** | `sigardapanel` — full ops from terminal |

---

## Architecture

```
┌────────────────────────────────────────────────────────────┐
│                   Browser  (SvelteKit UI)                  │
│         https://panel.example.com  (port 443)             │
└────────────┬──────────────────────────────┬──────────────┘
             │ HTTPS                         │ HTTPS
             ▼                               ▼
┌─────────────────────┐         ┌────────────────────────────┐
│   nginx (port 80/443) │         │   SigardaPanel Web         │
│   reverse proxy       │────────▶│   SvelteKit (port 7720)   │
│   + Let's Encrypt     │         │   HttpOnly cookie session  │
└──────────┬────────────┘         └────────┬───────────────────┘
           │                                │ session cookie
           ▼                                ▼
┌─────────────────────────────────────────────────────────────┐
│   SigardaPanel API   (port 7700)                              │
│   - Go 1.25 + Echo                                          │
│   - Cookie + Bearer auth, RBAC, 2FA                        │
│   - SQLite (sigardapanel.db) + PostgreSQL (member)          │
└──────────┬─────────────────────────────────┬───────────────┘
           │ HTTPS + Bearer token             │
           ▼                                  ▼
┌──────────────────────┐         ┌──────────────────────────┐
│  SigardaPanel Agent  │         │  Other VPS Agents        │
│  (port 7710)         │         │  (port 7710 each)        │
│  - nginx restart     │         │  Registered via token    │
│  - apt install PHP   │         │                          │
│  - docker exec       │         │                          │
│  - ufw rule add      │         │                          │
└──────────────────────┘         └──────────────────────────┘
```

### Components

| Component | Port | User | Purpose |
|---|---|---|---|
| `sigardapanel-api` | 127.0.0.1:7700 | `sigardapanel` | REST API, business logic |
| `sigardapanel-agent` | 127.0.0.1:7710 | `root` | Local VPS task executor |
| `sigardapanel-web` | 127.0.0.1:7720 | `sigardapanel` | SvelteKit SSR frontend |

All services bind localhost. Only nginx exposes 80/443 publicly.

---

## Quickstart (5 minutes)

```bash
# 1. Clone
git clone https://github.com/yourorg/sigardapanel.git /opt/sigardapanel/source
cd /opt/sigardapanel/source

# 2. Install (run on fresh Ubuntu 24.04)
sudo bash scripts/install.sh

# 3. Open
http://YOUR_SERVER_IP
admin / <printed-password>
```

See [docs/INSTALL.md](docs/INSTALL.md) for full installation guide.

---

## CLI

```bash
# Health
sigardapanel doctor --api-url http://127.0.0.1:7700

# Users
sigardapanel user list --direct
sigardapanel user reset-password <username> --direct

# Servers / tokens
sigardapanel server list
sigardapanel server token --id 1      # regenerate agent token, sync DB+env

# Login
sigardapanel login
```

Full CLI: `sigardapanel --help`

---

## Security

- **Auth:** Argon2id password hashing, 64-char session tokens (hashed in DB), 7/90-day TTL
- **TLS:** Self-signed by default, Let's Encrypt via DNS-01 (Cloudflare) supported
- **Headers:** HSTS, X-Frame-Options DENY, X-Content-Type-Options nosniff, Referrer-Policy
- **Network:** SSH key-only, UFW 22/80/443, fail2ban, rate limits on login/register
- **Agent:** Constant-time bearer compare, command allowlist, hostname spoofing prevention
- **Hardened systemd:** NoNewPrivileges, ProtectSystem, ReadWritePaths scoped

See [docs/SECURITY.md](docs/SECURITY.md) for threat model + audit log.

---

## Project Layout

```
source/
├── cmd/sigardapanel/         # CLI entry (api, agent, install, dev, ...)
├── internal/
│   ├── apihttp/              # HTTP handlers + middleware
│   ├── agenthttp/            # Agent task handlers
│   ├── store/                # SQLite repositories
│   ├── auth/                 # Argon2id, token, TOTP
│   ├── commands/             # CLI commands
│   ├── migrations/           # DB schema migrations
│   ├── executor/             # Allowlisted command execution
│   ├── audit/                # Action audit log
│   └── config/               # Env config loader
├── web/
│   ├── src/
│   │   ├── routes/api/       # SvelteKit endpoints (proxy → Go API)
│   │   ├── routes/           # UI pages
│   │   └── lib/              # Components, stores
│   └── package.json
├── scripts/
│   └── install.sh            # One-shot installer
└── docs/
    ├── INSTALL.md
    ├── SYSTEMD.md
    ├── API.md
    └── SECURITY.md
```

---

## API

37 endpoints across 18 domains. Browsers authenticate with the HttpOnly
`sp_session` cookie (SameSite=Strict); API clients and the CLI use a Bearer token.

```bash
TOKEN=$(sigardapanel login --user admin)
curl -H "Authorization: Bearer $TOKEN" http://127.0.0.1:7700/api/v1/servers/1/stack
```

Full reference: [docs/API.md](docs/API.md)

---

## License

MIT
