#!/bin/bash
set -euo pipefail

INSTALL_DIR="/root/SigardaPanel"
REPO_DIR="/root/SigardaPanel-Enterprise"
CACHE_FILE="/tmp/sigardapanel_latest_version"

# Set Go environment
# HOME is required by Go for build cache; setsid strips it from detached processes
export HOME="/root"
export PATH="/usr/local/go/bin:$PATH"
export GOPATH="/root/go"
export GOMODCACHE="$GOPATH/pkg/mod"
export GOCACHE="$HOME/.cache/go-build"

echo "🔄 SigardaPanel Update"
echo ""

if [ "$EUID" -ne 0 ]; then
    echo "❌ Run as root"
    exit 1
fi

# Current version
CURRENT=$("$INSTALL_DIR/sigardapanel" version 2>/dev/null || echo "unknown")
echo "📦 Current: $CURRENT"

# Get latest version
echo "🔍 Checking latest version..."
LATEST=""

# 1. Try GitHub API
LATEST=$(curl -s --max-time 5 "https://api.github.com/repos/bayurstarcool/SigardaPanel/tags" | grep -o '"name":"[^"]*"' | head -1 | cut -d'"' -f4 2>/dev/null || echo "")

# 2. If rate limited, try cache file
if [ -z "$LATEST" ] && [ -f "$CACHE_FILE" ]; then
    LATEST=$(head -1 "$CACHE_FILE" | cut -d'|' -f1)
fi

if [ -z "$LATEST" ]; then
    echo "❌ Could not determine latest version"
    exit 1
fi

echo "📦 Latest: $LATEST"

# Compare versions
CURRENT_CLEAN="${CURRENT#v}"
LATEST_CLEAN="${LATEST#v}"
if [ "$CURRENT_CLEAN" = "$LATEST_CLEAN" ]; then
    echo "✅ Already up to date!"
    exit 0
fi

# Build from source
echo "🔨 Building from source..."
cd "$REPO_DIR"

# Rebuild web
if [ -d "web" ]; then
    echo "  → Building dashboard..."
    cd web && npm run build 2>&1 | tail -1 && cd ..
fi

# Embed dashboard
rm -rf dashboard/web && mkdir -p dashboard/web
cp -r web/build dashboard/web/build 2>/dev/null || true

# Build binary
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-X sigardapanel/internal/commands.Version=${LATEST_CLEAN}" \
    -o sigardapanel-linux-amd64 ./cmd/sigardapanel

if [ ! -f sigardapanel-linux-amd64 ]; then
    echo "❌ Build failed"
    exit 1
fi

# Backup
echo "💾 Backing up..."
cp "$INSTALL_DIR/sigardapanel" "$INSTALL_DIR/sigardapanel.bak"

# Stop
echo "⏸️  Stopping..."
systemctl stop sigardapanel-api 2>/dev/null || true
systemctl stop sigardapanel-agent 2>/dev/null || true
sleep 1

# Replace
echo "🔄 Updating..."
cp sigardapanel-linux-amd64 "$INSTALL_DIR/sigardapanel"
chmod +x "$INSTALL_DIR/sigardapanel"

# Start
echo "▶️  Starting..."
systemctl start sigardapanel-api
systemctl start sigardapanel-agent
sleep 2

# Verify
NEW=$("$INSTALL_DIR/sigardapanel" version 2>/dev/null || echo "unknown")
echo ""
echo "══════════════════════════════════════"
echo "✅ Updated: $CURRENT → $NEW"
echo "══════════════════════════════════════"
echo "API: $(systemctl is-active sigardapanel-api)"
echo "Agent: $(systemctl is-active sigardapanel-agent)"
