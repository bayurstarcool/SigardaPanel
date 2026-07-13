#!/usr/bin/env bash
set -euo pipefail

# ╔══════════════════════════════════════════════════════════╗
# ║  SigardaPanel Agent Installer                           ║
# ║                                                         ║
# ║  One-line install:                                      ║
# ║    curl -sSL https://raw.githubusercontent.com/...      ║
# ║      .../install-agent.sh | sudo bash -s --             ║
# ║      --panel-url https://panel.example.com              ║
# ║      --registration-token YOUR_TOKEN                    ║
# ║                                                         ║
# ║  Manual install:                                        ║
# ║    bash install-agent.sh --panel-url URL --token TOKEN  ║
# ╚══════════════════════════════════════════════════════════╝

# ── Configuration ──────────────────────────────────────────
PANEL_URL=""
REG_TOKEN=""
AGENT_TOKEN=""
LEGACY_MODE=false
INSTALL_DIR="${SIGARDAPANEL_INSTALL_DIR:-/usr/local/bin}"
CONFIG_DIR="${SIGARDAPANEL_CONFIG_DIR:-/etc/sigardapanel}"
SITE_ROOT="${SIGARDAPANEL_AGENT_SITE_ROOT:-/var/www}"
SERVICE_NAME="sigardapanel-agent"
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
SigardaPanel Agent Installer v${VERSION}

Usage:
  $0 --panel-url URL --registration-token TOKEN
  $0 --panel-url URL --token AGENT_TOKEN  (legacy mode)

Options:
  --panel-url URL                Panel API URL (required)
  --registration-token TOKEN     One-time registration token (auto-register)
  --token TOKEN                  Agent token (legacy: manual server creation)
  --install-dir DIR              Install directory (default: /usr/local/bin)
  -h, --help                     Show this help

Examples:
  # Auto-register with panel
  $0 --panel-url https://panel.example.com --registration-token abc123

  # Legacy mode (manual server creation in panel)
  $0 --panel-url https://panel.example.com --token agent_token_here
EOF
    exit 1
}

# ── Parse arguments ────────────────────────────────────────
while [[ $# -gt 0 ]]; do
    case $1 in
        --panel-url) PANEL_URL="$2"; shift 2;;
        --registration-token) REG_TOKEN="$2"; shift 2;;
        --token) AGENT_TOKEN="$2"; LEGACY_MODE=true; shift 2;;
        --install-dir) INSTALL_DIR="$2"; shift 2;;
        -h|--help) usage;;
        *) log_error "Unknown argument: $1"; usage;;
    esac
done

# ── Validate ───────────────────────────────────────────────
if [ -z "$PANEL_URL" ]; then
    log_error "--panel-url is required"
    echo ""
    usage
fi

if [ -z "$REG_TOKEN" ] && [ -z "$AGENT_TOKEN" ]; then
    log_error "--registration-token or --token is required"
    echo ""
    usage
fi

if [ "$(id -u)" -ne 0 ]; then
    log_error "This installer must be run as root"
    echo "  Try: sudo $0 --panel-url $PANEL_URL --registration-token ..."
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
echo -e "${BLUE}║       SigardaPanel Agent Installer v${VERSION}        ║${NC}"
echo -e "${BLUE}╚══════════════════════════════════════════════════╝${NC}"
echo ""
log_info "Panel:     $PANEL_URL"
log_info "OS:        $OS $OS_VERSION"
log_info "Install:   $INSTALL_DIR"
echo ""

# ── Step 1: Install dependencies ───────────────────────────
log_info "[1/7] Installing dependencies..."
case "$OS" in
    ubuntu|debian)
        apt-get update -qq
        apt-get install -y -qq nginx tar git curl > /dev/null 2>&1
        ;;
    centos|rhel|fedora|rocky|almalinux)
        yum install -y -q nginx tar git curl
        ;;
    *)
        log_error "Unsupported OS: $OS"
        echo "  Supported: ubuntu, debian, centos, rhel, fedora, rocky, almalinux"
        exit 1
        ;;
esac
log_ok "Dependencies installed"

# ── Step 2: Generate agent token ───────────────────────────
if [ -z "$AGENT_TOKEN" ]; then
    log_info "[2/7] Generating agent token..."
    AGENT_TOKEN=$(openssl rand -hex 32)
    log_ok "Agent token generated"
else
    log_info "[2/7] Using provided agent token..."
    log_ok "Agent token set"
fi

# ── Step 3: Detect server info ─────────────────────────────
log_info "[3/7] Detecting server info..."
SERVER_IP=$(curl -s --max-time 5 ifconfig.me 2>/dev/null || \
            curl -s --max-time 5 icanhazip.com 2>/dev/null || \
            hostname -I | awk '{print $1}')
HOSTNAME=$(hostname -f 2>/dev/null || hostname)
log_ok "Hostname: $HOSTNAME"
log_ok "IP:       $SERVER_IP"

# ── Step 4: Register with panel ────────────────────────────
if [ -n "$REG_TOKEN" ]; then
    log_info "[4/7] Registering with panel..."
    REGISTER_RESPONSE=$(curl -sf --max-time 30 -X POST "$PANEL_URL/api/v1/agents/register" \
        -H "Content-Type: application/json" \
        -d "{
            \"registration_token\": \"$REG_TOKEN\",
            \"hostname\": \"$HOSTNAME\",
            \"ip_address\": \"$SERVER_IP\",
            \"agent_token\": \"$AGENT_TOKEN\",
            \"os_info\": \"$(cat /etc/os-release 2>/dev/null | head -5 | tr '\n' '; ')\"
        }" 2>/dev/null || echo "")

    if echo "$REGISTER_RESPONSE" | grep -q '"server_id"'; then
        SERVER_ID=$(echo "$REGISTER_RESPONSE" | grep -o '"server_id":[0-9]*' | cut -d: -f2)
        log_ok "Registered! Server ID: $SERVER_ID"
    else
        log_error "Registration failed"
        echo "  Response: $REGISTER_RESPONSE"
        echo "  Check panel URL and registration token."
        exit 1
    fi
else
    log_info "[4/7] Skipping registration (legacy mode)"
    log_warn "Create server manually in panel with token: $AGENT_TOKEN"
fi

# ── Step 5: Download binary ────────────────────────────────
log_info "[5/7] Downloading agent binary..."
mkdir -p "$INSTALL_DIR"

DOWNLOAD_URL="$PANEL_URL/api/v1/agent/download"
if curl -fsSL --max-time 60 "$DOWNLOAD_URL" -o "$INSTALL_DIR/sigardapanel" 2>/dev/null; then
    chmod +x "$INSTALL_DIR/sigardapanel"
    log_ok "Binary downloaded"
elif [ -f "$INSTALL_DIR/sigardapanel" ]; then
    log_warn "Using existing binary at $INSTALL_DIR/sigardapanel"
else
    log_error "No sigardapanel binary found"
    echo "  Download from: https://github.com/bayurstarcool/SigardaPanel/releases"
    echo "  Place at: $INSTALL_DIR/sigardapanel"
    exit 1
fi

# ── Step 6: Create configuration ───────────────────────────
log_info "[6/7] Creating configuration..."
mkdir -p "$CONFIG_DIR" "$SITE_ROOT"

cat > "$CONFIG_DIR/agent.env" <<EOF
SIGARDAPANEL_AGENT_ADDR=:7710
SIGARDAPANEL_PANEL_URL=$PANEL_URL
SIGARDAPANEL_AGENT_TOKEN=$AGENT_TOKEN
SIGARDAPANEL_AGENT_SITE_ROOT=$SITE_ROOT
SIGARDAPANEL_AGENT_ALLOW_STUB_SSL=false
EOF
chmod 600 "$CONFIG_DIR/agent.env"
log_ok "Configuration written to $CONFIG_DIR/agent.env"

# ── Step 7: Create systemd service ─────────────────────────
log_info "[7/7] Creating systemd service..."
cat > "/etc/systemd/system/$SERVICE_NAME.service" <<EOF
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
EOF

systemctl daemon-reload
systemctl enable "$SERVICE_NAME" > /dev/null 2>&1
systemctl start "$SERVICE_NAME"
log_ok "Systemd service created and started"

# ── Verify ─────────────────────────────────────────────────
sleep 2
echo ""
if systemctl is-active --quiet "$SERVICE_NAME"; then
    echo -e "${GREEN}╔══════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║       Agent Installed Successfully!              ║${NC}"
    echo -e "${GREEN}╚══════════════════════════════════════════════════╝${NC}"
    echo ""
    echo -e "  Hostname:  ${BLUE}$HOSTNAME${NC}"
    echo -e "  IP:        ${BLUE}$SERVER_IP${NC}"
    echo -e "  Agent:     ${GREEN}http://$SERVER_IP:7710${NC}"
    echo -e "  Panel:     ${GREEN}$PANEL_URL${NC}"
    echo ""
    echo -e "  ${YELLOW}Agent Token:${NC}"
    echo -e "  ${BLUE}$AGENT_TOKEN${NC}"
    echo ""
    echo "  Status:  systemctl status $SERVICE_NAME"
    echo "  Logs:    journalctl -u $SERVICE_NAME -f"
    echo "  Health:  curl http://localhost:7710/health"
    echo ""
else
    echo -e "${RED}╔══════════════════════════════════════════════════╗${NC}"
    echo -e "${RED}║       Agent Installation Failed                 ║${NC}"
    echo -e "${RED}╚══════════════════════════════════════════════════╝${NC}"
    echo ""
    echo "  Check logs: journalctl -u $SERVICE_NAME -e"
    echo "  Check config: cat $CONFIG_DIR/agent.env"
    echo ""
    exit 1
fi
