#!/usr/bin/env bash
# SigardaPanel installer / updater
#
#   sudo bash install.sh            install the latest release
#   sudo bash install.sh update     update an existing install (backup + automatic rollback)
#   sudo bash install.sh rollback [BACKUP|list]
#                                   return to the state before an update (binary, web, database)
#   sudo bash install.sh verify     show the release-signing key this script trusts
#
# Every download is checked against a minisign signature (public key below) and
# SHA-256 checksums before anything is installed. There is no option to skip it.
#
# Environment (all optional):
#   SIGARDA_VERSION       release tag to install, e.g. v1.5.0 (default: latest)
#   DOMAIN                public hostname; enables a matching TLS name and ORIGIN
#   LETSENCRYPT_EMAIL     with DOMAIN: request a Let's Encrypt certificate via nginx
#   ADMIN_USER            admin username          (default: admin)
#   ADMIN_EMAIL           admin e-mail            (default: admin@localhost)
#   ADMIN_PASS            admin password          (default: random, printed once)
#   INSTALL_FIREWALL=yes  allow SSH/80/443 in ufw and enable it (never resets rules)
#   SIGARDA_ASSET_BASE    alternative download directory (testing; verification still applies)
#   FORCE=1               with "update": reinstall even if already on the target version
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
# An if (not "&&") so the EXIT trap does not turn a successful run into exit status 1.
cleanup() { if [ -n "$TMP" ]; then rm -rf "$TMP"; fi; }
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
  # certbot: the panel certificate (DOMAIN + LETSENCRYPT_EMAIL) and the
  # Let's Encrypt certificates the panel requests for every new site.
  apt-get install -y -qq --no-install-recommends certbot python3-certbot-nginx >/dev/null \
    || warn "certbot install failed: sites will not get Let's Encrypt certificates until it is installed"
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
# Public sign-up is closed on self-hosted installs; create users in the panel instead.
SIGARDAPANEL_ALLOW_REGISTRATION=false
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
      LE_OK=1
      ok "Let's Encrypt certificate installed"
    else
      warn "Let's Encrypt failed (DNS must point here and port 80 be reachable); keeping the self-signed certificate"
    fi
  fi
}

wait_http() { # <url> <seconds>
  for _ in $(seq 1 "$2"); do
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
  # A password the operator supplies is theirs; a generated one is temporary
  # and must be replaced at the first login.
  ADMIN_PASS_GENERATED=""
  if [ -z "${ADMIN_PASS:-}" ]; then
    ADMIN_PASS="$(openssl rand -base64 24 | tr -d '=+/' | cut -c1-20)"
    ADMIN_PASS_GENERATED=1
  fi
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
  ok "Binary: $("$INSTALL_BIN" version --client)"

  log "Creating the admin account..."
  local enc_key
  enc_key="$(grep -E '^SIGARDAPANEL_ENCRYPTION_KEY=' "$CONF_DIR/panel.env" | cut -d= -f2-)"
  runuser -u sigardapanel -- env -i PATH=/usr/sbin:/usr/bin:/sbin:/bin SIGARDAPANEL_DB_PATH="${DATA_DIR}/sigardapanel.db" \
    SIGARDAPANEL_ENCRYPTION_KEY="$enc_key" SIGARDAPANEL_ADMIN_PASSWORD="$ADMIN_PASS" \
    sh -c 'umask 027; exec "$0" "$@"' "$INSTALL_BIN" init --user "$ADMIN_USER" --email "$ADMIN_EMAIL" >/dev/null || die "Could not create the admin user"
  # "init" creates an "admin"; the installing owner must be the super_admin
  # (user management across accounts, database server settings, audit purge).
  runuser -u sigardapanel -- sqlite3 "${DATA_DIR}/sigardapanel.db" \
    "UPDATE users SET role='super_admin' WHERE username='${ADMIN_USER}' AND NOT EXISTS (SELECT 1 FROM users WHERE role='super_admin');" \
    || die "Could not grant super_admin to ${ADMIN_USER}"
  # The API refuses everything but the password change while
  # password_changed_at is NULL. Set it explicitly either way so the result
  # does not depend on what this release's "init" defaults to.
  local pw_changed="CURRENT_TIMESTAMP"
  if [ -n "$ADMIN_PASS_GENERATED" ]; then pw_changed="NULL"; fi
  runuser -u sigardapanel -- sqlite3 "${DATA_DIR}/sigardapanel.db" \
    "UPDATE users SET password_changed_at=${pw_changed} WHERE username='${ADMIN_USER}';" \
    || die "Could not record the password state of ${ADMIN_USER}"
  chmod 640 "${DATA_DIR}"/sigardapanel.db* 2>/dev/null || true

  log "Starting services..."
  systemctl enable --now sigardapanel-api sigardapanel-agent sigardapanel-web >/dev/null 2>&1
  write_nginx
  healthy || { journalctl -u sigardapanel-api -u sigardapanel-web -n 30 --no-pager || true; die "Health check failed"; }
  ok "API and web are healthy"

  if [ "${INSTALL_FIREWALL:-no}" = "yes" ]; then
    apt-get install -y -qq ufw >/dev/null
    local ssh_port
    ssh_port=""
    if command -v sshd >/dev/null 2>&1; then
      ssh_port="$( (sshd -T 2>/dev/null || true) | awk '/^port /{print $2; exit}')"
    fi
    [[ "$ssh_port" =~ ^[0-9]+$ ]] || ssh_port=22
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
  if [ -z "${LE_OK:-}" ]; then
    warn "The certificate is self-signed (browser warning). Use DOMAIN=panel.example.com + LETSENCRYPT_EMAIL for a trusted one."
    warn "New servers added with the one-line install command must then be given the panel certificate (docs/INSTALL.md, SIGARDAPANEL_PANEL_CA)."
  fi
  if [ "${INSTALL_FIREWALL:-no}" != "yes" ]; then
    warn "No firewall was configured. Recommended: allow only SSH, 80 and 443 - e.g. ufw allow OpenSSH && ufw allow 80,443/tcp && ufw enable"
  fi
  echo
  echo "  Next steps:"
  if [ -n "$ADMIN_PASS_GENERATED" ]; then
    echo "   1. Open ${PUBLIC_URL} and log in; you must choose your own password first"
    echo "      (the API refuses everything else until then; CLI: sigardapanel passwd)."
  else
    echo "   1. Open ${PUBLIC_URL} and log in with the ADMIN_PASS you supplied."
  fi
  echo "   2. Enable two-factor authentication under Settings, then: rm /root/.sigardapanel_admin_cred"
  echo "   3. This server is already managed as \"Local Server\". To add more servers:"
  echo "      Servers -> Add server (docs/INSTALL.md#adding-remote-servers)."
  echo "   CLI on this server:  sigardapanel login --api-url http://127.0.0.1:7700 --user ${ADMIN_USER}"
  echo "   Update later:        sudo bash install.sh update"
}

# panel_db_file: the database path from panel.env (empty when unset).
panel_db_file() { grep -E '^SIGARDAPANEL_DB_PATH=' "$CONF_DIR/panel.env" | cut -d= -f2- || true; }

# make_backup <dir> <kind> <target>: save binary, web bundle, a consistent copy
# of the database and the secrets needed to decrypt it. VERSION/TARGET/KIND
# describe the backup for "rollback" and the panel's backup list.
make_backup() {
  local bk="$1" kind="$2" target="$3" dbfile keyfile
  install -d -m 700 "$bk"
  cp -a "$INSTALL_BIN" "$bk/sigardapanel.bin"
  cp -a "$WEB_DIR" "$bk/web"
  dbfile="$(panel_db_file)"
  if [ -f "$dbfile" ]; then sqlite3 "$dbfile" ".backup '${bk}/sigardapanel.db'"; fi
  # Secrets needed to decrypt the database (never modified by the update, saved for disaster recovery).
  cp -a "$CONF_DIR/panel.env" "$CONF_DIR/agent.env" "$bk/" 2>/dev/null || true
  keyfile="$(dirname "${dbfile:-$DATA_DIR/x}")/encryption.key"
  if [ -f "$keyfile" ]; then cp -a "$keyfile" "$bk/"; fi
  { "$INSTALL_BIN" version --client 2>/dev/null || echo unknown; } | head -n1 > "$bk/VERSION"
  echo "$target" > "$bk/TARGET"
  echo "$kind" > "$bk/KIND"
}

# restore_backup <dir>: stop the services, put back binary, web bundle and
# database from <dir>, start the services again. Returns non-zero when a
# step fails (the services are started either way).
restore_backup() {
  local bk="$1" dbfile rc=0
  dbfile="$(panel_db_file)"
  # Stop first: the database must not be replaced under a running API.
  systemctl stop sigardapanel-web sigardapanel-api sigardapanel-agent || true
  { install -m 755 "$bk/sigardapanel.bin" "${INSTALL_BIN}.rb" && mv -f "${INSTALL_BIN}.rb" "$INSTALL_BIN"; } || { warn "could not restore the binary"; rc=1; }
  { rm -rf "${WEB_DIR}.rb" && cp -a "$bk/web" "${WEB_DIR}.rb" && rm -rf "$WEB_DIR" && mv "${WEB_DIR}.rb" "$WEB_DIR"; } \
    || { warn "could not restore the web bundle"; rc=1; }
  if [ -f "$bk/sigardapanel.db" ] && [ -n "$dbfile" ]; then
    rm -f "${dbfile}-wal" "${dbfile}-shm"
    { cp -a "$bk/sigardapanel.db" "$dbfile" && chown sigardapanel:sigardapanel "$dbfile" && chmod 640 "$dbfile"; } \
      || { warn "could not restore the database"; rc=1; }
  fi
  systemctl start sigardapanel-agent sigardapanel-api sigardapanel-web || rc=1
  return "$rc"
}

do_update() {
  require_root; check_os; detect_arch
  [ -x "$INSTALL_BIN" ] && [ -f "$CONF_DIR/panel.env" ] || die "SigardaPanel is not installed here. Run: bash install.sh"
  command -v minisign >/dev/null || { apt-get update -qq; apt-get install -y -qq minisign >/dev/null; }
  local current; current="$("$INSTALL_BIN" version --client 2>/dev/null || echo unknown)"
  resolve_tag
  if [ "${current#v}" = "${TAG#v}" ] && [ -z "${FORCE:-}" ]; then
    ok "Already on ${TAG}. (FORCE=1 to reinstall)"; return 0
  fi
  log "Updating ${current} -> ${TAG}"
  TMP="$(mktemp -d)"
  download_release "$TMP"
  unpack_web "$TMP/sigardapanel-web.tar.gz" "$TMP/web"

  local ts bk; ts="$(date -u +%Y%m%dT%H%M%SZ)"; bk="${UPDATE_BACKUPS}/${ts}"
  make_backup "$bk" update "$TAG"
  ok "Backup saved to ${bk}"

  rollback() {
    warn "Update failed - rolling back"
    restore_backup "$bk" || true
    if healthy; then
      die "Update to ${TAG} failed; rolled back to ${current} and the panel is healthy again. Backup kept in ${bk}"
    fi
    die "Update to ${TAG} failed and the rolled-back panel is NOT healthy. Backup: ${bk}. Check: journalctl -u sigardapanel-api -u sigardapanel-web"
  }

  # Any failure from here on (copying files, a unit that refuses to start, a
  # failed health check) must end in a rollback, never in a half-updated panel.
  if ! { install -m 755 "$TMP/sigardapanel-linux-${ARCH}" "${INSTALL_BIN}.new" && mv -f "${INSTALL_BIN}.new" "$INSTALL_BIN" \
      && rm -rf "${WEB_DIR}.new" && cp -a "$TMP/web" "${WEB_DIR}.new" \
      && chown -R root:root "${WEB_DIR}.new" && chmod -R go-w "${WEB_DIR}.new" \
      && rm -rf "$WEB_DIR" && mv "${WEB_DIR}.new" "$WEB_DIR"; }; then
    rollback
  fi
  systemctl restart sigardapanel-agent || true; sleep 2
  systemctl restart sigardapanel-api || true; sleep 2
  systemctl restart sigardapanel-web || true
  healthy || rollback
  ok "Updated to $("$INSTALL_BIN" version --client)"
  if grep -qE '^SIGARDAPANEL_ALLOW_REGISTRATION=true' "$CONF_DIR/panel.env"; then
    warn "Public sign-up is OPEN on this panel (SIGARDAPANEL_ALLOW_REGISTRATION=true in ${CONF_DIR}/panel.env)."
  fi
}

# list_backups: the update backups that can be rolled back to, newest first.
list_backups() {
  local d name kind ver
  [ -d "$UPDATE_BACKUPS" ] || return 0
  for d in $(find "$UPDATE_BACKUPS" -mindepth 1 -maxdepth 1 -type d -name '20*T*Z' -printf '%f\n' | sort -r); do
    name="$d"; d="${UPDATE_BACKUPS}/${d}"
    [ -f "$d/sigardapanel.bin" ] && [ -f "$d/web/index.js" ] || continue
    kind="$(cat "$d/KIND" 2>/dev/null || echo update)"
    ver="$(cat "$d/VERSION" 2>/dev/null || echo unknown)"
    printf '%s\t%s\t%s\n' "$name" "$ver" "$kind"
  done
}

# do_rollback [backup]: return to the state saved before an update (binary,
# web bundle and database; changes made since that backup are lost). The
# current state is backed up first, and restored if the rolled-back panel is
# not healthy. Without an argument the newest update backup is used.
do_rollback() {
  require_root
  [ -x "$INSTALL_BIN" ] && [ -f "$CONF_DIR/panel.env" ] || die "SigardaPanel is not installed here."
  local name="${1:-}" bk current target ts safety
  if [ "$name" = "list" ]; then
    printf 'BACKUP\tVERSION\tKIND\n'; list_backups; return 0
  fi
  if [ -z "$name" ]; then
    name="$(list_backups | awk -F'\t' '$3 == "update" {print $1; exit}')"
    [ -n "$name" ] || die "No update backup found in ${UPDATE_BACKUPS}"
  fi
  [[ "$name" =~ ^20[0-9]{6}T[0-9]{6}Z$ ]] || die "Invalid backup name '${name}' (see: bash install.sh rollback list)"
  bk="${UPDATE_BACKUPS}/${name}"
  [ -f "$bk/sigardapanel.bin" ] && [ -f "$bk/web/index.js" ] || die "Backup ${bk} is incomplete (needs sigardapanel.bin and web/)"
  [ -f "$bk/sigardapanel.db" ] || warn "Backup ${name} has no database copy; the current database is kept"
  target="$("$bk/sigardapanel.bin" version --client 2>/dev/null | head -n1 || true)"
  [ -n "$target" ] || die "The binary in ${bk} does not run"
  current="$("$INSTALL_BIN" version --client 2>/dev/null || echo unknown)"
  log "Rolling back ${current} -> ${target} (backup ${name})"

  ts="$(date -u +%Y%m%dT%H%M%SZ)"
  if [ "$ts" = "$name" ]; then sleep 1; ts="$(date -u +%Y%m%dT%H%M%SZ)"; fi
  safety="${UPDATE_BACKUPS}/${ts}"
  make_backup "$safety" pre-rollback "$target"
  ok "Current state saved to ${safety}"

  if restore_backup "$bk" && healthy; then
    ok "Rolled back to $("$INSTALL_BIN" version --client) (backup ${name}). The state before the rollback is in ${safety}"
    return 0
  fi
  warn "The rolled-back panel is not healthy - restoring the state before the rollback"
  restore_backup "$safety" || true
  if healthy; then
    die "Rollback to ${target} failed; restored ${current} and the panel is healthy again."
  fi
  die "Rollback failed and restoring ${current} did not bring the panel back. Backups: ${bk}, ${safety}. Check: journalctl -u sigardapanel-api -u sigardapanel-web"
}

case "${1:-install}" in
  install) do_install ;;
  update)  do_update ;;
  rollback) shift; do_rollback "${1:-}" ;;
  verify)  echo "Release signing key: ${PUBKEY}"; echo "Published at https://github.com/${REPO}#verifying-releases" ;;
  -h|--help|help) sed -n '2,22p' "$0" ;;
  *) die "Unknown command '$1' (use: install | update | rollback | verify)" ;;
esac
