#!/usr/bin/env bash
set -euo pipefail

# ╔══════════════════════════════════════════════════════════╗
# ║  SigardaPanel Agent Installer                           ║
# ║                                                         ║
# ║  One-line install:                                      ║
# ║    curl -sSL http://panel:7700/api/v1/agents/install    ║
# ║      ?token=YOUR_REGISTRATION_TOKEN | bash              ║
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
DOWNLOAD_URL_BASE=""

usage() {
    echo "SigardaPanel Agent Installer"
    echo ""
    echo "Usage:"
    echo "  $0 --panel-url URL --registration-token TOKEN"
    echo "  $0 --panel-url URL --token AGENT_TOKEN  (legacy: requires manual server creation)"
    echo ""
    echo "Options:"
    echo "  --panel-url URL                Panel API URL (e.g., http://panel.example.com:7700)"
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
        apt-get install -y -qq nginx tar git curl > /dev/null 2>&1
        apt-get install -y -qq certbot python3-certbot-nginx > /dev/null 2>&1 || echo "  certbot install failed (optional)"
        ;;
    centos|rhel|fedora|rocky|almalinux)
        yum install -y -q nginx tar git curl 2>/dev/null
        yum install -y -q certbot 2>/dev/null || echo "  certbot install failed (optional)"
        ;;
    *)
        echo "Error: unsupported OS: $OS (supported: ubuntu, debian, centos, rhel, fedora, rocky, almalinux)"
        exit 1
        ;;
esac

# ── Generate agent token (if not in legacy mode) ──
if [ -z "$AGENT_TOKEN" ]; then
    echo "[2/7] Generating agent token..."
    AGENT_TOKEN=$(openssl rand -hex 32)
    echo "  Agent token generated."
else
    echo "[2/7] Using provided agent token."
fi

# ── Get server info ──
echo "[3/7] Detecting server info..."
SERVER_IP=$(curl -s ifconfig.me 2>/dev/null || curl -s icanhazip.com 2>/dev/null || hostname -I | awk '{print $1}')
HOSTNAME=$(hostname -f 2>/dev/null || hostname)
echo "  Hostname: $HOSTNAME"
echo "  IP: $SERVER_IP"

# ── Register with panel ──
if [ -n "$REG_TOKEN" ]; then
    echo "[4/7] Registering with panel..."
    REGISTER_RESPONSE=$(curl -sf -X POST "$PANEL_URL/api/v1/agents/register" \
        -H "Content-Type: application/json" \
        -d "{
            \"registration_token\": \"$REG_TOKEN\",
            \"hostname\": \"$HOSTNAME\",
            \"ip_address\": \"$SERVER_IP\",
            \"agent_token\": \"$AGENT_TOKEN\",
            \"os_info\": \"$(cat /etc/os-release 2>/dev/null | head -5 | tr '\n' '; ')\"
        }")

    if ! echo "$REGISTER_RESPONSE" | grep -q '"server_id"'; then
        echo "Error: registration failed"
        echo "Response: $REGISTER_RESPONSE"
        exit 1
    fi

    SERVER_ID=$(echo "$REGISTER_RESPONSE" | grep -o '"server_id":[0-9]*' | cut -d: -f2)
    echo "  Registered! Server ID: $SERVER_ID"
else
    echo "[4/7] Skipping registration (legacy mode — create server manually in panel)"
fi

# ── Download binary ──
echo "[5/7] Downloading agent binary..."
DOWNLOAD_URL="$PANEL_URL/api/v1/agent/download"
mkdir -p "$INSTALL_DIR"

if curl -fsSL "$DOWNLOAD_URL" -o "$INSTALL_DIR/sigardapanel" 2>/dev/null; then
    chmod +x "$INSTALL_DIR/sigardapanel"
    echo "  Binary downloaded."
else
    echo "  Direct download not available."
    if [ -f "$INSTALL_DIR/sigardapanel" ]; then
        echo "  Using existing binary at $INSTALL_DIR/sigardapanel"
    else
        echo "Error: no sigardapanel binary found at $INSTALL_DIR/sigardapanel"
        echo "Please place the binary manually before running this script."
        exit 1
    fi
fi

# ── Create config ──
echo "[6/7] Creating configuration..."
mkdir -p "$CONFIG_DIR"
mkdir -p "$SITE_ROOT"
cat > "$CONFIG_DIR/agent.env" <<ENVEOF
SIGARDAPANEL_AGENT_ADDR=:7790
SIGARDAPANEL_PANEL_URL=$PANEL_URL
SIGARDAPANEL_AGENT_TOKEN=$AGENT_TOKEN
SIGARDAPANEL_AGENT_SITE_ROOT=$SITE_ROOT
SIGARDAPANEL_AGENT_ALLOW_STUB_SSL=false
ENVEOF
chmod 600 "$CONFIG_DIR/agent.env"

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
systemctl start "$SERVICE_NAME"

sleep 2
if systemctl is-active --quiet "$SERVICE_NAME"; then
    echo ""
    echo "✅ Agent installed and running!"
    echo ""
    echo "  Hostname:  $HOSTNAME"
    echo "  IP:        $SERVER_IP"
    echo "  Agent:     http://$SERVER_IP:7790"
    echo "  Panel:     $PANEL_URL"
    echo ""
    echo "  Status:  systemctl status $SERVICE_NAME"
    echo "  Logs:    journalctl -u $SERVICE_NAME -f"
    echo "  Health:  curl http://localhost:7790/health"
else
    echo ""
    echo "❌ Agent service failed to start."
    echo "Check: journalctl -u $SERVICE_NAME -e"
    exit 1
fi
