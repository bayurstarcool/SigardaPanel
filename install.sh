#!/usr/bin/env bash
# SigardaPanel installer / updater
#
#   sudo bash install.sh            install the latest release
#   sudo bash install.sh update     update an existing install (backup + automatic rollback)
#   sudo bash install.sh rollback [BACKUP|list]
#                                   return to the state before an update (binary, web, database)
#   sudo bash install.sh verify     show the release-signing key this script trusts
#   sudo bash install.sh phpmyadmin [off]
#                                   add (or remove) phpMyAdmin for MariaDB/MySQL databases on
#                                   this host; the panel signs users in with the database's account
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
#   INSTALL_PHPMYADMIN=yes  also run "phpmyadmin" at the end of the install
#   SIGARDA_ASSET_BASE    alternative download directory (testing; verification still applies)
#   SIGARDA_REPO          GitHub repository the releases come from (default below;
#                         the master panel uses its private repository)
#   SIGARDA_UPDATE_TOKEN  read-only GitHub token for a private release repository
#                         (default: SIGARDAPANEL_UPDATE_TOKEN in /etc/sigardapanel/panel.env)
#   SIGARDA_UPDATE_TOKEN_FILE  file holding that token (set by the agent for a
#                         token from a super admin's GitHub connection)
#   FORCE=1               with "update": reinstall even if already on the target version
set -euo pipefail
umask 022

REPO="${SIGARDA_REPO:-bayurstarcool/SigardaPanel}"
[[ "$REPO" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || { echo "Invalid SIGARDA_REPO '$REPO'" >&2; exit 1; }
# minisign public key used to verify releases. Also published in the repository README.
PUBKEY="RWRuJBU1HM8C+XuaO7KjjPf1Bzwjara6ujA7FnHpqlbVIT6J/QGMC6BM"

INSTALL_BIN="/usr/local/bin/sigardapanel"
AGENT_UNIT="/etc/systemd/system/sigardapanel-agent.service"
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

# Node.js for the web process (SvelteKit 3 needs >= NODE_MIN). The unit runs
# NODE_BIN; when it is missing or older, Node.js 22 comes from NodeSource with
# the same keyring, source and pin files the Stack page uses (a different
# signed-by for the same repository would break apt). A newer Node.js that is
# already installed (e.g. 24 from the Stack page) is kept.
NODE_BIN="/usr/bin/node"
NODE_MIN="22.17"
NODE_MAJOR="22"

node_version() { [ -x "$NODE_BIN" ] && "$NODE_BIN" -v 2>/dev/null | sed 's/^v//' || true; }
node_ok() { # node_ok [version]: version >= NODE_MIN
  local v="${1:-$(node_version)}" major minor want_major want_minor
  [[ "$v" =~ ^[0-9]+\.[0-9]+ ]] || return 1
  major="${v%%.*}"; minor="${v#*.}"; minor="${minor%%.*}"
  want_major="${NODE_MIN%%.*}"; want_minor="${NODE_MIN#*.}"
  [ "$major" -gt "$want_major" ] || { [ "$major" -eq "$want_major" ] && [ "$minor" -ge "$want_minor" ]; }
}

ensure_node() {
  if node_ok; then return 0; fi
  local before; before="$(node_version)"
  log "Installing Node.js ${NODE_MAJOR} from NodeSource (the web panel needs >= ${NODE_MIN}; found ${before:-none})..."
  export DEBIAN_FRONTEND=noninteractive
  apt-get install -y -qq --no-install-recommends ca-certificates curl gnupg >/dev/null || die "could not install curl/gnupg for Node.js"
  local keyring=/etc/apt/keyrings/nodesource.gpg
  if [ ! -s "$keyring" ]; then
    install -d -m 755 /etc/apt/keyrings
    curl -fsSL --retry 3 https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key | gpg --dearmor --batch --yes -o "$keyring" \
      || die "could not download the NodeSource signing key"
    chmod 644 "$keyring"
  fi
  echo "deb [signed-by=${keyring}] https://deb.nodesource.com/node_${NODE_MAJOR}.x nodistro main" > /etc/apt/sources.list.d/nodesource.list
  printf 'Package: nodejs\nPin: origin deb.nodesource.com\nPin-Priority: 600\n' > /etc/apt/preferences.d/nodesource
  apt-get update -qq || die "apt-get update failed after adding NodeSource"
  apt-get install -y -qq --allow-downgrades nodejs >/dev/null || die "installing Node.js ${NODE_MAJOR} failed"
  node_ok || die "Node.js $(node_version) is installed at ${NODE_BIN}, but the web panel needs >= ${NODE_MIN}"
  ok "Node.js $(node_version) installed${before:+ (was ${before})}"
}

install_packages() {
  export DEBIAN_FRONTEND=noninteractive
  log "Installing system packages..."
  apt-get update -qq
  apt-get install -y -qq --no-install-recommends \
    curl ca-certificates openssl nginx sqlite3 minisign tar >/dev/null
  # certbot: the panel certificate (DOMAIN + LETSENCRYPT_EMAIL) and the
  # Let's Encrypt certificates the panel requests for every new site.
  apt-get install -y -qq --no-install-recommends certbot python3-certbot-nginx >/dev/null \
    || warn "certbot install failed: sites will not get Let's Encrypt certificates until it is installed"
  ensure_node
  ok "Packages ready (node $(node_version), minisign $(minisign -v 2>&1 | awk '{print $2}'))"
}

# update_token: the token for a private release repository, if any. It is
# read from the root-only panel.env (or a 0600 file the agent writes for a
# token from a super admin's GitHub connection), never passed on a command line.
update_token() {
  if [ -n "${SIGARDA_UPDATE_TOKEN:-}" ]; then printf '%s' "$SIGARDA_UPDATE_TOKEN"; return; fi
  if [ -n "${SIGARDA_UPDATE_TOKEN_FILE:-}" ] && [ -f "$SIGARDA_UPDATE_TOKEN_FILE" ]; then
    tr -d '\r\n' < "$SIGARDA_UPDATE_TOKEN_FILE"; return
  fi
  [ -f "$CONF_DIR/panel.env" ] || return 0
  grep -E '^SIGARDAPANEL_UPDATE_TOKEN=' "$CONF_DIR/panel.env" | tail -n1 | cut -d= -f2- | tr -d '"'"'"'\r' || true
}

# github_api <path>: GET from the GitHub API with the update token.
# The token goes to curl on stdin (-K -), never on its command line (ps).
auth_header() { printf 'header = "Authorization: Bearer %s"\n' "$(update_token)"; }
github_api() {
  auth_header | curl -fsSL --max-time 30 -K - -H "Accept: application/vnd.github+json" \
    "https://api.github.com/repos/${REPO}$1"
}

need_python() {
  command -v python3 >/dev/null || { apt-get update -qq; apt-get install -y -qq python3 >/dev/null; } || die "python3 is required"
}

resolve_tag() {
  if [ -n "${SIGARDA_VERSION:-}" ]; then TAG="$SIGARDA_VERSION"; return; fi
  if [ -n "${SIGARDA_ASSET_BASE:-}" ]; then die "Set SIGARDA_VERSION together with SIGARDA_ASSET_BASE"; fi
  if [ -n "$(update_token)" ]; then
    need_python
    TAG="$(github_api /releases/latest | python3 -c 'import json,sys; print(json.load(sys.stdin)["tag_name"])')" \
      || die "Could not read the latest release of ${REPO} (check SIGARDAPANEL_UPDATE_TOKEN)"
    [[ "$TAG" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][A-Za-z0-9.]+)?$ ]] || die "Unexpected release tag '$TAG'"
    return
  fi
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
  if [ -z "${SIGARDA_ASSET_BASE:-}" ] && [ -n "$(update_token)" ]; then
    # Private repository: assets are fetched through the API with the token
    # (curl does not forward it to the storage host the API redirects to).
    need_python
    local assets
    assets="$(github_api "/releases/tags/${TAG}")" || die "Could not read release ${TAG} of ${REPO} (check SIGARDAPANEL_UPDATE_TOKEN)"
    for f in "sigardapanel-linux-${ARCH}" "sigardapanel-web.tar.gz" "SHA256SUMS" "SHA256SUMS.minisig"; do
      local url
      url="$(printf '%s' "$assets" | F="$f" python3 -c 'import json,os,sys; print(next(a["url"] for a in json.load(sys.stdin)["assets"] if a["name"] == os.environ["F"]))')" \
        || die "Release ${TAG} has no asset ${f}"
      [[ "$url" =~ ^https://api\.github\.com/repos/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+/releases/assets/[0-9]+$ ]] || die "Unexpected asset URL for ${f}"
      auth_header | curl -fsSL --max-time 600 --retry 3 -K - -H "Accept: application/octet-stream" \
        -o "${dir}/${f}" "$url" || die "Download failed: ${f}"
    done
  else
    for f in "sigardapanel-linux-${ARCH}" "sigardapanel-web.tar.gz" "SHA256SUMS" "SHA256SUMS.minisig"; do
      curl -fsSL --max-time 600 --retry 3 -o "${dir}/${f}" "${base}/${f}" || die "Download failed: ${base}/${f}"
    done
  fi
  log "Verifying signature and checksums..."
  minisign -Vm "${dir}/SHA256SUMS" -P "$PUBKEY" -x "${dir}/SHA256SUMS.minisig" >/dev/null \
    || die "SIGNATURE VERIFICATION FAILED - refusing to install. The download may have been tampered with."
  ( cd "$dir" && grep -E " (sigardapanel-linux-${ARCH}|sigardapanel-web\.tar\.gz)\$" SHA256SUMS | sha256sum -c --quiet - ) \
    || die "CHECKSUM MISMATCH - refusing to install."
  local n
  n="$(grep -cE " (sigardapanel-linux-${ARCH}|sigardapanel-web\.tar\.gz)\$" "${dir}/SHA256SUMS")"
  [ "$n" -eq 2 ] || die "SHA256SUMS does not list the expected files"
  # The signature covers SHA256SUMS, not the tag: an older signed release
  # published under a newer tag would otherwise install (a downgrade).
  chmod +x "${dir}/sigardapanel-linux-${ARCH}"
  local got
  got="$("${dir}/sigardapanel-linux-${ARCH}" version --client 2>/dev/null | head -n1 || true)"
  [ "${got#v}" = "${TAG#v}" ] || die "The downloaded binary reports version '${got}', expected ${TAG#v} - refusing to install."
  ok "Release ${TAG} verified (minisign key ${PUBKEY:0:12}...)"
}

unpack_web() { # <tarball> <dest>
  mkdir -p "$2"
  tar -xzf "$1" -C "$2" --no-same-owner --no-same-permissions
  # The web service runs as an unprivileged user: make the bundle readable
  # whatever modes the archive carried (v1.7.0's were owner-only).
  chmod -R u+rwX,go+rX,go-w "$2"
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
Environment=HOME=/root
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
# adapter-node refuses bodies over 512K by default (file manager uploads
# go through the web process; the API caps them at 32M).
Environment=BODY_SIZE_LIMIT=34M
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

    # The web UI signs in through its own route: same limit.
    location = /api/auth/login {
        limit_req zone=sigarda_login burst=5 nodelay;
        proxy_pass http://127.0.0.1:7720;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-Proto \$scheme;
    }

    # phpMyAdmin credentials for nginx's internal auth_request only
    # (deploy/nginx/pma-per-database.conf proxies to it directly).
    location = /api/pma-auth {
        deny all;
    }

    # Optional tools (install.sh phpmyadmin writes sigardapanel-pma.conf).
    include /etc/nginx/snippets/sigardapanel-*.conf;

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
  # The super admin's Linux account may be a sudo-capable cloud default
  # ("admin" on Debian images): the agent refuses privileged accounts as site
  # owners unless the operator lists them here.
  if ! grep -qxF "$ADMIN_USER" "$CONF_DIR/trusted-site-users" 2>/dev/null; then
    printf '%s\n' "$ADMIN_USER" >> "$CONF_DIR/trusted-site-users"
  fi
  chown root:root "$CONF_DIR/trusted-site-users"; chmod 644 "$CONF_DIR/trusted-site-users"
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

  if [ "${INSTALL_PHPMYADMIN:-no}" = "yes" ]; then
    ( do_phpmyadmin ) || warn "phpMyAdmin was not set up; run later: sudo bash install.sh phpmyadmin"
  fi

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

# migrate_agent_unit: units written before v1.16.0 set User=root. With
# systemd 255, an explicit User=root plus NoNewPrivileges and a seccomp option
# starts the agent without CAP_SETUID, so tasks that run as another account
# (psql as postgres, deploys, the site terminal, restores) fail. Root is the
# default user of a system service, so the line is replaced by HOME=/root.
migrate_agent_unit() {
  local unit="$AGENT_UNIT"
  [ -f "$unit" ] && grep -qx 'User=root' "$unit" || return 0
  if grep -qx 'Environment=HOME=/root' "$unit"; then
    sed -i '/^User=root$/d' "$unit"
  else
    sed -i 's|^User=root$|Environment=HOME=/root|' "$unit"
  fi
  systemctl daemon-reload || true
  log "Agent unit: removed the explicit User=root (systemd 255 + NoNewPrivileges drops CAP_SETUID)"
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
  # Before anything is replaced: a failure here leaves the panel as it was.
  ensure_node

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
  migrate_agent_unit
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

# ─── phpMyAdmin (optional) ─────────────────────────────────────────────────
# phpMyAdmin runs behind an internal vhost (127.0.0.1:8081) and is reached
# only through the panel at /pma/: the panel hands out a one-time ticket
# (/pma/ticket), nginx asks the web process for the database account on every
# request (auth_request) and phpMyAdmin gets it as HTTP Basic credentials.
# There is no phpMyAdmin login page and no root or control-user sign-on.
PMA_SNIPPET=/etc/nginx/snippets/sigardapanel-pma.conf
PMA_VHOST=/etc/nginx/sites-available/sigardapanel-pma-internal
PMA_CONF=/etc/phpmyadmin/conf.d/sigardapanel.php
PMA_WEB_DROPIN=/etc/systemd/system/sigardapanel-web.service.d/phpmyadmin.conf

env_set() { # <file> <KEY> <value>: replace or append KEY=value
  local f="$1" k="$2" v="$3"
  if grep -qE "^${k}=" "$f"; then
    sed -i "s|^${k}=.*|${k}=${v}|" "$f"
  else
    printf '%s=%s\n' "$k" "$v" >> "$f"
  fi
}

ensure_pma_include() {
  local site=/etc/nginx/sites-available/sigardapanel
  [ -f "$site" ] || die "The panel's nginx site ($site) is missing"
  grep -q 'snippets/sigardapanel-\*.conf' "$site" && return 0
  # Panels installed before this option: add the include before the API block.
  grep -q '# Go API (also carries the terminal WebSocket)' "$site" || die "Cannot find where to add phpMyAdmin in $site; add 'include /etc/nginx/snippets/sigardapanel-*.conf;' to its 443 server block"
  sed -i 's|^    # Go API (also carries the terminal WebSocket)|    # Optional tools (install.sh phpmyadmin writes sigardapanel-pma.conf).\n    include /etc/nginx/snippets/sigardapanel-*.conf;\n\n    # Go API (also carries the terminal WebSocket)|' "$site"
}

pma_disable() {
  rm -f "$PMA_SNIPPET" "/etc/nginx/sites-enabled/$(basename "$PMA_VHOST")" "$PMA_VHOST" "$PMA_WEB_DROPIN"
  sed -i '/^SIGARDAPANEL_PHPMYADMIN=/d' "$CONF_DIR/panel.env"
  nginx -t >/dev/null 2>&1 && systemctl reload nginx
  systemctl daemon-reload
  systemctl restart sigardapanel-api sigardapanel-web || true
  ok "phpMyAdmin is no longer reachable through the panel (the phpmyadmin package is left installed: apt-get remove phpmyadmin)"
}

do_phpmyadmin() {
  require_root; check_os
  [ -x "$INSTALL_BIN" ] && [ -f "$CONF_DIR/panel.env" ] || die "SigardaPanel is not installed here. Run: bash install.sh"
  case "${1:-}" in off|disable|remove) pma_disable; return 0 ;; "" | on) ;; *) die "Use: install.sh phpmyadmin [off]" ;; esac
  if ! command -v mariadbd >/dev/null && ! command -v mysqld >/dev/null && [ ! -x /usr/sbin/mariadbd ] && [ ! -x /usr/sbin/mysqld ]; then
    die "No MariaDB/MySQL server on this host. Install MariaDB first (Stack page in the panel, or apt-get install mariadb-server)."
  fi
  export DEBIAN_FRONTEND=noninteractive
  log "Installing phpMyAdmin..."
  # No database for phpMyAdmin's own storage and no Apache configuration.
  printf '%s\n' "phpmyadmin phpmyadmin/dbconfig-install boolean false" \
    "phpmyadmin phpmyadmin/reconfigure-webserver multiselect" | debconf-set-selections
  apt-get update -qq
  # php-fpm satisfies phpMyAdmin's PHP dependency (otherwise apt may choose Apache's mod_php).
  apt-get install -y -qq --no-install-recommends php-fpm phpmyadmin >/dev/null || die "Installing phpMyAdmin failed"
  local phpv; phpv="$(php -r 'echo PHP_MAJOR_VERSION.".".PHP_MINOR_VERSION;' 2>/dev/null)"
  [[ "$phpv" =~ ^[0-9]+\.[0-9]+$ ]] || die "PHP did not install correctly"
  apt-get install -y -qq --no-install-recommends "php${phpv}-fpm" "php${phpv}-mysql" "php${phpv}-mbstring" "php${phpv}-xml" "php${phpv}-zip" "php${phpv}-gd" "php${phpv}-curl" >/dev/null \
    || die "Installing PHP ${phpv} extensions for phpMyAdmin failed"
  [ -d /usr/share/phpmyadmin ] || die "phpMyAdmin files are missing (/usr/share/phpmyadmin)"

  # Own FPM pool (socket used only by the internal vhost).
  cat > "/etc/php/${phpv}/fpm/pool.d/sigardapanel-pma.conf" <<POOL
; Managed by the SigardaPanel installer (phpMyAdmin).
[sigardapanel-pma]
user = www-data
group = www-data
listen = /run/php/sigardapanel-pma.sock
listen.owner = www-data
listen.group = www-data
listen.mode = 0660
pm = ondemand
pm.max_children = 8
pm.process_idle_timeout = 60s
php_admin_value[upload_max_filesize] = 64M
php_admin_value[post_max_size] = 64M
php_admin_value[max_execution_time] = 300
php_admin_value[memory_limit] = 512M
POOL
  systemctl enable "php${phpv}-fpm" >/dev/null 2>&1 || true
  systemctl restart "php${phpv}-fpm" || die "php${phpv}-fpm did not start"

  local origin; origin="$(grep -E '^SIGARDAPANEL_PUBLIC_URL=' "$CONF_DIR/panel.env" | cut -d= -f2- | tr -d '"'"'"'\r')"
  install -d -m 755 /etc/phpmyadmin/conf.d
  local blowfish; blowfish="$(rand_hex 16)"
  [ -f "$PMA_CONF" ] && blowfish="$(sed -n "s/^\$cfg\['blowfish_secret'\] = '\([0-9a-f]\{32\}\)';$/\1/p" "$PMA_CONF")"
  [ -n "$blowfish" ] || blowfish="$(rand_hex 16)"
  cat > "$PMA_CONF" <<PHP
<?php
// Managed by the SigardaPanel installer. Sign-on comes from the panel (/pma/ticket):
// nginx passes the database's own account as HTTP Basic credentials.
\$cfg['blowfish_secret'] = '${blowfish}';
\$cfg['Servers'][1]['auth_type'] = 'http';
\$cfg['Servers'][1]['host'] = 'localhost';
\$cfg['Servers'][1]['AllowRoot'] = false;
\$cfg['Servers'][1]['AllowNoPassword'] = false;
\$cfg['Servers'][1]['hide_db'] = '^(information_schema|performance_schema|mysql|sys|phpmyadmin)\$';
\$cfg['PmaAbsoluteUri'] = '${origin}/pma/';
\$cfg['VersionCheck'] = false;
\$cfg['SendErrorReports'] = 'never';
\$cfg['ShowServerInfo'] = false;
// No storage database (bookmarks etc.), and passwords are changed in the panel,
// which keeps the copy it signs in with.
\$cfg['PmaNoRelation_DisableWarning'] = true;
\$cfg['ShowChgPassword'] = false;
PHP
  chown root:www-data "$PMA_CONF"; chmod 640 "$PMA_CONF"

  cat > "$PMA_VHOST" <<'NGINX'
# Managed by the SigardaPanel installer: phpMyAdmin, reachable only through the panel's /pma/.
server {
    listen 127.0.0.1:8081;
    server_name localhost;
    root /usr/share/phpmyadmin;
    index index.php;
    client_max_body_size 64M;
    server_tokens off;

    location ~ ^/(setup|libraries|templates|sql|vendor|tmp|test|doc)(/|$) { deny all; }
    location / { try_files $uri $uri/ =404; }
    location ~ \.php$ {
        include snippets/fastcgi-php.conf;
        fastcgi_pass unix:/run/php/sigardapanel-pma.sock;
        fastcgi_read_timeout 300s;
    }
}
NGINX
  ln -sfn "$PMA_VHOST" "/etc/nginx/sites-enabled/$(basename "$PMA_VHOST")"

  install -d -m 755 /etc/nginx/snippets
  cat > "$PMA_SNIPPET" <<'NGINX'
# Managed by the SigardaPanel installer (install.sh phpmyadmin). Included in the panel's 443 server.
location = /pma/ticket {
    proxy_pass http://127.0.0.1:7720;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-Proto $scheme;
}
location ^~ /pma/t/ {
    proxy_pass http://127.0.0.1:7720;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-Proto $scheme;
}
location ^~ /pma/ {
    auth_request /pma-auth;
    auth_request_set $pma_authorization $upstream_http_x_pma_authorization;
    error_page 401 = @pma_login;

    # The panel's sg_ marker only decorates the browser URL; phpMyAdmin uses
    # its own "token" parameter, so never forward ours.
    if ($arg_token ~ "^sg_[A-Za-z0-9_-]{43}$") {
        set $args "";
    }

    proxy_pass http://127.0.0.1:8081/;
    proxy_set_header Host localhost;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header Authorization $pma_authorization;
    proxy_buffering off;
    proxy_read_timeout 300s;
    client_max_body_size 64M;

    # add_header here replaces the server-level headers: repeat them.
    add_header Cache-Control "no-store" always;
    add_header Referrer-Policy "no-referrer" always;
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header Strict-Transport-Security "max-age=31536000" always;
}
location = /pma {
    return 302 /pma/;
}
location = /pma-auth {
    internal;
    proxy_pass http://127.0.0.1:7720/api/pma-auth;
    proxy_pass_request_body off;
    proxy_set_header Content-Length "";
    proxy_set_header Cookie $http_cookie;
}
location @pma_login {
    return 302 /databases;
}
NGINX
  ensure_pma_include

  # The ticket secret is for the web process only (it encrypts the tickets).
  local secret_file="$CONF_DIR/web.env"
  touch "$secret_file"; chmod 600 "$secret_file"
  grep -qE '^SIGARDAPANEL_PMA_SECRET=.{32,}' "$secret_file" || env_set "$secret_file" SIGARDAPANEL_PMA_SECRET "$(rand_hex 32)"
  install -d -m 755 "$(dirname "$PMA_WEB_DROPIN")"
  printf '[Service]\nEnvironmentFile=%s\n' "$secret_file" > "$PMA_WEB_DROPIN"
  env_set "$CONF_DIR/panel.env" SIGARDAPANEL_PHPMYADMIN enabled

  nginx -t >/dev/null 2>&1 || { nginx -t; die "nginx configuration test failed (phpMyAdmin left unconfigured: run 'install.sh phpmyadmin off')"; }
  systemctl reload nginx
  systemctl daemon-reload
  systemctl restart sigardapanel-api; sleep 1
  systemctl restart sigardapanel-web
  healthy || warn "The panel did not answer after the restart; check: journalctl -u sigardapanel-api -u sigardapanel-web"
  ok "phpMyAdmin $(dpkg-query -W -f='${Version}' phpmyadmin 2>/dev/null) is ready: open a MySQL database in the panel and click phpMyAdmin"
}

case "${1:-install}" in
  install) do_install ;;
  update)  do_update ;;
  rollback) shift; do_rollback "${1:-}" ;;
  phpmyadmin) shift; do_phpmyadmin "${1:-}" ;;
  verify)  echo "Release signing key: ${PUBKEY}"; echo "Published at https://github.com/${REPO}#verifying-releases" ;;
  -h|--help|help) sed -n '2,30p' "$0" ;;
  *) die "Unknown command '$1' (use: install | update | rollback | verify | phpmyadmin)" ;;
esac
