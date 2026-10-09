#!/usr/bin/env bash
# SigardaPanel installer / updater
#
#   sudo bash install.sh            install the latest release
#   sudo bash install.sh update     update an existing install (backup + automatic rollback)
#   sudo bash install.sh verify     show the release-signing key this script trusts
#
# Every download is checked against a minisign signature (public key below) and
# SHA-256 checksums before anything is installed. There is no option to skip it.
#
# Environment (all optional):
#   SIGARDA_VERSION       release tag to install, e.g. v1.1.0 (default: latest)
#   DOMAIN                public hostname; enables a matching TLS name and ORIGIN
#   LETSENCRYPT_EMAIL     with DOMAIN: request a Let's Encrypt certificate via nginx
#   ADMIN_USER            admin username          (default: admin)
#   ADMIN_EMAIL           admin e-mail            (default: admin@localhost)
#   ADMIN_PASS            admin password          (default: random, printed once)
#   INSTALL_FIREWALL=yes  allow SSH/80/443 in ufw and enable it (never resets rules)
#   SIGARDA_ASSET_BASE    alternative download directory (testing; verification still applies)
set -euo pipefail
umask 022

REPO="bayurstarcool/SigardaPanel"
# minisign public key used to verify releases. Also published in the repository README.
PUBKEY="RWTNXRQ/oh97GPfWH0tDwxbnMTe/WTZeoJdZtGTW3ESF0OpTSlNXpFXI"

INSTALL_BIN="/usr/local/bin/sigardapanel"
WEB_DIR="/opt/sigardapanel/web"
CONF_DIR="/etc/sigardapanel"
DATA_DIR="/var/lib/sigardapanel"
LOG_DIR="/var/log/sigardapanel"
BACKUP_DIR="/var/backups/sigardapanel"
UPDATE_BACKUPS="/var/backups/sigardapanel-updates"

RED='\033[0;31m'; GREEN='\033[0;32m'; CYAN='\033[0;36m'; YELLOW='\033[1;33m'; NC='\033[0m'
log()  { echo -e "${CYAN}[sigarda]${NC} $*"; }
ok()   { echo -e "${GREEN}[  OK  ]${NC} $*"; }
warn() { echo -e "${YELLOW}[ WARN ]${NC} $*"; }
die()  { echo -e "${RED}[ FAIL ]${NC} $*" >&2; exit 1; }

TMP=""
cleanup() { [ -n "$TMP" ] && rm -rf "$TMP"; }
trap cleanup EXIT

require_root() { [ "$(id -u)" -eq 0 ] || die "Run as root (sudo bash install.sh)"; }

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64)  ARCH=amd64 ;;
    aarch64|arm64) ARCH=arm64 ;;
    *) die "Unsupported CPU architecture: $(uname -m)" ;;
  esac
}

check_os() {
  [ -f /etc/os-release ] || die "Cannot detect the operating system"
  . /etc/os-release
  case "${ID:-}" in
    ubuntu|debian) ;;
    *) die "Unsupported OS '${ID:-unknown}'. Supported: Ubuntu 24.04 (tested), Debian 12." ;;
  esac
  command -v apt-get >/dev/null || die "apt-get is required"
}

install_packages() {
  export DEBIAN_FRONTEND=noninteractive
  log "Installing system packages..."
  apt-get update -qq
  apt-get install -y -qq --no-install-recommends \
    curl ca-certificates openssl nginx nodejs sqlite3 minisign tar >/dev/null
  if [ -n "${DOMAIN:-}" ] && [ -n "${LETSENCRYPT_EMAIL:-}" ]; then
    apt-get install -y -qq --no-install-recommends certbot python3-certbot-nginx >/dev/null || warn "certbot install failed"
  fi
  command -v node >/dev/null || die "nodejs did not install"
  local v major minor
  v="$(node -v | sed 's/^v//')"; major="${v%%.*}"; minor="${v#*.}"; minor="${minor%%.*}"
  if [ "$major" -lt 18 ] || { [ "$major" -eq 18 ] && [ "$minor" -lt 13 ]; }; then
    die "Node.js $v is too old (need >= 18.13). Install a newer Node.js and re-run."
  fi
  ok "Packages ready (node $v, minisign $(minisign -v 2>&1 | awk '{print $2}'))"
}

resolve_tag() {
  if [ -n "${SIGARDA_VERSION:-}" ]; then TAG="$SIGARDA_VERSION"; return; fi
  if [ -n "${SIGARDA_ASSET_BASE:-}" ]; then die "Set SIGARDA_VERSION together with SIGARDA_ASSET_BASE"; fi
  local url
  url="$(curl -fsSL --max-time 20 -o /dev/null -w '%{url_effective}' "https://github.com/${REPO}/releases/latest")" \
    || die "Could not reach GitHub to find the latest release"
  TAG="${url##*/}"
  [[ "$TAG" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][A-Za-z0-9.]+)?$ ]] || die "Unexpected release tag '$TAG'"
}

# download_release <dir>: fetch and verify the release assets into <dir>
download_release() {
  local dir="$1" base
  base="${SIGARDA_ASSET_BASE:-https://github.com/${REPO}/releases/download/${TAG}}"
  log "Downloading ${TAG} (linux-${ARCH})..."
  local f
  for f in "sigardapanel-linux-${ARCH}" "sigardapanel-web.tar.gz" "SHA256SUMS" "SHA256SUMS.minisig"; do
    curl -fsSL --max-time 600 --retry 3 -o "${dir}/${f}" "${base}/${f}" || die "Download failed: ${base}/${f}"
  done
  log "Verifying signature and checksums..."
  minisign -Vm "${dir}/SHA256SUMS" -P "$PUBKEY" -x "${dir}/SHA256SUMS.minisig" >/dev/null \
    || die "SIGNATURE VERIFICATION FAILED - refusing to install. The download may have been tampered with."
  ( cd "$dir" && grep -E " (sigardapanel-linux-${ARCH}|sigardapanel-web\.tar\.gz)\$" SHA256SUMS | sha256sum -c --quiet - ) \
    || die "CHECKSUM MISMATCH - refusing to install."
  local n
  n="$(grep -cE " (sigardapanel-linux-${ARCH}|sigardapanel-web\.tar\.gz)\$" "${dir}/SHA256SUMS")"
  [ "$n" -eq 2 ] || die "SHA256SUMS does not list the expected files"
  ok "Release ${TAG} verified (minisign key ${PUBKEY:0:12}...)"
}

unpack_web() { # <tarball> <dest>
  mkdir -p "$2"
  tar -xzf "$1" -C "$2" --no-same-owner --no-same-permissions
  echo '{"type":"module"}' > "$2/package.json"
  [ -f "$2/index.js" ] || die "Web bundle is missing index.js"
}

rand_hex() { openssl rand -hex "$1"; }

server_ip() { curl -fsS --max-time 5 https://api.ipify.org 2>/dev/null || hostname -I | awk '{print $1}'; }

write_env_files() {
  local agent_token enc_key
  agent_token="$(rand_hex 32)"; enc_key="$(rand_hex 32)"
  install -d -m 750 -o root -g root "$CONF_DIR"
  cat > "$CONF_DIR/panel.env" <<EOF
SIGARDAPANEL_API_ADDR=127.0.0.1:7700
SIGARDAPANEL_PUBLIC_URL=${PUBLIC_URL}
SIGARDAPANEL_DB_PATH=${DATA_DIR}/sigardapanel.db
SIGARDAPANEL_LOG_DIR=${LOG_DIR}
SIGARDAPANEL_AGENT_URL=http://127.0.0.1:7710
SIGARDAPANEL_AGENT_TOKEN=${agent_token}
SIGARDAPANEL_ENCRYPTION_KEY=${enc_key}
# Optional member area (PostgreSQL). Leave commented out to keep it disabled.
# MEMBER_DB_HOST=127.0.0.1
# MEMBER_DB_PORT=5432
# MEMBER_DB_USER=sigardapanel
# MEMBER_DB_PASSWORD=
# MEMBER_DB_NAME=sigardapanel_member
EOF
  cat > "$CONF_DIR/agent.env" <<EOF
SIGARDAPANEL_AGENT_ADDR=127.0.0.1:7710
SIGARDAPANEL_AGENT_TOKEN=${agent_token}
EOF
  chmod 600 "$CONF_DIR/panel.env" "$CONF_DIR/agent.env"
}

write_units() {
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
Restart=always
RestartSec=3
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
UNIT

  cat > /etc/systemd/system/sigardapanel-agent.service <<'UNIT'
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
UNIT

  cat > /etc/systemd/system/sigardapanel-web.service <<UNIT
[Unit]
Description=SigardaPanel Web
After=sigardapanel-api.service
Wants=sigardapanel-api.service

[Service]
Type=simple
User=sigardapanel
Group=sigardapanel
WorkingDirectory=${WEB_DIR}
Environment=NODE_ENV=production
Environment=HOST=127.0.0.1
Environment=PORT=7720
Environment=ORIGIN=${ORIGIN}
Environment=SIGARDAPANEL_API_URL=http://127.0.0.1:7700
ExecStart=/usr/bin/node ${WEB_DIR}/index.js
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
UNIT
  systemctl daemon-reload
}

write_nginx() {
  local name="${DOMAIN:-_}" LISTEN6_80="" LISTEN6_443=""
  if [ -e /proc/net/if_inet6 ]; then
    LISTEN6_80="    listen [::]:80 default_server;"
    LISTEN6_443="    listen [::]:443 ssl http2 default_server;"
  fi
  mkdir -p /etc/nginx/conf.d
  cat > /etc/nginx/conf.d/sigardapanel-limits.conf <<'NGINX'
limit_req_zone $binary_remote_addr zone=sigarda_panel:10m rate=15r/s;
limit_req_zone $binary_remote_addr zone=sigarda_login:10m rate=5r/m;
NGINX

  if [ ! -f "$CONF_DIR/selfsigned.crt" ]; then
    local san="IP:${SERVER_IP}"; [ -n "${DOMAIN:-}" ] && san="DNS:${DOMAIN},${san}"
    openssl req -x509 -nodes -newkey rsa:2048 -days 825 \
      -keyout "$CONF_DIR/selfsigned.key" -out "$CONF_DIR/selfsigned.crt" \
      -subj "/CN=${DOMAIN:-$SERVER_IP}" -addext "subjectAltName=${san}" 2>/dev/null
    chmod 600 "$CONF_DIR/selfsigned.key"
  fi

  cat > /etc/nginx/sites-available/sigardapanel <<NGINX
# Managed by the SigardaPanel installer.
# HTTP: everything except ACME challenges is sent to HTTPS.
# (Cloudflare "Flexible" SSL would loop; use "Full (strict)" instead.)
server {
    listen 80 default_server;
${LISTEN6_80}
    server_name ${name};

    location ^~ /.well-known/acme-challenge/ { root /var/www/html; }
    location / { return 301 https://\$host\$request_uri; }
}

server {
    listen 443 ssl http2 default_server;
${LISTEN6_443}
    server_name ${name};

    ssl_certificate     ${CONF_DIR}/selfsigned.crt;
    ssl_certificate_key ${CONF_DIR}/selfsigned.key;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_prefer_server_ciphers on;

    client_max_body_size 100M;
    server_tokens off;

    add_header X-Frame-Options "DENY" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;
    add_header Strict-Transport-Security "max-age=31536000" always;

    location = /api/v1/auth/login {
        limit_req zone=sigarda_login burst=5 nodelay;
        proxy_pass http://127.0.0.1:7700;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-Proto \$scheme;
    }

    # Go API (also carries the terminal WebSocket)
    location ^~ /api/v1/ {
        limit_req zone=sigarda_panel burst=30 nodelay;
        proxy_pass http://127.0.0.1:7700;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 900s;
    }

    # SvelteKit app. X-Real-IP is always overwritten so clients cannot spoof it.
    location / {
        proxy_pass http://127.0.0.1:7720;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_buffers 8 16k;
        proxy_buffer_size 16k;
        proxy_read_timeout 900s;
    }
}
NGINX
  rm -f /etc/nginx/sites-enabled/default
  ln -sfn /etc/nginx/sites-available/sigardapanel /etc/nginx/sites-enabled/sigardapanel
  nginx -t >/dev/null 2>&1 || { nginx -t; die "nginx configuration test failed"; }
  systemctl enable --now nginx >/dev/null 2>&1 || true
  systemctl reload nginx

  if [ -n "${DOMAIN:-}" ] && [ -n "${LETSENCRYPT_EMAIL:-}" ] && command -v certbot >/dev/null; then
    log "Requesting a Let's Encrypt certificate for ${DOMAIN}..."
    if certbot --nginx -d "$DOMAIN" -m "$LETSENCRYPT_EMAIL" --agree-tos -n >/dev/null 2>&1; then
      ok "Let's Encrypt certificate installed"
    else
      warn "Let's Encrypt failed (DNS must point here and port 80 be reachable); keeping the self-signed certificate"
    fi
  fi
}

wait_http() { # <url> <seconds>
  local i
  for i in $(seq 1 "$2"); do
    curl -fsS -o /dev/null --max-time 3 "$1" 2>/dev/null && return 0
    sleep 1
  done
  return 1
}

healthy() { wait_http http://127.0.0.1:7700/api/v1/health 30 && wait_http http://127.0.0.1:7720/login 30; }

do_install() {
  require_root; check_os; detect_arch
  if [ -f "$CONF_DIR/panel.env" ] || [ -x "$INSTALL_BIN" ]; then
    die "SigardaPanel is already installed. To upgrade run: bash install.sh update"
  fi
  install_packages
  resolve_tag
  TMP="$(mktemp -d)"
  download_release "$TMP"

  SERVER_IP="$(server_ip)"
  if [ -n "${DOMAIN:-}" ]; then
    [[ "$DOMAIN" =~ ^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?$ ]] || die "DOMAIN looks invalid"
    ORIGIN="https://${DOMAIN}"
  else
    ORIGIN="https://${SERVER_IP}"
  fi
  PUBLIC_URL="$ORIGIN"
  ADMIN_USER="${ADMIN_USER:-admin}"; ADMIN_EMAIL="${ADMIN_EMAIL:-admin@localhost}"
  [[ "$ADMIN_USER" =~ ^[A-Za-z0-9_.-]{3,32}$ ]] || die "ADMIN_USER must be 3-32 characters of A-Z a-z 0-9 _ . -"
  ADMIN_PASS="${ADMIN_PASS:-$(openssl rand -base64 24 | tr -d '=+/' | cut -c1-20)}"
  [ "${#ADMIN_PASS}" -ge 8 ] || die "ADMIN_PASS must be at least 8 characters"

  log "Installing files..."
  id -u sigardapanel >/dev/null 2>&1 || useradd --system --home-dir /opt/sigardapanel --shell /usr/sbin/nologin sigardapanel
  install -d -m 755 /opt/sigardapanel
  install -d -m 750 -o sigardapanel -g sigardapanel "$DATA_DIR" "$LOG_DIR" "$BACKUP_DIR"
  install -m 755 "$TMP/sigardapanel-linux-${ARCH}" "$INSTALL_BIN"
  unpack_web "$TMP/sigardapanel-web.tar.gz" "$TMP/web"
  rm -rf "$WEB_DIR"; mv "$TMP/web" "$WEB_DIR"
  chown -R root:root "$WEB_DIR"; chmod -R go-w "$WEB_DIR"
  write_env_files
  write_units
  ok "Binary: $("$INSTALL_BIN" version)"

  log "Creating the admin account..."
  runuser -u sigardapanel -- env SIGARDAPANEL_DB_PATH="${DATA_DIR}/sigardapanel.db" SIGARDAPANEL_ADMIN_PASSWORD="$ADMIN_PASS" \
    "$INSTALL_BIN" init --user "$ADMIN_USER" --email "$ADMIN_EMAIL" >/dev/null || die "Could not create the admin user"

  log "Starting services..."
  systemctl enable --now sigardapanel-api sigardapanel-agent sigardapanel-web >/dev/null 2>&1
  write_nginx
  healthy || { journalctl -u sigardapanel-api -u sigardapanel-web -n 30 --no-pager || true; die "Health check failed"; }
  ok "API and web are healthy"

  if [ "${INSTALL_FIREWALL:-no}" = "yes" ]; then
    apt-get install -y -qq ufw >/dev/null
    local ssh_port
    ssh_port="$(sshd -T 2>/dev/null | awk '/^port /{print $2; exit}')"; ssh_port="${ssh_port:-22}"
    ufw allow "${ssh_port}/tcp" >/dev/null; ufw allow 80/tcp >/dev/null; ufw allow 443/tcp >/dev/null
    ufw --force enable >/dev/null
    ok "ufw enabled (SSH ${ssh_port}, 80, 443); existing rules were kept"
  fi

  ( umask 077; printf 'url=%s\nuser=%s\npassword=%s\n' "$PUBLIC_URL" "$ADMIN_USER" "$ADMIN_PASS" > /root/.sigardapanel_admin_cred )

  echo
  echo "=============================================================="
  echo "  SigardaPanel ${TAG} is installed"
  echo "=============================================================="
  echo "  URL:       ${PUBLIC_URL}"
  echo "  Username:  ${ADMIN_USER}"
  echo "  Password:  ${ADMIN_PASS}     (also saved to /root/.sigardapanel_admin_cred)"
  echo
  if [ -z "${DOMAIN:-}" ]; then
    warn "The certificate is self-signed (browser warning). Use DOMAIN=panel.example.com (+ LETSENCRYPT_EMAIL) for a trusted one."
  fi
  if [ "${INSTALL_FIREWALL:-no}" != "yes" ]; then
    warn "No firewall was configured. Recommended: allow only SSH, 80 and 443 (re-run with INSTALL_FIREWALL=yes)."
  fi
  warn "Change the admin password after the first login."
  echo "  Update later:  sudo bash install.sh update"
}

do_update() {
  require_root; check_os; detect_arch
  [ -x "$INSTALL_BIN" ] && [ -f "$CONF_DIR/panel.env" ] || die "SigardaPanel is not installed here. Run: bash install.sh"
  command -v minisign >/dev/null || { apt-get update -qq; apt-get install -y -qq minisign >/dev/null; }
  local current; current="$("$INSTALL_BIN" version 2>/dev/null || echo unknown)"
  resolve_tag
  if [ "${current#v}" = "${TAG#v}" ] && [ -z "${FORCE:-}" ]; then
    ok "Already on ${TAG}. (FORCE=1 to reinstall)"; return 0
  fi
  log "Updating ${current} -> ${TAG}"
  TMP="$(mktemp -d)"
  download_release "$TMP"
  unpack_web "$TMP/sigardapanel-web.tar.gz" "$TMP/web"

  local ts bk; ts="$(date -u +%Y%m%dT%H%M%SZ)"; bk="${UPDATE_BACKUPS}/${ts}"
  install -d -m 700 "$bk"
  cp -a "$INSTALL_BIN" "$bk/sigardapanel.bin"
  cp -a "$WEB_DIR" "$bk/web"
  local dbfile; dbfile="$(grep -E '^SIGARDAPANEL_DB_PATH=' "$CONF_DIR/panel.env" | cut -d= -f2-)"
  if [ -f "$dbfile" ]; then sqlite3 "$dbfile" ".backup '${bk}/sigardapanel.db'"; fi
  ok "Backup saved to ${bk}"

  rollback() {
    warn "Update failed - rolling back"
    install -m 755 "$bk/sigardapanel.bin" "${INSTALL_BIN}.rb" && mv -f "${INSTALL_BIN}.rb" "$INSTALL_BIN"
    rm -rf "$WEB_DIR"; cp -a "$bk/web" "$WEB_DIR"
    if [ -f "$bk/sigardapanel.db" ] && [ -n "$dbfile" ]; then
      cp -a "$bk/sigardapanel.db" "$dbfile"; chown sigardapanel:sigardapanel "$dbfile"; rm -f "${dbfile}-wal" "${dbfile}-shm"
    fi
    systemctl restart sigardapanel-agent sigardapanel-api sigardapanel-web || true
    die "Rolled back to ${current}. Backup kept in ${bk}"
  }

  install -m 755 "$TMP/sigardapanel-linux-${ARCH}" "${INSTALL_BIN}.new" && mv -f "${INSTALL_BIN}.new" "$INSTALL_BIN"
  rm -rf "${WEB_DIR}.new"; cp -a "$TMP/web" "${WEB_DIR}.new"
  chown -R root:root "${WEB_DIR}.new"; chmod -R go-w "${WEB_DIR}.new"
  rm -rf "$WEB_DIR"; mv "${WEB_DIR}.new" "$WEB_DIR"
  systemctl restart sigardapanel-agent; sleep 2
  systemctl restart sigardapanel-api; sleep 2
  systemctl restart sigardapanel-web
  healthy || rollback
  ok "Updated to $("$INSTALL_BIN" version)"
}

case "${1:-install}" in
  install) do_install ;;
  update)  do_update ;;
  verify)  echo "Release signing key: ${PUBKEY}"; echo "Published at https://github.com/${REPO}#verifying-releases" ;;
  -h|--help|help) sed -n '2,20p' "$0" ;;
  *) die "Unknown command '$1' (use: install | update | verify)" ;;
esac
