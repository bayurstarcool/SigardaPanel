# Installation Guide

This guide covers installing and running SigardaPanel.

## Quick Start

### 1. Download

```bash
curl -sSL https://github.com/bayurstarcool/SigardaPanel/releases/latest/download/sigardapanel-linux-amd64 -o sigardapanel
chmod +x sigardapanel
sudo mv sigardapanel /usr/local/bin/
```

### 2. Initialize

```bash
sigardapanel init --api-url https://panel.yourdomain.com
```

### 3. Run Services

**Panel Server:**

```bash
export SIGARDAPANEL_DB_PATH=/opt/sigardapanel/data/sigardapanel.db
export SIGARDAPANEL_API_ADDR=:8090

sigardapanel api
```

**Agent (Target VPS):**

```bash
export SIGARDAPANEL_AGENT_ADDR=:9090
export SIGARDAPANEL_AGENT_TOKEN=your_token

sigardapanel agent
```

## Automated Installation

### Panel Server

```bash
PANEL_DOMAIN=panel.yourdomain.com bash <(curl -sSL https://raw.githubusercontent.com/bayurstarcool/SigardaPanel/main/deploy/install-panel.sh)
```

### Agent

```bash
curl -sSL http://panel.yourdomain.com:8090/api/v1/agents/install?token=YOUR_TOKEN | bash
```

Or manual:

```bash
bash install-agent.sh --panel-url http://panel.yourdomain.com --token YOUR_TOKEN
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `SIGARDAPANEL_API_ADDR` | `:8090` | API server address |
| `SIGARDAPANEL_AGENT_ADDR` | `:9090` | Agent address |
| `SIGARDAPANEL_DB_PATH` | `sigardapanel.db` | Database file path |
| `SIGARDAPANEL_API_URL` | `http://localhost:8090` | Panel API URL |
| `SIGARDAPANEL_TOKEN` | - | API token from login |
| `SIGARDAPANEL_AGENT_TOKEN` | - | Agent authentication token |
| `SIGARDAPANEL_OUTPUT` | `json` | Output format: `json`, `table`, `yaml` |

## Systemd Services

The installers create systemd services automatically. Manual setup:

**Panel:**

```bash
sudo systemctl start sigardapanel-panel
sudo systemctl enable sigardapanel-panel
```

**Agent:**

```bash
sudo systemctl start sigardapanel-agent
sudo systemctl enable sigardapanel-agent
```

## Verify Installation

```bash
# Check system health
sigardapanel doctor

# Check API health
curl http://localhost:8090/api/v1/health

# Check agent health
curl http://localhost:9090/health
```

## Port Reference

| Service | Default Port | Description |
|---------|--------------|-------------|
| API | `:8090` | Panel API server |
| Agent | `:9090` | Agent service |
| Web UI | `:3000` | Dashboard (Enterprise) |

## Troubleshooting

### Agent Not Connecting

1. Verify agent token matches panel configuration
2. Check network connectivity between panel and agent
3. Verify agent service is running: `systemctl status sigardapanel-agent`
4. Check agent logs: `journalctl -u sigardapanel-agent -f`

### SSL Issues

1. Verify domain DNS points to server
2. Check port 80 is accessible for ACME challenge
3. Run: `sigardapanel ssl status --domain yourdomain.com`

### Database Issues

1. Verify database file exists and is writable
2. Check WAL mode is enabled
3. Run: `sigardapanel doctor`
