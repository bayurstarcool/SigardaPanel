#!/usr/bin/env bash
set -euo pipefail

ADMIN_USER="${SIGARDAPANEL_ADMIN_USER:-admin}"
ADMIN_EMAIL="${SIGARDAPANEL_ADMIN_EMAIL:-admin@localhost}"
AGENT_URL="${SIGARDAPANEL_AGENT_URL:-http://127.0.0.1:9090}"
AGENT_TOKEN="${SIGARDAPANEL_AGENT_TOKEN:-}"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/sigardapanel"
DATA_DIR="${SIGARDAPANEL_DATA_DIR:-/var/lib/sigardapanel}"
LOG_DIR="/var/log/sigardapanel"
BACKUP_DIR="/var/backups/sigardapanel"
PANEL_DOMAIN="${PANEL_DOMAIN:-}"
SERVICE_NAME="sigardapanel-panel"
DOWNLOAD_URL="https://github.com/bayurstarcool/SigardaPanel/releases/latest/download/sigardapanel-linux-amd64"
NGINX_CONF="/etc/nginx/sites-available/sigardapanel"

usage() {
    echo "SigardaPanel Panel Installer"
    echo ""
    echo "Usage: PANEL_DOMAIN=panel.example.com ./install-panel.sh [--install-dir DIR]"
    echo ""
    echo "Environment variables:"
    echo "  SIGARDAPANEL_ADMIN_USER   Admin username (default: admin)"
    echo "  SIGARDAPANEL_ADMIN_EMAIL  Admin email (default: admin@localhost)"
    echo "  SIGARDAPANEL_AGENT_URL    Agent URL for worker (default: http://127.0.0.1:9090)"
    echo "  SIGARDAPANEL_AGENT_TOKEN  Bearer token shared with agent"
    echo "  PANEL_DOMAIN              Panel public domain (required)"
    echo "  SIGARDAPANEL_DATA_DIR     Data directory (default: /var/lib/sigardapanel)"
    exit 1
}

while [[ $# -gt 0 ]]; do
    case $1 in
        --install-dir) INSTALL_DIR="$2"; shift 2;;
        --panel-domain) PANEL_DOMAIN="$2"; shift 2;;
        -h|--help) usage;;
        *) echo "Unknown arg: $1"; usage;;
    esac
done

[ -z "$PANEL_DOMAIN" ] && { echo "Error: PANEL_DOMAIN required"; usage; }

if [ "$(id -u)" -ne 0 ]; then
    echo "Error: must run as root"
    exit 1
fi

if [ -f /etc/os-release ]; then
    . /etc/os-release
    OS=$ID
else
    echo "Error: cannot detect OS"
    exit 1
fi

echo "[1/7] Installing dependencies..."
case "$OS" in
    ubuntu|debian)
        apt-get update -qq
        apt-get install -y -qq nginx sqlite3 curl tar certbot python3-certbot-nginx > /dev/null 2>&1 || \
            apt-get install -y -qq nginx sqlite3 curl tar > /dev/null
        ;;
    centos|rhel|fedora|rocky|almalinux)
        yum install -y -q nginx sqlite curl tar
        yum install -y -q certbot python3-certbot-nginx > /dev/null 2>&1 || echo "  certbot install failed (optional)"
        ;;
    *)
        echo "Error: unsupported OS: $OS (supported: ubuntu, debian, centos, rhel, fedora, rocky, almalinux)"
        exit 1
        ;;
esac

echo "[2/7] Downloading SigardaPanel binary..."
mkdir -p "$INSTALL_DIR"
curl -fsSL "$DOWNLOAD_URL" -o "$INSTALL_DIR/sigardapanel"
chmod +x "$INSTALL_DIR/sigardapanel"

echo "[3/7] Creating user & directories..."
id -u sigardapanel > /dev/null 2>&1 || useradd -r -s /usr/sbin/nologin -d "$DATA_DIR" sigardapanel
mkdir -p "$DATA_DIR" "$LOG_DIR" "$BACKUP_DIR" "$CONFIG_DIR"
chown -R sigardapanel:sigardapanel "$DATA_DIR" "$LOG_DIR" "$BACKUP_DIR"
chmod 750 "$DATA_DIR" "$LOG_DIR" "$BACKUP_DIR"

echo "[4/7] Initializing panel..."
export SIGARDAPANEL_DB_PATH="$DATA_DIR/sigardapanel.db"
cat > "$CONFIG_DIR/panel.env" <<EOF
SIGARDAPANEL_API_ADDR=:8080
SIGARDAPANEL_DB_PATH=$DATA_DIR/sigardapanel.db
SIGARDAPANEL_LOG_DIR=$LOG_DIR
SIGARDAPANEL_AGENT_URL=$AGENT_URL
SIGARDAPANEL_AGENT_TOKEN=$AGENT_TOKEN
EOF
chmod 600 "$CONFIG_DIR/panel.env"

if ! "$INSTALL_DIR/sigardapanel" init --user "$ADMIN_USER" --email "$ADMIN_EMAIL" 2>/dev/null; then
    echo "  init skipped (already initialized or no init command)"
fi

echo "[5/7] Creating systemd service..."
cat > "/etc/systemd/system/$SERVICE_NAME.service" <<EOF
[Unit]
Description=SigardaPanel Panel
After=network.target nginx.service

[Service]
Type=simple
User=sigardapanel
Group=sigardapanel
EnvironmentFile=$CONFIG_DIR/panel.env
ExecStart=$INSTALL_DIR/sigardapanel api
Restart=always
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF

echo "[6/7] Configuring nginx reverse proxy..."
cat > "$NGINX_CONF" <<EOF
server {
    listen 80;
    server_name $PANEL_DOMAIN;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_read_timeout 300s;
        proxy_connect_timeout 300s;
        proxy_send_timeout 300s;
    }
}
EOF

if [ -d /etc/nginx/sites-enabled ]; then
    ln -sf "$NGINX_CONF" /etc/nginx/sites-enabled/sigardapanel
    rm -f /etc/nginx/sites-enabled/default 2>/dev/null || true
elif [ -d /etc/nginx/conf.d ]; then
    cp "$NGINX_CONF" /etc/nginx/conf.d/sigardapanel.conf
fi

nginx -t || { echo "Error: nginx config test failed"; exit 1; }

if command -v certbot > /dev/null 2>&1; then
    echo "  Attempting Let's Encrypt SSL for $PANEL_DOMAIN..."
    certbot --nginx -d "$PANEL_DOMAIN" --email "$ADMIN_EMAIL" --agree-tos --no-eff-email --non-interactive 2>/dev/null || \
        echo "  certbot failed (you can retry manually)"
fi

echo "[7/7] Enabling & starting service..."
systemctl daemon-reload
systemctl enable "$SERVICE_NAME"
systemctl start "$SERVICE_NAME"
systemctl reload nginx

sleep 2
if systemctl is-active --quiet "$SERVICE_NAME"; then
    echo "Installation complete!"
    echo ""
    echo "Panel URL: http://$PANEL_DOMAIN"
    echo "Admin: $ADMIN_USER"
    echo ""
    echo "Service status: systemctl status $SERVICE_NAME"
    echo "Logs: journalctl -u $SERVICE_NAME -f"
else
    echo "Service failed to start"
    echo "Check: journalctl -u $SERVICE_NAME -e"
    exit 1
fi
