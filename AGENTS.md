# SigardaPanel — VPS Management Panel

Panel manajemen VPS open-source. Satu binary untuk panel server + agent.

## Quick Install

### Panel Server (first-time setup)

```bash
# 1. Download binary
curl -sSL https://github.com/bayurstarcool/SigardaPanel/releases/download/v0.4.0/sigardapanel -o /usr/local/bin/sigardapanel
chmod +x /usr/local/bin/sigardapanel

# 2. Run setup wizard
sigardapanel install
```

Wizard akan:
1. Siapkan database
2. Buat admin user (interactive)
3. Tampilkan cara start panel & agent

### Start Panel

```bash
# Dev mode (API + Agent lokal)
sigardapanel dev

# Atau terpisah
sigardapanel api      # API server :8080
sigardapanel agent     # Agent service :9090

# Frontend (SvelteKit)
cd web && npx vite dev --host 0.0.0.0 --port 4000
```

### Add Agent on Another VPS

```bash
# 1. Download binary
curl -sSL https://github.com/bayurstarcool/SigardaPanel/releases/download/v0.4.0/sigardapanel -o /usr/local/bin/sigardapanel
chmod +x /usr/local/bin/sigardapanel

# 2. Add server in panel UI → copy agent token

# 3. Start agent
SIGARDAPANEL_AGENT_TOKEN=<token> sigardapanel agent
```

## Services

| Service  | Port | Desc |
|----------|------|------|
| API      | 8080 | Panel REST API |
| Agent    | 9090 | Agent per VPS |
| Frontend | 4000 | Web dashboard |

## Login

Default: `admin` / (password from wizard)

## CLI Commands

```
sigardapanel install    Setup wizard (recommended for new installs)
sigardapanel init       Create admin user
sigardapanel dev        Run API + agent locally
sigardapanel api        Run API server
sigardapanel agent      Run agent service
sigardapanel login      Save API token
sigardapanel server     Manage servers
sigardapanel site       Manage sites
```

## GitHub Releases

https://github.com/bayurstarcool/SigardaPanel/releases

Latest: v0.4.0 — includes setup wizard, metrics, server auto-promotion

## ENV

| Variable | Default | Desc |
|----------|---------|------|
| SIGARDAPANEL_API_ADDR | :8080 | API listen |
| SIGARDAPANEL_AGENT_ADDR | :9090 | Agent listen |
| SIGARDAPANEL_DB_PATH | sigardapanel.db | DB path |
| SIGARDAPANEL_AGENT_TOKEN | - | Agent auth |
