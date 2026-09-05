# Systemd Units

Three systemd units, all bind to localhost. Only nginx exposes public ports.

## `sigardapanel-api.service`

**Binary:** `/usr/local/bin/sigardapanel api`
**Port:** 127.0.0.1:7700
**User:** `sigardapanel` (read DB)
**Env:** `/etc/sigardapanel/panel.env`

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
Restart=on-failure
RestartSec=3
ReadWritePaths=/var/lib/sigardapanel /var/log/sigardapanel /var/backups/sigardapanel
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
```

### Environment Variables (`panel.env`)

| Var | Default | Description |
|---|---|---|
| `SIGARDAPANEL_API_ADDR` | `127.0.0.1:7700` | Listen address |
| `SIGARDAPANEL_PUBLIC_URL` | (auto-detect) | Used in install scripts (Let's Encrypt) |
| `SIGARDAPANEL_DB_PATH` | `/var/lib/sigardapanel/sigardapanel.db` | SQLite database |
| `SIGARDAPANEL_LOG_DIR` | `/var/log/sigardapanel` | Logs |
| `SIGARDAPANEL_AGENT_URL` | `http://127.0.0.1:7710` | Local agent endpoint |
| `SIGARDAPANEL_AGENT_TOKEN` | (generated) | Shared secret with agent |
| `SIGARDAPANEL_ENCRYPTION_KEY` | (generated) | DB column encryption |
| `MEMBER_DB_HOST` | `127.0.0.1` | Member DB host (optional) |
| `MEMBER_DB_PORT` | `3306` | Member DB port |
| `MEMBER_DB_USER` | `sigardapanel` | Member DB user |
| `MEMBER_DB_PASSWORD` | (generated) | Member DB password |
| `MEMBER_DB_NAME` | `sigardapanel_member` | Member DB name |

---

## `sigardapanel-agent.service`

**Binary:** `/usr/local/bin/sigardapanel agent`
**Port:** 127.0.0.1:7710
**User:** `root` (needs root for apt, systemctl, ufw)
**Env:** `/etc/sigardapanel/agent.env`

```ini
[Unit]
Description=SigardaPanel Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
EnvironmentFile=/etc/sigardapanel/agent.env
ExecStart=/usr/local/bin/sigardapanel agent
Restart=on-failure
RestartSec=3
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
```

> **Note:** Agent runs as root because it manages system services (apt, systemctl, ufw, docker). Token auth controls who can call it.

### Environment Variables (`agent.env`)

| Var | Default | Description |
|---|---|---|
| `SIGARDAPANEL_AGENT_ADDR` | `:7710` | Listen address |
| `SIGARDAPANEL_PANEL_URL` | `http://127.0.0.1:7700` | Panel API URL |
| `SIGARDAPANEL_AGENT_TOKEN` | (generated) | Shared secret with panel |
| `SIGARDAPANEL_AGENT_SITE_ROOT` | `/var/www` | Where site roots live |

---

## `sigardapanel-web.service`

**Binary:** `node /opt/sigardapanel/web/index.js`
**Port:** 127.0.0.1:7720
**User:** `sigardapanel` (read-only on web/)
**Workdir:** `/opt/sigardapanel/web`

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
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
```

> **Important:** `SIGARDAPANEL_API_URL` in the web unit tells the SvelteKit proxy where to forward API requests. This was hardcoded to `localhost:7700` in v0.5.x; v1.0.0+ reads it from env. Multi-server installs must set it correctly.

---

## Operations

```bash
# Status
systemctl status sigardapanel-{api,agent,web}

# Restart
systemctl restart sigardapanel-api

# Logs
journalctl -u sigardapanel-api -f
journalctl -u sigardapanel-agent -f
journalctl -u sigardapanel-web -f

# Stop
systemctl stop sigardapanel-{api,agent,web}

# Disable on boot
systemctl disable sigardapanel-{api,agent,web}
```

## Multi-Server (Remote Agents)

To add a remote VPS:

1. **On the remote VPS:** install only the agent:
   ```bash
   SIGARDAPANEL_PANEL_URL=https://panel.example.com \
   bash <(curl -sL https://panel.example.com/api/v1/agents/install)
   ```

2. **The install script** registers the agent with the panel using a one-time registration token. The agent then receives its permanent token.

3. **The panel** stores the agent token in the `servers.agent_token` column.

## Why Each Service Runs as Different User

| Service | User | Why |
|---|---|---|
| API | `sigardapanel` | Doesn't need root. Can't be exploited into root. |
| Agent | `root` | Needs to manage system services (apt, systemctl). Token-gated. |
| Web | `sigardapanel` | Static files only. No writes. |
