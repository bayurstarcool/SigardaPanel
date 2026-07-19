# SigardaPanel — VPS Management Panel

Panel manajemen VPS open-source. Satu binary untuk panel server + agent.

## Quick Install

### Panel Server (first-time setup)

```bash
# 1. Download binary
curl -sSL https://github.com/bayurstarcool/SigardaPanel/releases/download/v0.5.6/sigardapanel-linux-amd64 -o /usr/local/bin/sigardapanel
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
sigardapanel api       # API server :7700
sigardapanel agent     # Agent service :7710

# Frontend (SvelteKit)
cd web && npx vite dev --host 0.0.0.0 --port 7720
```

### Add Agent on Another VPS

```bash
# 1. Download binary
curl -sSL https://github.com/bayurstarcool/SigardaPanel/releases/download/v0.5.6/sigardapanel-linux-amd64 -o /usr/local/bin/sigardapanel
chmod +x /usr/local/bin/sigardapanel

# 2. Add server in panel UI → copy agent token

# 3. Start agent
SIGARDAPANEL_AGENT_TOKEN=<token> sigardapanel agent
```

## Services

| Service  | Port | Desc |
|----------|------|------|
| API      | 7700 | Panel REST API |
| Agent    | 7710 | Agent per VPS |
| Frontend | 7720 | SvelteKit web dashboard |

## Login

Default: `admin` / `admin123` (change after first login)

## CLI Commands

```
sigardapanel
├── api / agent / dev / install / init / login / logout / doctor / version
├── server    add|list|update|remove|doctor
├── site      create|list|update|delete|deploy|config|setup-logrotate
├── ssl       status|issue|renew|renew-all
├── job       list|watch|cancel|logs
├── backup    create|list|delete|restore
├── db        create|list|delete
│   └── user  create|list|rotate-password|delete
├── docker    ps|start|stop|restart|rm|logs|images|compose
├── git       branches|log|rollback|checkout
├── firewall  status|enable|disable|rules|reset
├── channels  add|list|remove
├── alerts    list
```

## GitHub Releases

https://github.com/bayurstarcool/SigardaPanel/releases

Latest: v0.5.6 — Docker Multi-Language, Auto Token Sync, Progressive Create Flow

## ENV

| Variable | Default | Desc |
|----------|---------|------|
| SIGARDAPANEL_API_ADDR | :7700 | API listen |
| SIGARDAPANEL_AGENT_ADDR | :7710 | Agent listen |
| SIGARDAPANEL_DB_PATH | sigardapanel.db | DB path |
| SIGARDAPANEL_AGENT_TOKEN | - | Agent auth |
| SIGARDAPANEL_DEV | false | Dev mode |
