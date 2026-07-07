#!/bin/bash
set -euo pipefail

echo "🔄 SigardaPanel Local Update"
echo ""

# Check root
if [ "$EUID" -ne 0 ]; then
    echo "❌ Run as root"
    exit 1
fi

# Build from source if available
if [ -d "/root/SigardaPanel-Enterprise" ]; then
    echo "📦 Building from source..."
    cd /root/SigardaPanel-Enterprise
    
    # Build web
    if [ -d "web" ] && [ -f "web/package.json" ]; then
        echo "  → Building dashboard..."
        cd web && npm run build && cd ..
    fi
    
    # Copy web/build to dashboard for embed
    rm -rf dashboard/web
    mkdir -p dashboard/web
    cp -r web/build dashboard/web/build 2>/dev/null || true
    
    # Build binary
    echo "  → Building binary..."
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-X sigardapanel/internal/commands.Version=0.2.0" -o sigardapanel-linux-amd64 ./cmd/sigardapanel
    
    BINARY="sigardapanel-linux-amd64"
else
    echo "⬇️  Downloading latest..."
    curl -sSL https://github.com/bayurstarcool/SigardaPanel/releases/latest/download/sigardapanel-linux-amd64 -o sigardapanel-linux-amd64
    chmod +x sigardapanel-linux-amd64
    BINARY="sigardapanel-linux-amd64"
fi

# Backup
echo "💾 Backing up..."
cp /root/SigardaPanel/sigardapanel /root/SigardaPanel/sigardapanel.bak

# Stop
echo "⏸️  Stopping..."
systemctl stop sigardapanel-api
systemctl stop sigardapanel-agent
sleep 1

# Replace
echo "🔄 Updating..."
cp /root/SigardaPanel-Enterprise/$BINARY /root/SigardaPanel/sigardapanel
chmod +x /root/SigardaPanel/sigardapanel

# Start
echo "▶️  Starting..."
systemctl start sigardapanel-api
systemctl start sigardapanel-agent
sleep 2

# Verify
echo ""
echo "✅ Done! Version: $(/root/SigardaPanel/sigardapanel version)"
echo "   API: $(systemctl is-active sigardapanel-api)"
echo "   Agent: $(systemctl is-active sigardapanel-agent)"
