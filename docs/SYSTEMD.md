# Systemd Units and Environment

`install.sh` creates three systemd units. All of them listen on localhost; only
nginx exposes ports 80/443. This page shows the units exactly as the installer
writes them and lists every environment variable the services read. For the
install itself see [INSTALL.md](INSTALL.md).

| Unit | Command | Listens on | User | Environment |
|---|---|---|---|---|
| `sigardapanel-api` | `/usr/local/bin/sigardapanel api` | `127.0.0.1:7700` | `sigardapanel` | `/etc/sigardapanel/panel.env` |
| `sigardapanel-agent` | `/usr/local/bin/sigardapanel agent` | `127.0.0.1:7710` | `root` | `/etc/sigardapanel/agent.env` |
| `sigardapanel-web` | `/usr/bin/node /opt/sigardapanel/web/index.js` | `127.0.0.1:7720` | `sigardapanel` | in the unit |

## `sigardapanel-api.service`

```ini
[Unit]
Description=SigardaPanel API
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=sigardapanel
Group=sigardapanel
EnvironmentFile=/etc/sigardapanel/panel.env
ExecStart=/usr/local/bin/sigardapanel api
Restart=always
RestartSec=3
UMask=0027
NoNewPrivileges=true
PrivateTmp=true
PrivateDevices=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/sigardapanel /var/log/sigardapanel /var/backups/sigardapanel
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictSUIDSGID=true
RestrictRealtime=true
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6
SystemCallArchitectures=native
LockPersonality=true
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
```

On start the API applies pending database migrations, settles the encryption
key (see below), registers the panel host as server 1 and starts the job worker,
the backup scheduler and the agent health checker.

### `panel.env`

The first group is written by the installer; the rest are optional.

| Variable | Code default | Installer value | Description |
|---|---|---|---|
| `SIGARDAPANEL_API_ADDR` | `:7700` | `127.0.0.1:7700` | API listen address |
| `SIGARDAPANEL_PUBLIC_URL` | empty | `https://<DOMAIN or IP>` | Public panel URL. Must be a clean `https://` URL for the agent install endpoint. |
| `SIGARDAPANEL_DB_PATH` | `sigardapanel.db` (working directory) | `/var/lib/sigardapanel/sigardapanel.db` | SQLite database |
| `SIGARDAPANEL_LOG_DIR` | - | `/var/log/sigardapanel` | Reserved; the services log to the journal |
| `SIGARDAPANEL_AGENT_URL` | `http://localhost:7710` | `http://127.0.0.1:7710` | Agent used for the local server |
| `SIGARDAPANEL_AGENT_TOKEN` | empty | random 64 hex | Shared secret with the local agent. Must equal the value in `agent.env`; the API copies it into the database on start. |
| `SIGARDAPANEL_ENCRYPTION_KEY` | empty | random 64 hex | AES-256 key (32 bytes hex) for secrets at rest |
| `SIGARDAPANEL_ALLOW_REGISTRATION` | `false` (`true` before v1.6.0) | `false` | Public sign-up (`POST /api/v1/auth/register`) |
| `SIGARDAPANEL_LANDING_PAGE` | `false` (always on before v1.6.1) | - | `true`: `/` and `/pricing` show the Sigarda SaaS landing (pricing, "Contact Sales"). `false`: they redirect to `/login` (or `/dashboard` when signed in). Reported to the web by `GET /api/v1/public/config`. |
| `SIGARDAPANEL_ALLOW_MEMBER_SIGNUP` | `false` (open before v1.6.1) | - | `true`: `POST /api/v1/member/register` creates member (SaaS customer) accounts; also needs `MEMBER_DB_HOST`. `false`: 403 `member_signup_disabled`, and `/register` says sign-up is closed. |
| `SIGARDAPANEL_ENCRYPTION_KEY_FILE` | `encryption.key` next to the database | - | Key file used when `SIGARDAPANEL_ENCRYPTION_KEY` is not set |
| `SIGARDAPANEL_LOCAL_IP` | auto | - | Address shown for the panel host (useful behind NAT) |
| `SIGARDAPANEL_WORKER_CONCURRENCY` | `3` | - | Parallel background jobs (1-16) |
| `SIGARDAPANEL_API_TLS_CERT`, `SIGARDAPANEL_API_TLS_KEY` | empty | - | Serve the API over TLS directly (not needed behind nginx) |
| `SIGARDAPANEL_API_READ_TIMEOUT`, `SIGARDAPANEL_API_WRITE_TIMEOUT` | `10`, `30` | - | Seconds |
| `SIGARDAPANEL_MYSQL_ROOT_PASSWORD` | empty | - | If set, a default MySQL/MariaDB database server is registered on first start |
| `SIGARDAPANEL_PMA_SECRET` | empty | - | Shared secret (min. 32 characters) for the signed phpMyAdmin sign-on bridge |
| `MEMBER_DB_HOST` | empty | commented out | Enables the optional member area (PostgreSQL) |
| `MEMBER_DB_PORT`, `MEMBER_DB_USER`, `MEMBER_DB_PASSWORD`, `MEMBER_DB_NAME` | `5432`, `sigardapanel`, empty, `sigardapanel_member` | commented out | Member area connection |
| `SIGARDAPANEL_DEV` | empty | - | `true` only for local development (non-secure cookies, no rate limits, skips Cloudflare token checks). Never set it in production. |

**Encryption key.** The key is chosen in this order:

1. `SIGARDAPANEL_ENCRYPTION_KEY` (installer-made panels use this);
2. the key file (`SIGARDAPANEL_ENCRYPTION_KEY_FILE`, default `encryption.key` in
   the database directory). On a new database without a configured key, the API
   generates this file (mode 600) on first start;
3. a legacy key derived from the database path and agent token. It is only kept
   so that existing secrets encrypted with it still decrypt, and is logged as a
   security warning.

Changing or losing the key makes stored secrets (database passwords, job
credentials, 2FA secrets) unreadable. Back it up with the database.

## `sigardapanel-agent.service`

```ini
[Unit]
Description=SigardaPanel Agent
After=network-online.target nginx.service
Wants=network-online.target

[Service]
Type=simple
User=root
EnvironmentFile=/etc/sigardapanel/agent.env
ExecStart=/usr/local/bin/sigardapanel agent
Restart=always
RestartSec=3
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
LockPersonality=true
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
```

The agent runs as root because it manages nginx, packages, services, the
firewall and Docker. Every task endpoint requires the bearer token.

### `agent.env`

| Variable | Code default | Installer value | Description |
|---|---|---|---|
| `SIGARDAPANEL_AGENT_ADDR` | `:7710` (all interfaces) | `127.0.0.1:7710` | Listen address. On a remote server use the address the panel connects to and firewall it. |
| `SIGARDAPANEL_AGENT_TOKEN` | empty | same as `panel.env` | Bearer token the panel must send |
| `SIGARDAPANEL_AGENT_SITE_ROOT` | empty (no restriction) | - | Restrict site file operations to this directory |
| `SIGARDAPANEL_AGENT_TLS_CERT`, `SIGARDAPANEL_AGENT_TLS_KEY` | empty | - | Serve the agent over HTTPS (the panel verifies the certificate) |
| `SIGARDAPANEL_AGENT_READ_TIMEOUT`, `SIGARDAPANEL_AGENT_WRITE_TIMEOUT` | `10`, `900` | - | Seconds |
| `SIGARDAPANEL_AGENT_ALLOW_STUB_SSL` | `false` | - | Test-only: return stub certificates. Keep `false`. |

## `sigardapanel-web.service`

```ini
[Unit]
Description=SigardaPanel Web
After=sigardapanel-api.service
Wants=sigardapanel-api.service

[Service]
Type=simple
User=sigardapanel
Group=sigardapanel
WorkingDirectory=/opt/sigardapanel/web
Environment=NODE_ENV=production
Environment=HOST=127.0.0.1
Environment=PORT=7720
Environment=ORIGIN=https://panel.example.com
Environment=SIGARDAPANEL_API_URL=http://127.0.0.1:7700
ExecStart=/usr/bin/node /opt/sigardapanel/web/index.js
Restart=always
RestartSec=3
NoNewPrivileges=true
PrivateTmp=true
PrivateDevices=true
ProtectSystem=strict
ProtectHome=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictSUIDSGID=true
RestrictRealtime=true
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6
SystemCallArchitectures=native
LockPersonality=true

[Install]
WantedBy=multi-user.target
```

The web app is the prebuilt SvelteKit (adapter-node) bundle from the release; it
only needs the Node.js runtime. `ORIGIN` must be the public URL of the panel
(the installer uses `https://<DOMAIN>` or `https://<server-ip>`).
`SIGARDAPANEL_API_URL` is where the web app forwards API requests.

## Operations

```bash
# Status
systemctl status sigardapanel-api sigardapanel-agent sigardapanel-web

# Restart after editing an env file
systemctl restart sigardapanel-api

# Logs
journalctl -u sigardapanel-api -f
journalctl -u sigardapanel-agent -f
journalctl -u sigardapanel-web -f

# Stop / disable
systemctl stop sigardapanel-web sigardapanel-api sigardapanel-agent
systemctl disable sigardapanel-web sigardapanel-api sigardapanel-agent
```

Do not edit the unit files for upgrades: `install.sh update` only replaces the
binary and web bundle. If you change a unit, run `systemctl daemon-reload`.

## Remote servers

A remote server runs only `sigardapanel-agent` (same unit, `agent.env` with the
token the panel generated and an address the panel can reach). See
[INSTALL.md - Adding remote servers](INSTALL.md#adding-remote-servers).

## Why the services run as different users

| Service | User | Why |
|---|---|---|
| API | `sigardapanel` | Needs no root; sandboxed, can only write its data, log and backup directories. |
| Agent | `root` | Manages system services and packages. Token-gated, listens on localhost on the panel host. |
| Web | `sigardapanel` | Serves the UI and proxies to the API; read-only filesystem. |
