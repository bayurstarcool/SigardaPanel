#!/usr/bin/env bash
set -euo pipefail

# ╔══════════════════════════════════════════════════════════╗
# ║  SigardaPanel Agent Installer                           ║
# ║                                                         ║
# ║  One-line install:                                      ║
# ║    curl -H "X-Sigarda-Registration-Token: TOKEN"       ║
# ║      https://panel/api/v1/agents/install | bash          ║
# ║                                                         ║
# ║  Manual install:                                        ║
# ║    bash install-agent.sh --panel-url URL --token TOKEN  ║
# ╚══════════════════════════════════════════════════════════╝

PANEL_URL=""
REG_TOKEN=""
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/sigardapanel"
SITE_ROOT="${SIGARDAPANEL_AGENT_SITE_ROOT:-/var/www}"
SERVICE_NAME="sigardapanel-agent"
RELEASE_VERSION="${SIGARDAPANEL_RELEASE_VERSION:-v0.5.5}"
BINARY_URL="${SIGARDAPANEL_BINARY_URL:-https://github.com/bayurstarcool/SigardaPanel/releases/download/$RELEASE_VERSION/sigardapanel}"
BINARY_SHA256="${SIGARDAPANEL_BINARY_SHA256:-a4e0f9b10671cd465d1e66248b6512557dc497bf7b5d5d6aecdb6739cf7bf198}"
DASHBOARD_URL="${SIGARDAPANEL_DASHBOARD_URL:-https://github.com/bayurstarcool/SigardaPanel/releases/download/$RELEASE_VERSION/sigardapanel-info.sh}"

usage() {
    echo "SigardaPanel Agent Installer"
    echo ""
    echo "Usage:"
    echo "  $0 --panel-url URL --registration-token TOKEN"
    echo "  $0 --panel-url URL --token AGENT_TOKEN  (legacy: requires manual server creation)"
    echo ""
    echo "Options:"
    echo "  --panel-url URL                Panel API URL (e.g., http://panel.example.com:8090)"
    echo "  --registration-token TOKEN     One-time registration token (auto-registers server)"
    echo "  --token TOKEN                  Agent token (legacy mode, requires manual server creation)"
    echo "  --install-dir DIR              Install directory (default: /usr/local/bin)"
    echo "  -h, --help                     Show this help"
    exit 1
}

LEGACY_MODE=false
AGENT_TOKEN=""

while [[ $# -gt 0 ]]; do
    case $1 in
        --panel-url) PANEL_URL="$2"; shift 2;;
        --registration-token) REG_TOKEN="$2"; shift 2;;
        --token) AGENT_TOKEN="$2"; LEGACY_MODE=true; shift 2;;
        --install-dir) INSTALL_DIR="$2"; shift 2;;
        -h|--help) usage;;
        *) echo "Unknown arg: $1"; usage;;
    esac
done

# Check that this script was NOT piped from curl (env vars set by panel template)
# If PANEL_URL and REG_TOKEN are already set (from curl | bash), skip arg parsing.
# They're set at the top of the file by the template.

if [ -z "$PANEL_URL" ] || [ -z "$REG_TOKEN" ]; then
    # Not piped from panel — check manual args
    if [ -z "$PANEL_URL" ]; then
        echo "Error: --panel-url required"
        usage
    fi
    if [ -z "$REG_TOKEN" ] && [ -z "$AGENT_TOKEN" ]; then
        echo "Error: --registration-token or --token required"
        usage
    fi
fi

# If legacy mode (manual --token), we need the agent token from args
if [ "$LEGACY_MODE" = true ] && [ -z "$AGENT_TOKEN" ]; then
    echo "Error: --token requires a value"
    usage
fi

if [ "$(id -u)" -ne 0 ]; then
    echo "Error: must run as root"
    exit 1
fi

python3 - "$PANEL_URL" <<'PYURL' || { echo "Error: installer requires a clean root https:// panel URL" >&2; exit 1; }
import sys, urllib.parse
u = urllib.parse.urlsplit(sys.argv[1])
raise SystemExit(0 if u.scheme == 'https' and u.hostname and not u.username and not u.password and u.path in ('', '/') and not u.query and not u.fragment else 1)
PYURL

echo "╔══════════════════════════════════════════════════╗"
echo "║       SigardaPanel Agent Installer              ║"
echo "╚══════════════════════════════════════════════════╝"
echo ""

# ── Detect OS ──
if [ -f /etc/os-release ]; then
    . /etc/os-release
    OS=$ID
else
    echo "Error: cannot detect OS"
    exit 1
fi
echo "Detected OS: $OS"

# ── Install dependencies ──
echo "[1/7] Installing dependencies..."
case "$OS" in
    ubuntu|debian)
        apt-get update -qq
        apt-get install -y -qq nginx tar git curl jq openssl ca-certificates > /dev/null 2>&1
        apt-get install -y -qq certbot python3-certbot-nginx > /dev/null 2>&1 || echo "  certbot install failed (optional)"
        ;;
    centos|rhel|fedora|rocky|almalinux)
        yum install -y -q nginx tar git curl jq openssl ca-certificates 2>/dev/null
        yum install -y -q certbot 2>/dev/null || echo "  certbot install failed (optional)"
        ;;
    *)
        echo "Error: unsupported OS: $OS (supported: ubuntu, debian, centos, rhel, fedora, rocky, almalinux)"
        exit 1
        ;;
esac

# ── Generate or preserve agent token ──
if [ -z "$AGENT_TOKEN" ] && [ -r "$CONFIG_DIR/agent.env" ]; then
    AGENT_TOKEN=$(sed -n 's/^SIGARDAPANEL_AGENT_TOKEN=//p' "$CONFIG_DIR/agent.env" | head -n1)
fi
if [ -z "$AGENT_TOKEN" ]; then
    echo "[2/7] Generating agent token..."
    AGENT_TOKEN=$(openssl rand -hex 32)
    echo "  Agent token generated."
else
    echo "[2/7] Using provided agent token."
fi

# ── Get server info ──
echo "[3/7] Detecting server info..."
SERVER_IP=${SIGARDAPANEL_AGENT_PUBLIC_IP:-$(curl --proto '=https' --proto-redir '=https' --tlsv1.2 -fsSL --max-time 5 https://api.ipify.org 2>/dev/null || curl --proto '=https' --proto-redir '=https' --tlsv1.2 -fsSL --max-time 5 https://icanhazip.com 2>/dev/null)}
HOSTNAME=$(hostname -f 2>/dev/null || hostname)
if ! python3 - "$SERVER_IP" <<'PYIP'
import ipaddress, sys
try:
    ip = ipaddress.ip_address(sys.argv[1].strip())
except ValueError:
    raise SystemExit(1)
raise SystemExit(0 if ip.is_global else 1)
PYIP
then
    echo "Error: callback must be a global-unicast IP; set SIGARDAPANEL_AGENT_PUBLIC_IP" >&2
    exit 1
fi
echo "  Hostname: $HOSTNAME"
echo "  IP: $SERVER_IP"

# ── Download binary ──
echo "[5/7] Downloading agent binary..."
DOWNLOAD_URL="$BINARY_URL"
mkdir -p "$INSTALL_DIR"
DOWNLOAD_TARGET="$INSTALL_DIR/.sigardapanel.new"
ROLLBACK_DIR=$(mktemp -d)
ROLLBACK_ACTIVE=1
[ ! -f "$INSTALL_DIR/sigardapanel" ] || cp -a "$INSTALL_DIR/sigardapanel" "$ROLLBACK_DIR/binary"
[ ! -f "$CONFIG_DIR/agent.env" ] || cp -a "$CONFIG_DIR/agent.env" "$ROLLBACK_DIR/agent.env"
[ ! -d "$CONFIG_DIR" ] || touch "$ROLLBACK_DIR/config-dir-existed"
[ ! -f "/etc/systemd/system/$SERVICE_NAME.service" ] || cp -a "/etc/systemd/system/$SERVICE_NAME.service" "$ROLLBACK_DIR/service"
[ ! -f /etc/profile.d/sigardapanel-info.sh ] || cp -a /etc/profile.d/sigardapanel-info.sh "$ROLLBACK_DIR/dashboard"
if [ -L /usr/local/bin/sigardapanel-info ]; then readlink /usr/local/bin/sigardapanel-info > "$ROLLBACK_DIR/dashboard-link"; elif [ -e /usr/local/bin/sigardapanel-info ]; then cp -a /usr/local/bin/sigardapanel-info "$ROLLBACK_DIR/dashboard-command"; fi
rollback_agent_install() {
    rc=$?
    rm -f "$DOWNLOAD_TARGET" "${DASHBOARD_TMP:-}"
    if [ "$rc" -ne 0 ] && [ "$ROLLBACK_ACTIVE" -eq 1 ]; then
        if [ -f "$ROLLBACK_DIR/binary" ]; then cp -a "$ROLLBACK_DIR/binary" "$INSTALL_DIR/sigardapanel"; else rm -f "$INSTALL_DIR/sigardapanel"; fi
        if [ -f "$ROLLBACK_DIR/agent.env" ]; then cp -a "$ROLLBACK_DIR/agent.env" "$CONFIG_DIR/agent.env"; else rm -f "$CONFIG_DIR/agent.env"; fi
        if [ ! -f "$ROLLBACK_DIR/config-dir-existed" ]; then rmdir "$CONFIG_DIR" 2>/dev/null || true; fi
        if [ -f "$ROLLBACK_DIR/service" ]; then cp -a "$ROLLBACK_DIR/service" "/etc/systemd/system/$SERVICE_NAME.service"; else rm -f "/etc/systemd/system/$SERVICE_NAME.service"; fi
        if [ -f "$ROLLBACK_DIR/dashboard" ]; then cp -a "$ROLLBACK_DIR/dashboard" /etc/profile.d/sigardapanel-info.sh; else rm -f /etc/profile.d/sigardapanel-info.sh /usr/local/bin/sigardapanel-info; fi
        rm -f /usr/local/bin/sigardapanel-info
        if [ -f "$ROLLBACK_DIR/dashboard-link" ]; then ln -s "$(cat "$ROLLBACK_DIR/dashboard-link")" /usr/local/bin/sigardapanel-info; elif [ -f "$ROLLBACK_DIR/dashboard-command" ]; then cp -a "$ROLLBACK_DIR/dashboard-command" /usr/local/bin/sigardapanel-info; fi
        systemctl daemon-reload 2>/dev/null || true
        systemctl restart "$SERVICE_NAME" 2>/dev/null || true
    fi
    rm -rf "$ROLLBACK_DIR" "$INSTALL_DIR/.sigardapanel.previous"
    exit "$rc"
}
trap rollback_agent_install EXIT

if curl --proto '=https' --proto-redir '=https' --tlsv1.2 -fsSL "$DOWNLOAD_URL" -o "$DOWNLOAD_TARGET" 2>/dev/null; then
    printf '%s  %s\n' "$BINARY_SHA256" "$DOWNLOAD_TARGET" | sha256sum -c -
    chmod 0755 "$DOWNLOAD_TARGET"
    "$DOWNLOAD_TARGET" version >/dev/null
    if [ -f "$INSTALL_DIR/sigardapanel" ]; then
        cp -a "$INSTALL_DIR/sigardapanel" "$INSTALL_DIR/.sigardapanel.previous"
    fi
    mv -f "$DOWNLOAD_TARGET" "$INSTALL_DIR/sigardapanel"
    echo "  Binary downloaded."
else
    rm -f "$DOWNLOAD_TARGET"
    echo "Error: pinned SigardaPanel release could not be downloaded" >&2
    exit 1
fi

# ── Create config ──
echo "[6/7] Creating configuration..."
mkdir -p "$CONFIG_DIR"
mkdir -p "$SITE_ROOT"
cat > "$CONFIG_DIR/agent.env" <<ENVEOF
SIGARDAPANEL_AGENT_ADDR=:7710
SIGARDAPANEL_PANEL_URL=$PANEL_URL
SIGARDAPANEL_AGENT_TOKEN=$AGENT_TOKEN
SIGARDAPANEL_AGENT_SITE_ROOT=$SITE_ROOT
SIGARDAPANEL_AGENT_ALLOW_STUB_SSL=false
ENVEOF
chmod 600 "$CONFIG_DIR/agent.env"

# Install the SSH status dashboard from the local checkout or signed release source.
DASHBOARD_LOCAL="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/sigardapanel-info.sh"
if [ -f "$DASHBOARD_LOCAL" ]; then
    install -o root -g root -m 0755 "$DASHBOARD_LOCAL" /etc/profile.d/sigardapanel-info.sh
else
    DASHBOARD_TMP=$(mktemp)

    curl --proto '=https' --proto-redir '=https' --tlsv1.2 -fsSL --retry 3 "$PANEL_URL/api/v1/agents/status-dashboard" -o "$DASHBOARD_TMP"
    printf '%s  %s\n' '9d4650c2d78fa71bbc19fdd1001717d599f6fd7badbac6bef662ca8c6490d58c' "$DASHBOARD_TMP" | sha256sum -c -
    bash -n "$DASHBOARD_TMP"
    install -o root -g root -m 0755 "$DASHBOARD_TMP" /etc/profile.d/sigardapanel-info.sh
fi
ln -sfn /etc/profile.d/sigardapanel-info.sh /usr/local/bin/sigardapanel-info

# ── Create systemd service ──
echo "[7/7] Setting up systemd service..."
cat > "/etc/systemd/system/$SERVICE_NAME.service" <<SVCEOF
[Unit]
Description=SigardaPanel Agent
After=network.target nginx.service
Wants=nginx.service

[Service]
Type=simple
User=root
EnvironmentFile=$CONFIG_DIR/agent.env
ExecStart=$INSTALL_DIR/sigardapanel agent
Restart=always
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
SVCEOF

# ── Start service ──
systemctl daemon-reload
systemctl enable "$SERVICE_NAME" > /dev/null 2>&1
systemctl restart "$SERVICE_NAME"

sleep 2
if systemctl is-active --quiet "$SERVICE_NAME" && curl -fsS --max-time 3 http://127.0.0.1:7710/health >/dev/null; then
# ── Register with panel ──
if [ -n "$REG_TOKEN" ]; then
    echo "[4/7] Registering with panel..."
    REGISTER_RESPONSE=$(curl --proto '=https' --proto-redir '=https' --tlsv1.2 -sf -X POST "$PANEL_URL/api/v1/agents/register" \
        -H "Content-Type: application/json" \
            -d "$(jq -n --arg registration_token "$REG_TOKEN" --arg hostname "$HOSTNAME" \
                --arg ip_address "$SERVER_IP" --arg agent_token "$AGENT_TOKEN" \
                --arg os_info "$(head -n5 /etc/os-release 2>/dev/null | tr '\n' '; ')" \
                '{registration_token:$registration_token,hostname:$hostname,ip_address:$ip_address,agent_token:$agent_token,os_info:$os_info}')")

    if ! echo "$REGISTER_RESPONSE" | grep -q '"server_id"'; then
        echo "Error: registration failed"
        echo "Response: $REGISTER_RESPONSE"
        exit 1
    fi

    SERVER_ID=$(jq -er '.server_id' <<<"$REGISTER_RESPONSE")
    echo "  Registered! Server ID: $SERVER_ID"
else
    echo "[4/7] Skipping registration (legacy mode — create server manually in panel)"
fi



    echo ""
    echo "✅ Agent installed and running!"
    echo ""
    echo "  Hostname:  $HOSTNAME"
    echo "  IP:        $SERVER_IP"
    echo "  Agent:     http://$SERVER_IP:7710"
    echo "  Panel:     $PANEL_URL"
    echo ""
    echo "  Status:  systemctl status $SERVICE_NAME"
    echo "  Logs:    journalctl -u $SERVICE_NAME -f"
    echo "  Health:    curl http://localhost:7710/health"
    ROLLBACK_ACTIVE=0
    rm -rf "$ROLLBACK_DIR" "$INSTALL_DIR/.sigardapanel.previous"
else
    echo ""
    echo "❌ Agent service failed to start."
    echo "Check: journalctl -u $SERVICE_NAME -e"
    exit 1
fi
