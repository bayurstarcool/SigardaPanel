# Installation Guide

SigardaPanel v0.5.0 — Installation and setup guide.

## Quick Start

### 1. Download Binary

```bash
curl -sSL https://github.com/bayurstarcool/SigardaPanel/releases/latest/download/sigardapanel -o sigardapanel
chmod +x sigardapanel
```

### 2. Run Setup Wizard

```bash
./sigardapanel install
```

This will:
- Create database and run migrations
- Create admin user
- Configure default settings

### 3. Start Services

```bash
# Development (API + Agent)
./sigardapanel dev

# Production (separate)
./sigardapanel api &
./sigardapanel agent &
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| SIGARDAPANEL_API_ADDR | :7700 | API listen address |
| SIGARDAPANEL_AGENT_ADDR | :7710 | Agent listen address |
| SIGARDAPANEL_DB_PATH | sigardapanel.db | Database path |
| SIGARDAPANEL_AGENT_TOKEN | - | Agent auth token |

## Default Credentials

- Username: `admin`
- Password: `admin123`

**Important:** Change the default password after first login!

## Frontend Setup

```bash
cd web
npm install
npm run dev
```

Frontend runs on port 4000 (dev) or 4001 (production).

## Agent Installation on VPS

```bash
# Download agent binary
curl -sSL https://github.com/bayurstarcool/SigardaPanel/releases/latest/download/sigardapanel -o /usr/local/bin/sigardapanel
chmod +x /usr/local/bin/sigardapanel

# Start agent with token from panel
SIGARDAPANEL_AGENT_TOKEN=<token> sigardapanel agent
```

## Systemd Service

```bash
cat > /etc/systemd/system/sigardapanel.service << 'EOF'
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
EOF

systemctl daemon-reload
systemctl enable --now sigardapanel
```

## Nginx Configuration

```nginx
server {
    listen 443 ssl http2;
    server_name panel.example.com;

    ssl_certificate /etc/letsencrypt/live/panel.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/panel.example.com/privkey.pem;

    location /api/v1/ {
        proxy_pass http://127.0.0.1:7700;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }

    location / {
        proxy_pass http://127.0.0.1:7720;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

## Troubleshooting

### Agent not connecting
- Check port 9090 is open
- Verify SIGARDAPANEL_AGENT_TOKEN matches
- Check agent logs

### Frontend blank page
- Hard refresh (Ctrl+Shift+R)
- Check browser console for errors
- Verify API is running on port 8080

### Database locked
- Stop all services
- Delete sigardapanel.db
- Restart services
