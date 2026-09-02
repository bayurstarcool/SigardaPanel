package apihttp

import (
	"crypto/rand"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/audit"
	"sigardapanel/internal/auth"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/store"
)

type agentHandler struct {
	store *store.Store
	audit *audit.Recorder
}

//go:embed assets/sigardapanel-info.sh
var statusDashboardScript string

func newAgentHandler(s *store.Store, a *audit.Recorder) *agentHandler {
	return &agentHandler{store: s, audit: a}
}

// --- Admin: Generate Registration Token ---

// createRegistrationToken generates a one-time registration token.
// POST /api/v1/agents/tokens  (admin only)
func (h *agentHandler) createRegistrationToken(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	var userID int64
	if sess != nil {
		userID = sess.UserID
	}

	rawToken, err := auth.NewToken()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to generate token")
	}

	tokenHash := auth.HashToken(rawToken)
	expiresAt := time.Now().Add(1 * time.Hour)

	rt := &store.RegistrationToken{
		TokenHash: tokenHash,
		CreatedBy: sql.NullInt64{Int64: userID, Valid: userID > 0},
		ExpiresAt: expiresAt,
	}

	if err := h.store.RegistrationTokens.Create(c.Request().Context(), rt); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create registration token")
	}

	h.audit.RecordAction(c.Request().Context(), userID, "registration_token_create", "registration_token", rt.ID, map[string]any{
		"expires_at": expiresAt,
	})

	return c.JSON(http.StatusCreated, httpjson.Success{
		Data: map[string]any{
			"token":      rawToken,
			"token_id":   rt.ID,
			"expires_at": expiresAt,
		},
		RequestID: rid,
	})
}

// --- Public: Agent Auto-Register ---

type registerRequest struct {
	RegistrationToken string `json:"registration_token"`
	Hostname          string `json:"hostname"`
	IPAddress         string `json:"ip_address,omitempty"`
	AgentToken        string `json:"agent_token"`
	OSInfo            string `json:"os_info,omitempty"`
}

// registerAgent creates a server record from a registration token.
// POST /api/v1/agents/register  (no auth — uses registration token)
func (h *agentHandler) registerAgent(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)

	var req registerRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	req.RegistrationToken = strings.TrimSpace(req.RegistrationToken)
	req.Hostname = strings.TrimSpace(req.Hostname)
	req.AgentToken = strings.TrimSpace(req.AgentToken)
	req.IPAddress = strings.TrimSpace(req.IPAddress)

	if req.RegistrationToken == "" || req.Hostname == "" || req.AgentToken == "" || req.IPAddress == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "registration_token, hostname, agent_token, and ip_address are required")
	}

	// Validate and consume registration token
	tokenHash := auth.HashToken(req.RegistrationToken)
	rt, err := h.store.RegistrationTokens.GetByHash(c.Request().Context(), tokenHash)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if rt == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid registration token")
	}
	if rt.UsedAt.Valid {
		return echo.NewHTTPError(http.StatusConflict, "registration token already used")
	}
	if time.Now().After(rt.ExpiresAt) {
		return echo.NewHTTPError(http.StatusGone, "registration token expired")
	}

	// Prefer the explicitly reported, validated address. Do not derive the
	// privileged callback target from spoofable forwarded-client headers.
	callbackIP := net.ParseIP(req.IPAddress)
	if !isAllowedAgentCallbackIP(callbackIP) {
		return echo.NewHTTPError(http.StatusBadRequest, "ip_address must be a global-unicast callback address")
	}
	agentHost := callbackIP.String()
	agentURL := "http://" + net.JoinHostPort(agentHost, "7710")

	server := &store.Server{
		Name:       req.Hostname,
		Hostname:   req.Hostname,
		Status:     "online",
		AgentToken: req.AgentToken,
		AgentURL:   sql.NullString{String: agentURL, Valid: true},
		SSHPort:    22,
	}
	if req.IPAddress != "" && net.ParseIP(req.IPAddress) != nil {
		server.IPAddress = sql.NullString{String: req.IPAddress, Valid: true}
	}

	if err := h.store.ConsumeRegistrationTokenAndCreateServer(c.Request().Context(), rt.ID, server); err != nil {
		if strings.Contains(err.Error(), "already used") || strings.Contains(err.Error(), "expired") {
			return echo.NewHTTPError(http.StatusConflict, "registration token already used or expired")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create server record")
	}

	h.audit.RecordAction(c.Request().Context(), 0, "server_register", "server", server.ID, map[string]any{
		"hostname": req.Hostname,
		"ip":       req.IPAddress,
		"os_info":  req.OSInfo,
		"token_id": rt.ID,
	})

	return c.JSON(http.StatusCreated, httpjson.Success{
		Data: map[string]any{
			"server_id": server.ID,
			"agent_url": agentURL,
			"status":    server.Status,
		},
		RequestID: rid,
	})
}

func isAllowedAgentCallbackIP(ip net.IP) bool {
	return ip != nil && ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast()
}

// --- Public: Serve Install Script ---

// installScript serves the one-line install script. The registration token is
// carried in a header so it does not leak into URLs, proxy logs, or history.
func (h *agentHandler) installScript(c echo.Context) error {
	token := strings.TrimSpace(c.Request().Header.Get("X-Sigarda-Registration-Token"))
	if token == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "X-Sigarda-Registration-Token header required")
	}

	panelURL := strings.TrimSpace(os.Getenv("SIGARDAPANEL_PUBLIC_URL"))
	parsed, err := url.Parse(panelURL)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "SIGARDAPANEL_PUBLIC_URL must be a clean https URL")
	}
	panelURL = strings.TrimRight(parsed.String(), "/")

	script := fmt.Sprintf(installScriptTemplate, strconv.Quote(panelURL), strconv.Quote(token))

	c.Response().Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	c.Response().Header().Set("Content-Disposition", "attachment; filename=\"install-agent.sh\"")
	return c.String(http.StatusOK, script)
}

// statusDashboard serves the version-matched SSH dashboard used by agent installers.
func (h *agentHandler) statusDashboard(c echo.Context) error {
	c.Response().Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	c.Response().Header().Set("Content-Disposition", "attachment; filename=\"sigardapanel-info.sh\"")
	return c.String(http.StatusOK, statusDashboardScript)
}

// installScriptTemplate is the bash install script with two %s placeholders:
// %1 = PANEL_URL, %2 = REGISTRATION_TOKEN
const installScriptTemplate = `#!/usr/bin/env bash
set -euo pipefail

# ── Configuration (set by panel) ──
PANEL_URL=%s
REG_TOKEN=%s
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/sigardapanel"
SITE_ROOT="${SIGARDAPANEL_AGENT_SITE_ROOT:-/var/www}"
SERVICE_NAME="sigardapanel-agent"

echo "╔══════════════════════════════════════════════════╗"
echo "║       SigardaPanel Agent Installer              ║"
echo "╚══════════════════════════════════════════════════╝"
echo ""

# ── Root check ──
if [ "$(id -u)" -ne 0 ]; then
    echo "Error: must run as root"
    exit 1
fi

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

# ── Generate agent token ──
echo "[2/7] Generating agent token..."
AGENT_TOKEN=$(openssl rand -hex 32)
echo "  Agent token generated."

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
DOWNLOAD_URL="${SIGARDAPANEL_BINARY_URL:-https://github.com/bayurstarcool/SigardaPanel/releases/download/${SIGARDAPANEL_VERSION:-v0.5.5}/sigardapanel}"
BINARY_SHA256="${SIGARDAPANEL_BINARY_SHA256:-a4e0f9b10671cd465d1e66248b6512557dc497bf7b5d5d6aecdb6739cf7bf198}"
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

if curl --proto '=https' --proto-redir '=https' --tlsv1.2 -fsSL --retry 3 "$DOWNLOAD_URL" -o "$DOWNLOAD_TARGET" 2>/dev/null; then
    printf '%%s  %%s\n' "$BINARY_SHA256" "$DOWNLOAD_TARGET" | sha256sum -c -
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

# ── Install SSH status dashboard ──
DASHBOARD_TMP=$(mktemp)

curl --proto '=https' --proto-redir '=https' --tlsv1.2 -fsSL --retry 3 "$PANEL_URL/api/v1/agents/status-dashboard" -o "$DASHBOARD_TMP"
printf '%%s  %%s\n' '9d4650c2d78fa71bbc19fdd1001717d599f6fd7badbac6bef662ca8c6490d58c' "$DASHBOARD_TMP" | sha256sum -c -
bash -n "$DASHBOARD_TMP"
install -o root -g root -m 0755 "$DASHBOARD_TMP" /etc/profile.d/sigardapanel-info.sh
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



    echo ""
    echo "✅ Agent installed and running!"
    echo ""
    echo "  Server ID:   $SERVER_ID"
    echo "  Agent URL:   http://$SERVER_IP:7710"
    echo "  Panel URL:   $PANEL_URL"
    echo ""
    echo "  Status:  systemctl status $SERVICE_NAME"
    echo "  Logs:    journalctl -u $SERVICE_NAME -f"
    echo "  Health:  curl http://localhost:7710/health"
    ROLLBACK_ACTIVE=0
    rm -rf "$ROLLBACK_DIR" "$INSTALL_DIR/.sigardapanel.previous"
else
    echo ""
    echo "❌ Agent service failed to start."
    echo "Check: journalctl -u $SERVICE_NAME -e"
    exit 1
fi
`

// generateRandomHex generates a random hex string of the given byte length.
func generateRandomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
