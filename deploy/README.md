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
curl -sSL https://github.com/bayurstarcool/SigardaPanel/releases/latest/download/sigardapanel -o /usr/local/bin/sigardapanel
chmod +x /usr/local/bin/sigardapanel

# Run setup
./sigardapanel install

# Start service
./sigardapanel dev
```

### Agent

```bash
# Download binary
curl -sSL https://github.com/bayurstarcool/SigardaPanel/releases/latest/download/sigardapanel -o /usr/local/bin/sigardapanel
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

### Agent

| Variable | Default | Description |
|----------|---------|-------------|
| SIGARDAPANEL_AGENT_TOKEN | - | Agent auth token (required) |
| SIGARDAPANEL_AGENT_ADDR | :9090 | Agent listen address |

## Systemd Service

```bash
cat > /etc/systemd/system/sigardapanel.service << 'UNIT'
[Unit]
Description=SigardaPanel
After=network.target

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/sigardapanel dev
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl enable --now sigardapanel
```
