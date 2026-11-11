# SigardaPanel

**VPS control plane** — self-hosted, multi-server, open source.
Manage nginx, PHP, Node, databases, SSL, backups, firewall from one web UI.

[Latest release](https://github.com/bayurstarcool/SigardaPanel/releases/latest) ·
[Install guide](docs/INSTALL.md) · License: MIT ·
Stack: Go (Echo + SQLite) · SvelteKit

---

## Quick install

On a fresh **Ubuntu 24.04** (tested) or **Debian 12** server, amd64 or arm64, as root:

```bash
curl -fsSLO https://raw.githubusercontent.com/bayurstarcool/SigardaPanel/main/install.sh
less install.sh                     # read it first - it is a plain shell script
sudo bash install.sh
```

The installer downloads the latest signed release (prebuilt binary and web
bundle - no Go or npm needed), verifies it, sets up nginx with HTTPS and the three
services, and prints the panel URL and a random admin password (also saved to
`/root/.sigardapanel_admin_cred`). There are no default credentials.

Common options: `DOMAIN=panel.example.com LETSENCRYPT_EMAIL=you@example.com`
(trusted certificate), `ADMIN_USER`/`ADMIN_EMAIL`/`ADMIN_PASS`,
`INSTALL_FIREWALL=yes`, `SIGARDA_VERSION=v1.5.0` - e.g.
`sudo DOMAIN=panel.example.com LETSENCRYPT_EMAIL=you@example.com bash install.sh`.

Update later (backup + automatic rollback):

```bash
sudo bash install.sh update
```

**[docs/INSTALL.md](docs/INSTALL.md) is the complete guide**: requirements, all
options, first login, ports and firewall, update, rollback, backups, adding remote
servers, uninstall and troubleshooting.

## Verifying releases

Every release contains `SHA256SUMS` and `SHA256SUMS.minisig`, signed with this
[minisign](https://jedisct1.github.io/minisign/) key:

```
RWTNXRQ/oh97GPfWH0tDwxbnMTe/WTZeoJdZtGTW3ESF0OpTSlNXpFXI
```

`install.sh` pins this key and refuses to install anything that fails the
signature or checksum check. To verify by hand:

```bash
V=v1.5.0
base=https://github.com/bayurstarcool/SigardaPanel/releases/download/$V
for f in SHA256SUMS SHA256SUMS.minisig sigardapanel-linux-amd64 sigardapanel-web.tar.gz; do
  curl -fsSLO "$base/$f"
done
minisign -Vm SHA256SUMS -P RWTNXRQ/oh97GPfWH0tDwxbnMTe/WTZeoJdZtGTW3ESF0OpTSlNXpFXI
sha256sum -c --ignore-missing SHA256SUMS
```

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
│   - SQLite (sigardapanel.db), optional PostgreSQL (member)  │
└──────────┬─────────────────────────────────┬───────────────┘
           │ HTTP(S) + bearer token           │
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

All services bind localhost. Only nginx exposes 80/443 publicly. Agents on
remote servers listen on 7710; allow that port only from the panel
(see [Adding remote servers](docs/INSTALL.md#adding-remote-servers)).

---

## CLI

```bash
# Log in once (token is stored in ~/.config/sigardapanel/)
sigardapanel login --api-url http://127.0.0.1:7700 --user admin

# Health
sigardapanel doctor --api-url http://127.0.0.1:7700

# Servers / sites / users
sigardapanel server list
sigardapanel site list
sigardapanel user list
```

Full CLI: `sigardapanel --help` and [docs/CLI.md](docs/CLI.md).

---

## Security

- **Auth:** Argon2id password hashing, 64-char session tokens (hashed in DB), 7/90-day TTL
- **Releases:** minisign-signed checksums, verified by the installer with a pinned key
- **TLS:** Self-signed by default, Let's Encrypt via the installer (`DOMAIN` + `LETSENCRYPT_EMAIL`); sites can use DNS-01 (Cloudflare)
- **Headers:** HSTS, X-Frame-Options DENY, X-Content-Type-Options nosniff, Referrer-Policy
- **Network:** services on localhost behind nginx, rate limits on the API and login; recommended: UFW 22/80/443 (`INSTALL_FIREWALL=yes`), SSH keys, fail2ban
- **Accounts:** no default password, public sign-up disabled by the installer, optional TOTP 2FA
- **Agent:** Constant-time bearer compare, command allowlist, hostname spoofing prevention
- **Hardened systemd:** NoNewPrivileges, ProtectSystem, ReadWritePaths scoped

See [docs/SECURITY.md](docs/SECURITY.md) for the threat model and
[SECURITY.md](SECURITY.md) to report a vulnerability.

---

## Project Layout

```
.
├── install.sh                # Installer (copy of installer/install.sh, kept identical)
├── cmd/sigardapanel/         # Single binary entry (api, agent, CLI, dev)
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
├── installer/                # Installer source (install.sh)
├── scripts/
│   └── release.sh            # Build, checksum and sign a release
└── docs/
    ├── INSTALL.md            # Installation and operations guide
    ├── SYSTEMD.md            # Units and environment variables
    ├── CLI.md
    ├── API.md
    └── SECURITY.md
```

---

## API

REST API under `/api/v1`. Browsers authenticate with the HttpOnly
`sp_session` cookie (SameSite=Strict); API clients and the CLI use a Bearer token.

```bash
TOKEN=$(curl -s -X POST http://127.0.0.1:7700/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"..."}' | jq -r .data.token)
curl -H "Authorization: Bearer $TOKEN" http://127.0.0.1:7700/api/v1/servers/1/stack
```

Full reference: [docs/API.md](docs/API.md)

---

## License

MIT
