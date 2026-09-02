#!/usr/bin/env bash
set -Eeuo pipefail
umask 027

ADMIN_USER="${SIGARDAPANEL_ADMIN_USER:-admin}"
ADMIN_EMAIL="${SIGARDAPANEL_ADMIN_EMAIL:-admin@localhost}"
INSTALL_DIR="${SIGARDAPANEL_INSTALL_DIR:-/usr/local/bin}"
CONFIG_DIR="${SIGARDAPANEL_CONFIG_DIR:-/etc/sigardapanel}"
DATA_DIR="${SIGARDAPANEL_DATA_DIR:-/var/lib/sigardapanel}"
WEB_DIR="${SIGARDAPANEL_WEB_DIR:-/opt/sigardapanel/web}"
LOG_DIR="${SIGARDAPANEL_LOG_DIR:-/var/log/sigardapanel}"
BACKUP_DIR="${SIGARDAPANEL_BACKUP_DIR:-/var/backups/sigardapanel}"
PANEL_DOMAIN="${PANEL_DOMAIN:-}"
AGENT_URL="${SIGARDAPANEL_AGENT_URL:-http://127.0.0.1:7710}"
AGENT_TOKEN="${SIGARDAPANEL_AGENT_TOKEN:-}"
ADMIN_PASSWORD="${SIGARDAPANEL_ADMIN_PASSWORD:-}"
API_SERVICE="sigardapanel-api"
AGENT_SERVICE="sigardapanel-agent"
WEB_SERVICE="sigardapanel-web"
RELEASE_VERSION="${SIGARDAPANEL_RELEASE_VERSION:-v0.5.5}"
BINARY_URL="${SIGARDAPANEL_BINARY_URL:-https://github.com/bayurstarcool/SigardaPanel/releases/download/$RELEASE_VERSION/sigardapanel}"
WEB_ARCHIVE_URL="${SIGARDAPANEL_WEB_ARCHIVE_URL:-https://github.com/bayurstarcool/SigardaPanel/releases/download/$RELEASE_VERSION/sigardapanel-web.tar.gz}"
BINARY_SHA256="${SIGARDAPANEL_BINARY_SHA256:-a4e0f9b10671cd465d1e66248b6512557dc497bf7b5d5d6aecdb6739cf7bf198}"
WEB_ARCHIVE_SHA256="${SIGARDAPANEL_WEB_ARCHIVE_SHA256:-69b5da003395f64d0fd95493b205d2fde39d86edbdbaa4f25a09951cc4549235}"
DASHBOARD_URL="${SIGARDAPANEL_DASHBOARD_URL:-https://github.com/bayurstarcool/SigardaPanel/releases/download/$RELEASE_VERSION/sigardapanel-info.sh}"
DASHBOARD_SHA256="${SIGARDAPANEL_DASHBOARD_SHA256:-9d4650c2d78fa71bbc19fdd1001717d599f6fd7badbac6bef662ca8c6490d58c}"
NGINX_CONF="/etc/nginx/sites-available/sigardapanel"

usage() {
    cat <<'USAGE'
SigardaPanel production panel installer

Usage:
  PANEL_DOMAIN=panel.example.com ./install-panel.sh [options]

Options:
  --panel-domain DOMAIN       Public panel domain
  --install-dir DIR           Binary directory (default: /usr/local/bin)
  --binary-url URL            Binary release URL
  --web-archive-url URL       SvelteKit adapter-node tar.gz URL
  -h, --help                  Show this help

Release contract: the web archive must contain index.js, client/, and server/.
USAGE
}

while (($#)); do
    case "$1" in
        --panel-domain) PANEL_DOMAIN=${2:?missing domain}; shift 2 ;;
        --install-dir) INSTALL_DIR=${2:?missing directory}; shift 2 ;;
        --binary-url) BINARY_URL=${2:?missing URL}; shift 2 ;;
        --web-archive-url) WEB_ARCHIVE_URL=${2:?missing URL}; shift 2 ;;
        -h|--help) usage; exit 0 ;;
        *) printf 'Unknown argument: %s\n' "$1" >&2; usage >&2; exit 2 ;;
    esac
done

[[ $(id -u) -eq 0 ]] || { echo 'Error: must run as root' >&2; exit 1; }
[[ "$PANEL_DOMAIN" =~ ^[A-Za-z0-9.-]+$ && "$PANEL_DOMAIN" != .* && "$PANEL_DOMAIN" != *. ]] || {
    echo 'Error: PANEL_DOMAIN must be a valid hostname' >&2; exit 1;
}
for path_value in "$INSTALL_DIR" "$CONFIG_DIR" "$DATA_DIR" "$WEB_DIR" "$LOG_DIR" "$BACKUP_DIR"; do
    [[ "$path_value" =~ ^/[A-Za-z0-9._/-]+$ ]] || { echo "Error: unsafe install path: $path_value" >&2; exit 1; }
done

source /etc/os-release 2>/dev/null || { echo 'Error: cannot detect OS' >&2; exit 1; }
case "${ID:-}" in
    ubuntu|debian)
        apt-get update -qq
        DEBIAN_FRONTEND=noninteractive apt-get install -y -qq nginx sqlite3 curl tar nodejs openssl ca-certificates certbot python3-certbot-nginx
        ;;
    centos|rhel|fedora|rocky|almalinux)
        dnf install -y -q nginx sqlite curl tar nodejs openssl ca-certificates certbot python3-certbot-nginx
        ;;
    *) echo "Error: unsupported OS: ${ID:-unknown}" >&2; exit 1 ;;
esac

printf '[1/8] Installing release artifacts...\n'
install -d -m 0755 "$INSTALL_DIR"
tmp_binary=$(mktemp); tmp_web=$(mktemp); tmp_dashboard=$(mktemp)
rollback_dir=$(mktemp -d)
rollback_enabled=1
[ ! -f "$INSTALL_DIR/sigardapanel" ] || cp -a "$INSTALL_DIR/sigardapanel" "$rollback_dir/binary"
[ ! -d "$WEB_DIR" ] || cp -a "$WEB_DIR" "$rollback_dir/web"
[ ! -d "$CONFIG_DIR" ] || cp -a "$CONFIG_DIR" "$rollback_dir/config"
[ ! -f "/etc/systemd/system/$API_SERVICE.service" ] || cp -a "/etc/systemd/system/$API_SERVICE.service" "$rollback_dir/api.service"
[ ! -f "/etc/systemd/system/$AGENT_SERVICE.service" ] || cp -a "/etc/systemd/system/$AGENT_SERVICE.service" "$rollback_dir/agent.service"
[ ! -f "/etc/systemd/system/$WEB_SERVICE.service" ] || cp -a "/etc/systemd/system/$WEB_SERVICE.service" "$rollback_dir/web.service"
[ ! -f "$NGINX_CONF" ] || cp -a "$NGINX_CONF" "$rollback_dir/nginx.conf"
[ ! -e /etc/nginx/sites-enabled/sigardapanel ] || cp -aP /etc/nginx/sites-enabled/sigardapanel "$rollback_dir/nginx-enabled"
[ ! -e /etc/nginx/sites-enabled/default ] || cp -aP /etc/nginx/sites-enabled/default "$rollback_dir/nginx-default"
[ ! -f /etc/profile.d/sigardapanel-info.sh ] || cp -a /etc/profile.d/sigardapanel-info.sh "$rollback_dir/dashboard"
if [ -L /usr/local/bin/sigardapanel-info ]; then readlink /usr/local/bin/sigardapanel-info > "$rollback_dir/dashboard-link"; elif [ -e /usr/local/bin/sigardapanel-info ]; then cp -a /usr/local/bin/sigardapanel-info "$rollback_dir/dashboard-command"; fi
cleanup_install() {
    local rc=$?
    rm -f "$tmp_binary" "$tmp_web" "$tmp_dashboard" "$INSTALL_DIR/.sigardapanel.new"
    if (( rc != 0 && rollback_enabled )); then
        if [[ -f "$rollback_dir/binary" ]]; then cp -a "$rollback_dir/binary" "$INSTALL_DIR/sigardapanel"; else rm -f "$INSTALL_DIR/sigardapanel"; fi
        rm -rf "$WEB_DIR"; [[ ! -d "$rollback_dir/web" ]] || cp -a "$rollback_dir/web" "$WEB_DIR"
        rm -rf "$CONFIG_DIR"; [[ ! -d "$rollback_dir/config" ]] || cp -a "$rollback_dir/config" "$CONFIG_DIR"
        for spec in "$API_SERVICE:api.service" "$AGENT_SERVICE:agent.service" "$WEB_SERVICE:web.service"; do
            svc=${spec%%:*}; snap=${spec#*:}; target="/etc/systemd/system/$svc.service"
            if [[ -f "$rollback_dir/$snap" ]]; then cp -a "$rollback_dir/$snap" "$target"; else rm -f "$target"; fi
        done
        if [[ -f "$rollback_dir/nginx.conf" ]]; then cp -a "$rollback_dir/nginx.conf" "$NGINX_CONF"; else rm -f "$NGINX_CONF"; fi
        rm -f /etc/nginx/sites-enabled/sigardapanel /etc/nginx/sites-enabled/default
        [[ ! -e "$rollback_dir/nginx-enabled" ]] || cp -aP "$rollback_dir/nginx-enabled" /etc/nginx/sites-enabled/sigardapanel
        [[ ! -e "$rollback_dir/nginx-default" ]] || cp -aP "$rollback_dir/nginx-default" /etc/nginx/sites-enabled/default
        if [[ -f "$rollback_dir/dashboard" ]]; then cp -a "$rollback_dir/dashboard" /etc/profile.d/sigardapanel-info.sh; else rm -f /etc/profile.d/sigardapanel-info.sh /usr/local/bin/sigardapanel-info; fi
        rm -f /usr/local/bin/sigardapanel-info
        if [[ -f "$rollback_dir/dashboard-link" ]]; then ln -s "$(cat "$rollback_dir/dashboard-link")" /usr/local/bin/sigardapanel-info; elif [[ -f "$rollback_dir/dashboard-command" ]]; then cp -a "$rollback_dir/dashboard-command" /usr/local/bin/sigardapanel-info; fi
        systemctl daemon-reload 2>/dev/null || true
        systemctl restart "$AGENT_SERVICE" "$API_SERVICE" "$WEB_SERVICE" nginx 2>/dev/null || true
    fi
    rm -rf "$rollback_dir" "$INSTALL_DIR/.sigardapanel.previous" "$WEB_DIR.old" "$WEB_DIR.new"
    exit "$rc"
}
trap cleanup_install EXIT
curl --proto '=https' --proto-redir '=https' --tlsv1.2 -fsSL --retry 3 "$BINARY_URL" -o "$tmp_binary"
[[ -s "$tmp_binary" ]] || { echo 'Error: downloaded binary is empty' >&2; exit 1; }
printf '%s  %s\n' "$BINARY_SHA256" "$tmp_binary" | sha256sum -c -
install -o root -g root -m 0755 "$tmp_binary" "$INSTALL_DIR/.sigardapanel.new"
"$INSTALL_DIR/.sigardapanel.new" version >/dev/null
if [[ -f "$INSTALL_DIR/sigardapanel" ]]; then
    cp -a "$INSTALL_DIR/sigardapanel" "$INSTALL_DIR/.sigardapanel.previous"
fi
mv -f "$INSTALL_DIR/.sigardapanel.new" "$INSTALL_DIR/sigardapanel"

curl --proto '=https' --proto-redir '=https' --tlsv1.2 -fsSL --retry 3 "$WEB_ARCHIVE_URL" -o "$tmp_web"
printf '%s  %s\n' "$WEB_ARCHIVE_SHA256" "$tmp_web" | sha256sum -c -
tar -tzf "$tmp_web" >/dev/null
tar -tzf "$tmp_web" | grep -qx './index.js'
tar -tzf "$tmp_web" | grep -q '^./client/'
tar -tzf "$tmp_web" | grep -q '^./server/'
if tar -tzf "$tmp_web" | grep -Eq '(^/|(^|/)\.\.(/|$))'; then
    echo 'Error: unsafe path in web archive' >&2; exit 1
fi
rm -rf "$WEB_DIR.new"
install -d -m 0755 "$WEB_DIR.new"
tar -xzf "$tmp_web" -C "$WEB_DIR.new" --no-same-owner --no-same-permissions
[[ -f "$WEB_DIR.new/index.js" ]] || { echo 'Error: web archive does not contain index.js at its root' >&2; exit 1; }
rm -rf "$WEB_DIR.old"
[[ ! -d "$WEB_DIR" ]] || mv "$WEB_DIR" "$WEB_DIR.old"
mv "$WEB_DIR.new" "$WEB_DIR"

printf '[2/8] Creating service account and protected directories...\n'
id -u sigardapanel >/dev/null 2>&1 || useradd --system --home-dir "$DATA_DIR" --shell /usr/sbin/nologin sigardapanel
install -d -o sigardapanel -g sigardapanel -m 0750 "$DATA_DIR" "$LOG_DIR" "$BACKUP_DIR"
install -d -o root -g sigardapanel -m 0750 "$CONFIG_DIR"
chown -R root:root "$WEB_DIR"
chmod -R u=rwX,go=rX "$WEB_DIR"

printf '[3/8] Writing secure configuration...\n'
if [[ -f "$CONFIG_DIR/panel.env" ]]; then
    existing_agent_token=$(sed -n 's/^SIGARDAPANEL_AGENT_TOKEN=//p' "$CONFIG_DIR/panel.env" | head -n1)
    existing_encryption_key=$(sed -n 's/^SIGARDAPANEL_ENCRYPTION_KEY=//p' "$CONFIG_DIR/panel.env" | head -n1)
fi
[[ -n "$AGENT_TOKEN" ]] || AGENT_TOKEN=${existing_agent_token:-$(openssl rand -hex 32)}
ENCRYPTION_KEY=${existing_encryption_key:-$(openssl rand -hex 32)}
cat > "$CONFIG_DIR/panel.env" <<EOF
SIGARDAPANEL_API_ADDR=127.0.0.1:7700
SIGARDAPANEL_PUBLIC_URL=https://$PANEL_DOMAIN
SIGARDAPANEL_DB_PATH=$DATA_DIR/sigardapanel.db
SIGARDAPANEL_LOG_DIR=$LOG_DIR
SIGARDAPANEL_AGENT_URL=$AGENT_URL
SIGARDAPANEL_AGENT_TOKEN=$AGENT_TOKEN
SIGARDAPANEL_ENCRYPTION_KEY=$ENCRYPTION_KEY
EOF
chown root:sigardapanel "$CONFIG_DIR/panel.env"
chmod 0640 "$CONFIG_DIR/panel.env"
cat > "$CONFIG_DIR/agent.env" <<EOF
SIGARDAPANEL_AGENT_ADDR=127.0.0.1:7710
SIGARDAPANEL_PANEL_URL=http://127.0.0.1:7700
SIGARDAPANEL_AGENT_TOKEN=$AGENT_TOKEN
SIGARDAPANEL_AGENT_SITE_ROOT=/home
SIGARDAPANEL_AGENT_ALLOW_STUB_SSL=false
EOF
chown root:root "$CONFIG_DIR/agent.env"
chmod 0600 "$CONFIG_DIR/agent.env"

printf '[4/8] Initializing database...\n'
export SIGARDAPANEL_API_ADDR=127.0.0.1:7700
export SIGARDAPANEL_DB_PATH="$DATA_DIR/sigardapanel.db"
export SIGARDAPANEL_LOG_DIR="$LOG_DIR"
export SIGARDAPANEL_AGENT_URL="$AGENT_URL"
export SIGARDAPANEL_AGENT_TOKEN="$AGENT_TOKEN"
export SIGARDAPANEL_ENCRYPTION_KEY="$ENCRYPTION_KEY"
if [[ ! -s "$DATA_DIR/sigardapanel.db" ]]; then
    if [[ -t 0 ]]; then
        "$INSTALL_DIR/sigardapanel" init --user "$ADMIN_USER" --email "$ADMIN_EMAIL"
    elif [[ -n "$ADMIN_PASSWORD" ]]; then
        "$INSTALL_DIR/sigardapanel" init --user "$ADMIN_USER" --email "$ADMIN_EMAIL" --password "$ADMIN_PASSWORD"
    else
        echo 'Error: non-interactive install requires SIGARDAPANEL_ADMIN_PASSWORD (minimum 8 characters)' >&2
        exit 1
    fi
fi
chown sigardapanel:sigardapanel "$DATA_DIR/sigardapanel.db"* 2>/dev/null || true

printf '[5/8] Installing systemd services...\n'
cat > "/etc/systemd/system/$API_SERVICE.service" <<EOF
[Unit]
Description=SigardaPanel API
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=sigardapanel
Group=sigardapanel
EnvironmentFile=$CONFIG_DIR/panel.env
ExecStart=$INSTALL_DIR/sigardapanel api
Restart=on-failure
RestartSec=3
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=$DATA_DIR $LOG_DIR $BACKUP_DIR
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictSUIDSGID=true
LockPersonality=true
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF

cat > "/etc/systemd/system/$AGENT_SERVICE.service" <<EOF
[Unit]
Description=SigardaPanel Agent
After=network-online.target nginx.service
Wants=network-online.target nginx.service

[Service]
Type=simple
User=root
EnvironmentFile=$CONFIG_DIR/agent.env
ExecStart=$INSTALL_DIR/sigardapanel agent
Restart=on-failure
RestartSec=3
NoNewPrivileges=true
PrivateTmp=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictSUIDSGID=true
LockPersonality=true
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF

cat > "/etc/systemd/system/$WEB_SERVICE.service" <<EOF
[Unit]
Description=SigardaPanel Web
After=$API_SERVICE.service
Requires=$API_SERVICE.service

[Service]
Type=simple
User=sigardapanel
Group=sigardapanel
WorkingDirectory=$WEB_DIR
Environment=NODE_ENV=production
Environment=HOST=127.0.0.1
Environment=PORT=7720
Environment=ORIGIN=https://$PANEL_DOMAIN
ExecStart=/usr/bin/node $WEB_DIR/index.js
Restart=on-failure
RestartSec=3
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictSUIDSGID=true
LockPersonality=true

[Install]
WantedBy=multi-user.target
EOF

printf '[6/8] Configuring Nginx...\n'
install -d -m 0755 /etc/nginx/sites-available /etc/nginx/sites-enabled
cat > "$NGINX_CONF" <<EOF
server {
    listen 80;
    listen [::]:80;
    server_name $PANEL_DOMAIN;
    client_max_body_size 2g;

    # Keep SvelteKit API routes (/api/auth, /api/proxy, /api/metrics) on Web.
    # Only Go control-plane routes under /api/v1/ go directly to API.
    location ^~ /api/v1/ {
        proxy_pass http://127.0.0.1:7700;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 900s;
    }

    location /api/ {
        proxy_pass http://127.0.0.1:7720;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 900s;
    }

    location / {
        proxy_pass http://127.0.0.1:7720;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 900s;
    }
}
EOF
ln -sfn "$NGINX_CONF" /etc/nginx/sites-enabled/sigardapanel
rm -f /etc/nginx/sites-enabled/default
nginx -t

printf '[7/8] Installing SSH status dashboard...\n'
if [[ -f "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/sigardapanel-info.sh" ]]; then
    install -o root -g root -m 0755 "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/sigardapanel-info.sh" /etc/profile.d/sigardapanel-info.sh
else
    curl --proto '=https' --proto-redir '=https' --tlsv1.2 -fsSL --retry 3 "$DASHBOARD_URL" -o "$tmp_dashboard"
    printf '%s  %s\n' "$DASHBOARD_SHA256" "$tmp_dashboard" | sha256sum -c -
    bash -n "$tmp_dashboard"
    install -o root -g root -m 0755 "$tmp_dashboard" /etc/profile.d/sigardapanel-info.sh
fi
if [[ -f /etc/profile.d/sigardapanel-info.sh ]]; then
    ln -sfn /etc/profile.d/sigardapanel-info.sh /usr/local/bin/sigardapanel-info
fi

printf '[8/8] Starting and verifying services...\n'
systemctl daemon-reload
systemctl enable "$AGENT_SERVICE" "$API_SERVICE" "$WEB_SERVICE" nginx
systemctl restart "$AGENT_SERVICE" "$API_SERVICE" "$WEB_SERVICE" nginx
for _ in {1..20}; do curl -fsS --max-time 2 http://127.0.0.1:7710/health >/dev/null && break; sleep 1; done
curl -fsS --max-time 3 http://127.0.0.1:7710/health >/dev/null || { journalctl -u "$AGENT_SERVICE" -n 50 --no-pager; exit 1; }
for _ in {1..20}; do curl -fsS --max-time 2 http://127.0.0.1:7700/health >/dev/null && break; sleep 1; done
curl -fsS --max-time 3 http://127.0.0.1:7700/health >/dev/null || { journalctl -u "$API_SERVICE" -n 50 --no-pager; exit 1; }

for _ in {1..20}; do curl -fsS --max-time 2 http://127.0.0.1:7720/ >/dev/null && break; sleep 1; done
curl -fsS --max-time 3 http://127.0.0.1:7720/ >/dev/null || { journalctl -u "$WEB_SERVICE" -n 50 --no-pager; exit 1; }

if command -v certbot >/dev/null 2>&1; then
    certbot --nginx -d "$PANEL_DOMAIN" --email "$ADMIN_EMAIL" --agree-tos --no-eff-email --non-interactive --redirect || {
        echo 'Error: SSL issuance failed; rolling back this production installation.' >&2
        exit 1
    }
fi

rollback_enabled=0
rm -rf "$rollback_dir" "$WEB_DIR.old" "$INSTALL_DIR/.sigardapanel.previous"
printf '\nSigardaPanel installation verified.\n  URL: https://%s\n  API: systemctl status %s\n  Agent: systemctl status %s\n  Web: systemctl status %s\n' "$PANEL_DOMAIN" "$API_SERVICE" "$AGENT_SERVICE" "$WEB_SERVICE"
