# SigardaPanel Deployment

## Quick Install — Panel

On panel server (Ubuntu/Debian):

```bash
curl -fsSL https://raw.githubusercontent.com/bayurstarcool/SigardaPanel/main/deploy/install-panel.sh | \
  sudo bash -s -- --panel-domain panel.example.com
```

## Quick Install — Agent

On target VPS (Ubuntu/Debian):

```bash
curl -fsSL https://raw.githubusercontent.com/bayurstarcool/SigardaPanel/main/deploy/install-agent.sh | \
  sudo bash -s -- --panel-url https://panel.example.com --token YOUR_AGENT_TOKEN
```

## Manual Install

### Panel

```bash
# Download binary
curl -sSL https://github.com/bayurstarcool/SigardaPanel/releases/latest/download/sigardapanel-linux-amd64 -o /usr/local/bin/sigardapanel
chmod +x /usr/local/bin/sigardapanel

# Run setup
sigardapanel install

# Start service
sigardapanel dev
```

### Agent

```bash
# Download binary
curl -sSL https://github.com/bayurstarcool/SigardaPanel/releases/latest/download/sigardapanel-linux-amd64 -o /usr/local/bin/sigardapanel
chmod +x /usr/local/bin/sigardapanel

# Start with token
SIGARDAPANEL_AGENT_TOKEN=<token> sigardapanel agent
```

## Environment Variables

### Panel

| Variable | Default | Description |
|----------|---------|-------------|
| PANEL_DOMAIN | - | Panel domain (required) |
| SIGARDAPANEL_ADMIN_USER | admin | Admin username |
| SIGARDAPANEL_ADMIN_EMAIL | admin@localhost | Admin email |
| SIGARDAPANEL_AGENT_TOKEN | - | Agent auth token |
| SIGARDAPANEL_AGENT_URL | http://127.0.0.1:7710 | Agent URL for worker |

### Agent

| Variable | Default | Description |
|----------|---------|-------------|
| SIGARDAPANEL_AGENT_TOKEN | - | Agent auth token (required) |
| SIGARDAPANEL_AGENT_ADDR | :7710 | Agent listen address |

## Systemd Service

### Panel

```bash
cat > /etc/systemd/system/sigardapanel-panel.service << 'UNIT'
[Unit]
Description=SigardaPanel Panel
After=network.target nginx.service

[Service]
Type=simple
User=sigardapanel
Group=sigardapanel
EnvironmentFile=/etc/sigardapanel/panel.env
ExecStart=/usr/local/bin/sigardapanel api
Restart=always
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl enable --now sigardapanel-panel
```

### Agent

```bash
cat > /etc/systemd/system/sigardapanel-agent.service << 'UNIT'
[Unit]
Description=SigardaPanel Agent
After=network.target nginx.service

[Service]
Type=simple
User=root
EnvironmentFile=/etc/sigardapanel/agent.env
ExecStart=/usr/local/bin/sigardapanel agent
Restart=always
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl enable --now sigardapanel-agent
```

## Ports

| Service | Port | Description |
|---------|------|-------------|
| API | 7700 | Panel API server |
| Agent | 7710 | Agent HTTP server |
| Frontend | 7720 | SvelteKit web dashboard |
