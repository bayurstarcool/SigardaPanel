#!/usr/bin/env bash
# SigardaPanel v1.0.0 — One-shot Installer
# Tested on: Ubuntu 24.04 LTS (fresh install)
# Result:   fully working panel on http://SERVER_IP in ~5 min
set -euo pipefail

VERSION="1.0.0"
RED='\033[0;31m'; GREEN='\033[0;32m'; CYAN='\033[0;36m'; YELLOW='\033[1;33m'; NC='\033[0m'
log() { echo -e "${CYAN}[sigarda]${NC} $1"; }
ok()  { echo -e "${GREEN}[  OK  ]${NC} $1"; }
warn(){ echo -e "${YELLOW}[WARN ]${NC} $1"; }
fail(){ echo -e "${RED}[FAIL]${NC} $1"; exit 1; }

# ── Flags ──
DOMAIN="${DOMAIN:-}"              # optional FQDN for HTTPS
CF_TOKEN="${CF_TOKEN:-}"          # optional Cloudflare API token
ADMIN_USER="${ADMIN_USER:-admin}"
ADMIN_EMAIL="${ADMIN_EMAIL:-admin@localhost}"
ADMIN_PASS="${ADMIN_PASS:-$(openssl rand -base64 18 | tr -d '=+/' | cut -c1-16)}"
INSTALL_NGINX="${INSTALL_NGINX:-yes}"
INSTALL_FIREWALL="${INSTALL_FIREWALL:-yes}"
SOURCE_DIR="${SOURCE_DIR:-/opt/sigardapanel/source}"

# ── Prerequisites ──
[ "$EUID" -eq 0 ] || fail "Must run as root"
. /etc/os-release
[ "$ID" = "ubuntu" ] && [ "${VERSION_ID%.*}" = "24.04" ] || fail "Requires Ubuntu 24.04 (got $ID $VERSION_ID)"

log "SigardaPanel v$VERSION installer"
log "Server: $(hostname)  |  IP: $(hostname -I | awk '{print $1}')"

# ── 1. System dependencies ──
log "[1/9] Installing system dependencies..."
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq --no-install-recommends \
  curl git nginx nodejs npm certbot python3-certbot-dns-cloudflare \
  mariadb-server redis-server docker.io docker-compose-v2 \
  ufw fail2ban openssl ca-certificates 2>&1 | tail -1
ok "System packages installed"

# ── 2. Go toolchain ──
log "[2/9] Ensuring Go 1.25+..."
if ! command -v go &>/dev/null || ! go version | grep -qE 'go1\.(2[5-9]|[3-9][0-9])'; then
  log "Installing Go 1.25..."
  curl -sL https://go.dev/dl/go1.25.0.linux-amd64.tar.gz -o /tmp/go.tar.gz
  rm -rf /usr/local/go && tar -C /usr/local -xzf /tmp/go.tar.gz
  echo 'export PATH=$PATH:/usr/local/go/bin' > /etc/profile.d/go.sh
  chmod +x /etc/profile.d/go.sh
  export PATH=$PATH:/usr/local/go/bin
fi
go version
ok "Go $(go version | awk '{print $3}')"

# ── 3. Source code ──
log "[3/9] Checking source code..."
if [ ! -d "$SOURCE_DIR" ]; then
  fail "Source not found at $SOURCE_DIR. Clone the repo first:
    git clone https://github.com/yourorg/sigardapanel.git $SOURCE_DIR"
fi
[ -f "$SOURCE_DIR/go.mod" ] || fail "Invalid source: $SOURCE_DIR"
ok "Source at $SOURCE_DIR"

# ── 4. Build Go binary ──
log "[4/9] Building SigardaPanel binary..."
cd "$SOURCE_DIR"
go build -trimpath -ldflags "-s -w" -o /usr/local/bin/sigardapanel ./cmd/sigardapanel
chmod 755 /usr/local/bin/sigardapanel
ok "Binary: $(/usr/local/bin/sigardapanel version)"

# ── 5. Build frontend ──
log "[5/9] Building frontend..."
cd "$SOURCE_DIR/web"
[ -d node_modules ] || npm ci --silent 2>/dev/null || npm install --silent
ORIGIN_URL="https://${DOMAIN:-127.0.0.1}"
export ORIGIN=$ORIGIN_URL
ORIGIN=$ORIGIN_URL npm run build 2>&1 | tail -1
ok "Frontend built (35s)"

# ── 6. Deploy user + directories ──
log "[6/9] Creating user & directories..."
id -u sigardapanel &>/dev/null || useradd -r -s /usr/sbin/nologin -d /opt/sigardapanel sigardapanel
mkdir -p /opt/sigardapanel/{web,backups,source}
mkdir -p /var/lib/sigardapanel /var/log/sigardapanel /etc/sigardapanel

# Frontend deploy
rm -rf /opt/sigardapanel/web/*
cp -r "$SOURCE_DIR/web/build/"* /opt/sigardapanel/web/
echo '{"type":"module"}' > /opt/sigardapanel/web/package.json

# Permissions
chown -R sigardapanel:sigardapanel /opt/sigardapanel /var/lib/sigardapanel /var/log/sigardapanel
ok "Layout deployed"

# ── 7. Generate secrets & env files ──
log "[7/9] Generating secrets..."
AGENT_TOKEN=$(openssl rand -hex 32)
ENCRYPTION_KEY=$(openssl rand -hex 32)
SERVER_IP=$(curl -s https://api.ipify.org 2>/dev/null || hostname -I | awk '{print $1}')
PUBLIC_URL="${DOMAIN:+https://$DOMAIN}"
PUBLIC_URL="${PUBLIC_URL:-http://$SERVER_IP}"

cat > /etc/sigardapanel/agent.env <<EOF
SIGARDAPANEL_AGENT_ADDR=:7710
SIGARDAPANEL_PANEL_URL=http://127.0.0.1:7700
SIGARDAPANEL_AGENT_TOKEN=$AGENT_TOKEN
SIGARDAPANEL_AGENT_SITE_ROOT=/var/www
EOF

cat > /etc/sigardapanel/panel.env <<EOF
SIGARDAPANEL_API_ADDR=127.0.0.1:7700
SIGARDAPANEL_PUBLIC_URL=$PUBLIC_URL
SIGARDAPANEL_DB_PATH=/var/lib/sigardapanel/sigardapanel.db
SIGARDAPANEL_LOG_DIR=/var/log/sigardapanel
SIGARDAPANEL_AGENT_URL=http://127.0.0.1:7710
SIGARDAPANEL_AGENT_TOKEN=$AGENT_TOKEN
SIGARDAPANEL_ENCRYPTION_KEY=$ENCRYPTION_KEY
MEMBER_DB_HOST=127.0.0.1
MEMBER_DB_PORT=3306
MEMBER_DB_USER=sigardapanel
MEMBER_DB_PASSWORD=$(openssl rand -hex 16)
MEMBER_DB_NAME=sigardapanel_member
EOF
chmod 600 /etc/sigardapanel/*.env
ok "Env files (mode 600) created"

# Save generated passwords for the summary
echo "$ADMIN_PASS" > /tmp/.sigarda_admin_pass
echo "$AGENT_TOKEN" > /tmp/.sigarda_agent_token
echo "$ENCRYPTION_KEY" > /tmp/.sigarda_enc_key
chmod 600 /tmp/.sigarda_*

# ── 8. Systemd units ──
log "[8/9] Installing systemd units..."

# API
cat > /etc/systemd/system/sigardapanel-api.service <<'UNIT'
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
UNIT

# Agent
cat > /etc/systemd/system/sigardapanel-agent.service <<'UNIT'
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
UNIT

# Web
cat > /etc/systemd/system/sigardapanel-web.service <<UNIT
[Unit]
Description=SigardaPanel Web
After=sigardapanel-api.service
Requires=sigardapanel-api.service
[Service]
Type=simple
User=sigardapanel
Group=sigardapanel
WorkingDirectory=/opt/sigardapanel/web
Environment=NODE_ENV=production
Environment=HOST=127.0.0.1
Environment=PORT=7720
Environment=ORIGIN=$ORIGIN_URL
Environment=SIGARDAPANEL_API_URL=http://127.0.0.1:7700
ExecStart=/usr/bin/node /opt/sigardapanel/web/index.js
Restart=on-failure
RestartSec=3
LimitNOFILE=65536
[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
ok "Systemd units installed"

# ── 9. Initialize DB + start services ──
log "[9/9] Initializing database & services..."

# Non-interactive init: create admin + DB schema
cd /opt/sigardapanel
/usr/local/bin/sigardapanel init \
  --email "$ADMIN_EMAIL" \
  --password "$ADMIN_PASS" 2>&1 | tail -3

# Enable + start
for s in sigardapanel-api sigardapanel-agent sigardapanel-web; do
  systemctl enable --now $s 2>&1
  sleep 2
  if ! systemctl is-active --quiet $s; then
    fail "Service $s failed. Check: journalctl -u $s -n 30"
  fi
done
ok "All 3 services running"

# ── 10. Nginx (optional) ──
if [ "$INSTALL_NGINX" = "yes" ]; then
  log "Configuring nginx..."

  # Default site → redirect to panel
  cat > /etc/nginx/sites-enabled/default <<'NGINX'
server { listen 80 default_server; server_name _; return 404; }
NGINX

  # Panel site
  cat > /etc/nginx/sites-enabled/sigardapanel <<'NGINX'
# HTTP — HTTPS redirect for direct, proxy for CF (anti-loop)
server {
    listen 80 default_server;
    server_name _;
    client_max_body_size 100M;
    if ($http_cf_ray = "") { return 301 https://$host$request_uri; }
    location / { proxy_pass http://127.0.0.1:7720; proxy_set_header Host $host; proxy_set_header X-Forwarded-Proto $scheme; proxy_buffers 8 16k; proxy_buffer_size 16k; proxy_read_timeout 900s; }
    location ^~ /api/v1/ { proxy_pass http://127.0.0.1:7700; proxy_http_version 1.1; proxy_set_header Host $host; proxy_set_header X-Real-IP $remote_addr; proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for; proxy_set_header X-Forwarded-Proto $scheme; proxy_set_header Upgrade $http_upgrade; proxy_set_header Connection "upgrade"; proxy_buffers 8 16k; proxy_buffer_size 16k; proxy_read_timeout 900s; }
    location /api/ { proxy_pass http://127.0.0.1:7720; proxy_http_version 1.1; proxy_set_header Host $host; proxy_set_header X-Forwarded-Proto $scheme; proxy_buffers 8 16k; proxy_buffer_size 16k; proxy_read_timeout 900s; }
}

# HTTPS — self-signed by default; replace with Let's Encrypt
server {
    listen 443 ssl http2 default_server;
    server_name _;
    ssl_certificate /etc/sigardapanel/selfsigned.crt;
    ssl_certificate_key /etc/sigardapanel/selfsigned.key;
    ssl_protocols TLSv1.2 TLSv1.3;
    client_max_body_size 100M;
    add_header X-Frame-Options "DENY" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header Strict-Transport-Security "max-age=31536000" always;
    location / { proxy_pass http://127.0.0.1:7720; proxy_set_header Host $host; proxy_set_header X-Forwarded-Proto $scheme; proxy_buffers 8 16k; proxy_buffer_size 16k; proxy_read_timeout 900s; }
    location ^~ /api/v1/ { proxy_pass http://127.0.0.1:7700; proxy_http_version 1.1; proxy_set_header Host $host; proxy_set_header X-Real-IP $remote_addr; proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for; proxy_set_header X-Forwarded-Proto $scheme; proxy_set_header Upgrade $http_upgrade; proxy_set_header Connection "upgrade"; proxy_buffers 8 16k; proxy_buffer_size 16k; proxy_read_timeout 900s; }
    location /api/ { proxy_pass http://127.0.0.1:7720; proxy_http_version 1.1; proxy_set_header Host $host; proxy_set_header X-Forwarded-Proto $scheme; proxy_buffers 8 16k; proxy_buffer_size 16k; proxy_read_timeout 900s; }
}
NGINX

  # Generate self-signed cert (fallback if no Let's Encrypt)
  if [ ! -f /etc/sigardapanel/selfsigned.crt ]; then
    openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
      -keyout /etc/sigardapanel/selfsigned.key \
      -out /etc/sigardapanel/selfsigned.crt \
      -subj "/CN=$SERVER_IP" 2>/dev/null
    chmod 600 /etc/sigardapanel/selfsigned.key
  fi

  # Let's Encrypt if domain provided
  if [ -n "$DOMAIN" ] && [ -n "$CF_TOKEN" ]; then
    log "  Issuing Let's Encrypt cert for $DOMAIN..."
    cat > /etc/letsencrypt/cloudflare.ini <<EOF
dns_cloudflare_api_token = $CF_TOKEN
EOF
    chmod 600 /etc/letsencrypt/cloudflare.ini
    if certbot certonly --dns-cloudflare --dns-cloudflare-credentials /etc/letsencrypt/cloudflare.ini \
      --dns-cloudflare-propagation-seconds 20 \
      -d "$DOMAIN" --agree-tos -m admin@$DOMAIN --non-interactive 2>&1 | tail -3; then
      sed -i "s|ssl_certificate /etc/sigardapanel/selfsigned.crt;|ssl_certificate /etc/letsencrypt/live/$DOMAIN/fullchain.pem;|" /etc/nginx/sites-enabled/sigardapanel
      sed -i "s|ssl_certificate_key /etc/sigardapanel/selfsigned.key;|ssl_certificate_key /etc/letsencrypt/live/$DOMAIN/privkey.pem;|" /etc/nginx/sites-enabled/sigardapanel
      ok "  Let's Encrypt cert issued"
    else
      warn "  Let's Encrypt failed, using self-signed"
    fi
  fi

  nginx -t && systemctl reload nginx
  ok "Nginx configured"
fi

# ── 11. Firewall ──
if [ "$INSTALL_FIREWALL" = "yes" ]; then
  log "Configuring firewall..."
  ufw --force reset 2>/dev/null
  ufw default deny incoming
  ufw allow 22/tcp comment 'SSH'
  ufw allow 80/tcp comment 'HTTP'
  ufw allow 443/tcp comment 'HTTPS'
  ufw --force enable 2>/dev/null
  ok "UFW active (22/80/443)"
fi

# ── 12. Verify ──
log "Verifying install..."
sleep 3
HEALTH=$(curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:7700/api/v1/health)
[ "$HEALTH" = "200" ] || fail "API health check failed: HTTP $HEALTH"
ok "API health 200"

SERVICES=$(systemctl list-units --type=service --state=active --no-legend | grep -c sigardapanel)
[ "$SERVICES" -ge 3 ] || fail "Only $SERVICES sigardapanel services running"
ok "Services: $SERVICES/3 active"

# ── Summary ──
echo ""
echo "╔══════════════════════════════════════════════════╗"
echo "║       SigardaPanel v$VERSION  READY              ║"
echo "╚══════════════════════════════════════════════════╝"
echo ""
echo "  URL:      $PUBLIC_URL"
[ -n "$DOMAIN" ] && echo "  HTTPS:    self-signed or Let's Encrypt (see /etc/nginx/sites-enabled/)"
echo "  Admin:    $ADMIN_USER / $ADMIN_PASS"
echo ""
echo "  Services:  sigardapanel-api, -agent, -web (all running)"
echo "  Binary:    /usr/local/bin/sigardapanel"
echo "  Config:    /etc/sigardapanel/"
echo "  DB:        /var/lib/sigardapanel/sigardapanel.db"
echo "  Logs:      journalctl -u sigardapanel-{api,agent,web}"
echo ""
echo "  Quick commands:"
echo "    sigardapanel doctor --api-url http://127.0.0.1:7700"
echo "    sigardapanel user list --direct"
echo "    sigardapanel server token --id 1"
echo ""
warn "⚠  Change admin password immediately!"
warn "   sigardapanel user reset-password $ADMIN_USER --direct"
echo ""
