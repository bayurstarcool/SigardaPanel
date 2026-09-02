# Installation Guide

## Prerequisites

- **OS:** Ubuntu 24.04 LTS (fresh or minimal install)
- **Access:** root (`sudo bash` or `ssh root@...`)
- **Network:** open ports 22, 80, 443
- **Resources:** 1 CPU, 1 GB RAM, 10 GB disk minimum

## One-shot Install (Recommended)

```bash
git clone https://github.com/yourorg/sigardapanel.git /opt/sigardapanel/source
cd /opt/sigardapanel/source
sudo bash scripts/install.sh
```

The script will:
1. Install system dependencies (Go 1.25, nginx, nodejs, MariaDB, Redis, etc.)
2. Build the Go binary + SvelteKit frontend
3. Create `sigardapanel` user + directories
4. Generate secrets + environment files
5. Install systemd units
6. Initialize the database with admin user
7. Configure nginx + UFW firewall
8. Start all services
9. Print URL + admin password

Duration: ~5 minutes on a fresh server.

### Environment Variables (optional)

```bash
DOMAIN=panel.example.com \
CF_TOKEN=your-cloudflare-api-token \
ADMIN_USER=admin \
ADMIN_EMAIL=admin@example.com \
ADMIN_PASS=YourStrongPassword123 \
sudo -E bash scripts/install.sh
```

| Var | Default | Notes |
|---|---|---|
| `DOMAIN` | (empty) | FQDN for Let's Encrypt |
| `CF_TOKEN` | (empty) | Cloudflare API token (DNS-01) |
| `ADMIN_USER` | `admin` | Initial admin username |
| `ADMIN_EMAIL` | `admin@localhost` | Initial admin email |
| `ADMIN_PASS` | random 16 chars | Initial admin password |
| `INSTALL_NGINX` | `yes` | Set `no` if you bring your own reverse proxy |
| `INSTALL_FIREWALL` | `yes` | UFW auto-config |
| `SOURCE_DIR` | `/opt/sigardapanel/source` | Where the source code is |

## Step-by-Step Install (Manual)

If you need fine-grained control:

### 1. Install dependencies
```bash
apt-get update
apt-get install -y curl git nginx nodejs npm certbot \
  python3-certbot-dns-cloudflare mariadb-server redis-server \
  docker.io docker-compose-v2 ufw fail2ban openssl

# Go 1.25
curl -sL https://go.dev/dl/go1.25.0.linux-amd64.tar.gz | tar -C /usr/local -xz
echo 'export PATH=$PATH:/usr/local/go/bin' > /etc/profile.d/go.sh
```

### 2. Build
```bash
cd /opt/sigardapanel/source
go build -trimpath -ldflags "-s -w" -o /usr/local/bin/sigardapanel ./cmd/sigardapanel
cd web && npm ci && npm run build
mkdir -p /opt/sigardapanel/web
cp -r build/* /opt/sigardapanel/web/
echo '{"type":"module"}' > /opt/sigardapanel/web/package.json
```

### 3. User + directories
```bash
useradd -r -s /usr/sbin/nologin -d /opt/sigardapanel sigardapanel
mkdir -p /opt/sigardapanel/{web,backups} /var/lib/sigardapanel /var/log/sigardapanel /etc/sigardapanel
chown -R sigardapanel:sigardapanel /opt/sigardapanel /var/lib/sigardapanel /var/log/sigardapanel
```

### 4. Generate secrets
```bash
openssl rand -hex 32  # agent token
openssl rand -hex 32  # encryption key
```

Write `/etc/sigardapanel/agent.env` and `panel.env` (see [SYSTEMD.md](SYSTEMD.md)).
`chmod 600` both.

### 5. Systemd units
Install the three services (see [SYSTEMD.md](SYSTEMD.md)) and start:
```bash
systemctl daemon-reload
systemctl enable --now sigardapanel-api sigardapanel-agent sigardapanel-web
```

### 6. Initialize admin
```bash
sigardapanel init --email admin@example.com --password 'YourStrongPassword123'
```

### 7. Nginx
Copy the `sigardapanel` server block from `scripts/install.sh` to `/etc/nginx/sites-enabled/`, then `nginx -t && systemctl reload nginx`.

### 8. Firewall
```bash
ufw default deny incoming
ufw allow 22,80,443/tcp
ufw enable
```

---

## Verifying Install

```bash
# Services
systemctl status sigardapanel-{api,agent,web}

# Health
curl -s http://127.0.0.1:7700/api/v1/health | jq

# Doctor
sigardapanel doctor --api-url http://127.0.0.1:7700
```

## Updating

```bash
cd /opt/sigardapanel/source
git pull
go build -o /usr/local/bin/sigardapanel ./cmd/sigardapanel
cd web && npm ci && npm run build
cp -r build/* /opt/sigardapanel/web/
systemctl restart sigardapanel-api sigardapanel-agent sigardapanel-web
```

## Uninstalling

```bash
systemctl disable --now sigardapanel-{api,agent,web}
rm /etc/systemd/system/sigardapanel-*.service
rm -rf /opt/sigardapanel /var/lib/sigardapanel /var/log/sigardapanel /etc/sigardapanel
userdel sigardapanel
```

## Troubleshooting

### Service won't start
```bash
journalctl -u sigardapanel-api -n 30
```

### DB locked
```bash
fuser /var/lib/sigardapanel/sigardapanel.db
```

### Token mismatch
```bash
sigardapanel server token --id 1
systemctl restart sigardapanel-api
```

### Port already in use
```bash
ss -tlnp | grep -E ':(7700|7710|7720)'
```
