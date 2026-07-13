#!/usr/bin/env bash
set -euo pipefail

# ╔══════════════════════════════════════════════════════════╗
# ║  SigardaPanel Panel Installer                           ║
# ║                                                         ║
# ║  One-line install:                                      ║
# ║    curl -sSL https://raw.githubusercontent.com/...      ║
# ║      .../install-panel.sh | sudo bash -s --             ║
# ║      --panel-domain panel.example.com                   ║
# ║                                                         ║
# ║  Manual install:                                        ║
# ║    PANEL_DOMAIN=panel.example.com                       ║
# ║      ./install-panel.sh --install-dir /usr/local/bin    ║
# ╚══════════════════════════════════════════════════════════╝

# ── Configuration ──────────────────────────────────────────
ADMIN_USER="${SIGARDAPANEL_ADMIN_USER:-admin}"
ADMIN_EMAIL="${SIGARDAPANEL_ADMIN_EMAIL:-admin@localhost}"
AGENT_URL="${SIGARDAPANEL_AGENT_URL:-http://127.0.0.1:7710}"
AGENT_TOKEN="${SIGARDAPANEL_AGENT_TOKEN:-}"
INSTALL_DIR="${SIGARDAPANEL_INSTALL_DIR:-/usr/local/bin}"
CONFIG_DIR="${SIGARDAPANEL_CONFIG_DIR:-/etc/sigardapanel}"
DATA_DIR="${SIGARDAPANEL_DATA_DIR:-/var/lib/sigardapanel}"
LOG_DIR="${SIGARDAPANEL_LOG_DIR:-/var/log/sigardapanel}"
BACKUP_DIR="${SIGARDAPANEL_BACKUP_DIR:-/var/backups/sigardapanel}"
PANEL_DOMAIN="${PANEL_DOMAIN:-}"
SERVICE_NAME="sigardapanel-panel"
DOWNLOAD_BASE="https://github.com/bayurstarcool/SigardaPanel/releases/latest/download"
NGINX_CONF="/etc/nginx/sites-available/sigardapanel"
VERSION="0.5.4"

# ── Colors ─────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info()  { echo -e "${BLUE}[INFO]${NC}  $1"; }
log_ok()    { echo -e "${GREEN}[  OK]${NC}  $1"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC}  $1"; }
log_error() { echo -e "${RED}[FAIL]${NC}  $1"; }

# ── Usage ──────────────────────────────────────────────────
usage() {
    cat << EOF
SigardaPanel Panel Installer v${VERSION}

Usage:
  PANEL_DOMAIN=panel.example.com ./install-panel.sh [OPTIONS]

Options:
  --install-dir DIR      Install directory (default: /usr/local/bin)
  --panel-domain DOMAIN  Panel domain (required, or set PANEL_DOMAIN env)
  -h, --help             Show this help

Environment Variables:
  PANEL_DOMAIN              Panel domain (required)
  SIGARDAPANEL_ADMIN_USER   Admin username (default: admin)
  SIGARDAPANEL_ADMIN_EMAIL  Admin email (default: admin@localhost)
  SIGARDAPANEL_AGENT_URL    Agent URL (default: http://127.0.0.1:7710)
  SIGARDAPANEL_AGENT_TOKEN  Bearer token shared with agent
  SIGARDAPANEL_INSTALL_DIR  Install directory (default: /usr/local/bin)
  SIGARDAPANEL_CONFIG_DIR   Config directory (default: /etc/sigardapanel)
  SIGARDAPANEL_DATA_DIR     Data directory (default: /var/lib/sigardapanel)

Examples:
  # Basic install
  PANEL_DOMAIN=panel.example.com ./install-panel.sh

  # Custom ports and paths
  SIGARDAPANEL_AGENT_TOKEN=abc123 PANEL_DOMAIN=panel.example.com \\
    ./install-panel.sh --install-dir /opt/sigardapanel
EOF
    exit 1
}

# ── Parse arguments ────────────────────────────────────────
while [[ $# -gt 0 ]]; do
    case $1 in
        --install-dir) INSTALL_DIR="$2"; shift 2;;
        --panel-domain) PANEL_DOMAIN="$2"; shift 2;;
        -h|--help) usage;;
        *) log_error "Unknown argument: $1"; usage;;
    esac
done

# ── Validate ───────────────────────────────────────────────
if [ -z "$PANEL_DOMAIN" ]; then
    log_error "PANEL_DOMAIN is required"
    echo ""
    usage
fi

if [ "$(id -u)" -ne 0 ]; then
    log_error "This installer must be run as root"
    echo "  Try: sudo $0 --panel-domain $PANEL_DOMAIN"
    exit 1
fi

# ── Detect OS ──────────────────────────────────────────────
if [ -f /etc/os-release ]; then
    . /etc/os-release
    OS=$ID
    OS_VERSION="${VERSION_ID:-}"
else
    log_error "Cannot detect operating system"
    exit 1
fi

echo ""
echo -e "${BLUE}╔══════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║       SigardaPanel Panel Installer v${VERSION}        ║${NC}"
echo -e "${BLUE}╚══════════════════════════════════════════════════╝${NC}"
echo ""
log_info "Domain:    $PANEL_DOMAIN"
log_info "OS:        $OS $OS_VERSION"
log_info "Install:   $INSTALL_DIR"
echo ""

# ── Step 1: Install dependencies ───────────────────────────
log_info "[1/8] Installing dependencies..."
case "$OS" in
    ubuntu|debian)
        apt-get update -qq
        apt-get install -y -qq nginx sqlite3 curl tar > /dev/null 2>&1
        apt-get install -y -qq certbot python3-certbot-nginx > /dev/null 2>&1 || \
            log_warn "certbot install failed (optional, SSL can be configured later)"
        ;;
    centos|rhel|fedora|rocky|almalinux)
        yum install -y -q nginx sqlite curl tar
        yum install -y -q certbot python3-certbot-nginx > /dev/null 2>&1 || \
            log_warn "certbot install failed (optional)"
        ;;
    *)
        log_error "Unsupported OS: $OS"
        echo "  Supported: ubuntu, debian, centos, rhel, fedora, rocky, almalinux"
        exit 1
        ;;
esac
log_ok "Dependencies installed"

# ── Step 2: Download binary ────────────────────────────────
log_info "[2/8] Downloading SigardaPanel v${VERSION}..."
mkdir -p "$INSTALL_DIR"

DOWNLOAD_URL="${DOWNLOAD_BASE}/sigardapanel-linux-amd64"
if curl -fsSL "$DOWNLOAD_URL" -o "$INSTALL_DIR/sigardapanel" 2>/dev/null; then
    chmod +x "$INSTALL_DIR/sigardapanel"
    log_ok "Binary downloaded to $INSTALL_DIR/sigardapanel"
else
    log_error "Failed to download binary from $DOWNLOAD_URL"
    echo "  Please check your network connection and try again."
    exit 1
fi

# ── Step 3: Create user & directories ──────────────────────
log_info "[3/8] Creating user and directories..."
if ! id -u sigardapanel > /dev/null 2>&1; then
    useradd -r -s /usr/sbin/nologin -d "$DATA_DIR" sigardapanel
    log_ok "Created user: sigardapanel"
else
    log_ok "User sigardapanel already exists"
fi

mkdir -p "$DATA_DIR" "$LOG_DIR" "$BACKUP_DIR" "$CONFIG_DIR"
chown -R sigardapanel:sigardapanel "$DATA_DIR" "$LOG_DIR" "$BACKUP_DIR"
chmod 750 "$DATA_DIR" "$LOG_DIR" "$BACKUP_DIR"
log_ok "Directories configured"

# ── Step 4: Generate agent token ───────────────────────────
log_info "[4/8] Configuring agent token..."
if [ -z "$AGENT_TOKEN" ]; then
    AGENT_TOKEN=$(openssl rand -hex 32)
    log_ok "Generated new agent token"
else
    log_ok "Using provided agent token"
fi

# ── Step 5: Create configuration ───────────────────────────
log_info "[5/8] Creating configuration..."
cat > "$CONFIG_DIR/panel.env" <<EOF
SIGARDAPANEL_API_ADDR=:7700
SIGARDAPANEL_AGENT_ADDR=:7710
SIGARDAPANEL_DB_PATH=$DATA_DIR/sigardapanel.db
SIGARDAPANEL_LOG_DIR=$LOG_DIR
SIGARDAPANEL_AGENT_URL=$AGENT_URL
SIGARDAPANEL_AGENT_TOKEN=$AGENT_TOKEN
EOF
chmod 600 "$CONFIG_DIR/panel.env"
log_ok "Configuration written to $CONFIG_DIR/panel.env"

# ── Step 6: Initialize database ────────────────────────────
log_info "[6/8] Initializing database..."
export SIGARDAPANEL_DB_PATH="$DATA_DIR/sigardapanel.db"
if "$INSTALL_DIR/sigardapanel" init --user "$ADMIN_USER" --email "$ADMIN_EMAIL" 2>/dev/null; then
    log_ok "Database initialized"
else
    log_warn "Database init skipped (may already exist)"
fi

# ── Step 7: Create systemd service ─────────────────────────
log_info "[7/8] Creating systemd service..."
cat > "/etc/systemd/system/$SERVICE_NAME.service" <<EOF
[Unit]
Description=SigardaPanel Panel
After=network.target nginx.service
Wants=nginx.service

[Service]
Type=simple
User=sigardapanel
Group=sigardapanel
EnvironmentFile=$CONFIG_DIR/panel.env
ExecStart=$INSTALL_DIR/sigardapanel api
Restart=always
RestartSec=5
LimitNOFILE=65536
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=$DATA_DIR $LOG_DIR $BACKUP_DIR

[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload
systemctl enable "$SERVICE_NAME" > /dev/null 2>&1
log_ok "Systemd service created"

# ── Step 8: Configure nginx ────────────────────────────────
log_info "[8/8] Configuring nginx..."

# Backup existing config
if [ -f "$NGINX_CONF" ]; then
    cp "$NGINX_CONF" "${NGINX_CONF}.bak.$(date +%Y%m%d%H%M%S)"
    log_info "Backed up existing nginx config"
fi

cat > "$NGINX_CONF" <<EOF
server {
    listen 80;
    server_name $PANEL_DOMAIN;

    # Security headers
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;

    # Main proxy
    location / {
        proxy_pass http://127.0.0.1:7700;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_read_timeout 300s;
        proxy_connect_timeout 60s;
        proxy_send_timeout 300s;
        proxy_buffer_size 16k;
        proxy_buffers 4 32k;
    }

    # WebSocket terminal proxy
    location /api/ws/ {
        proxy_pass http://127.0.0.1:7710/ws/;
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
    }

    # Deny access to hidden files
    location ~ /\. {
        deny all;
        access_log off;
        log_not_found off;
    }
}
EOF

# Enable site
if [ -d /etc/nginx/sites-enabled ]; then
    ln -sf "$NGINX_CONF" /etc/nginx/sites-enabled/sigardapanel
    rm -f /etc/nginx/sites-enabled/default 2>/dev/null || true
elif [ -d /etc/nginx/conf.d ]; then
    cp "$NGINX_CONF" /etc/nginx/conf.d/sigardapanel.conf
fi

# Test nginx config
if nginx -t 2>/dev/null; then
    log_ok "Nginx configuration valid"
else
    log_error "Nginx configuration test failed"
    echo "  Check: nginx -t"
    exit 1
fi

# ── SSL Setup ──────────────────────────────────────────────
if command -v certbot > /dev/null 2>&1; then
    log_info "Attempting Let's Encrypt SSL for $PANEL_DOMAIN..."
    if certbot --nginx -d "$PANEL_DOMAIN" --email "$ADMIN_EMAIL" \
        --agree-tos --no-eff-email --non-interactive 2>/dev/null; then
        log_ok "SSL certificate installed"
    else
        log_warn "SSL setup failed (you can retry manually with certbot)"
    fi
fi

# ── Start services ─────────────────────────────────────────
log_info "Starting services..."
systemctl start "$SERVICE_NAME" 2>/dev/null || true
systemctl reload nginx 2>/dev/null || true
sleep 2

# ── Verify ─────────────────────────────────────────────────
echo ""
if systemctl is-active --quiet "$SERVICE_NAME"; then
    echo -e "${GREEN}╔══════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║       Installation Complete!                     ║${NC}"
    echo -e "${GREEN}╚══════════════════════════════════════════════════╝${NC}"
    echo ""
    echo -e "  Panel URL:  ${GREEN}https://$PANEL_DOMAIN${NC}"
    echo -e "  Admin User: ${BLUE}$ADMIN_USER${NC}"
    echo ""
    echo -e "  ${YELLOW}Save this agent token for server registration:${NC}"
    echo -e "  ${BLUE}$AGENT_TOKEN${NC}"
    echo ""
    echo "  Service:  systemctl status $SERVICE_NAME"
    echo "  Logs:     journalctl -u $SERVICE_NAME -f"
    echo "  Config:   $CONFIG_DIR/panel.env"
    echo ""
else
    echo -e "${RED}╔══════════════════════════════════════════════════╗${NC}"
    echo -e "${RED}║       Installation Failed                       ║${NC}"
    echo -e "${RED}╚══════════════════════════════════════════════════╝${NC}"
    echo ""
    echo "  Check logs: journalctl -u $SERVICE_NAME -e"
    echo "  Check config: cat $CONFIG_DIR/panel.env"
    echo ""
    exit 1
fi
