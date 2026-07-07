# Agent Instructions

Instructions for AI agents working on the **SigardaPanel** repository.

## Language & Style

- Use English for all documentation and code comments
- Keep responses concise and technical
- Do not add large features without confirmation
- Do not commit or create branches unless explicitly asked

## Repository

### Public — this repo (`github.com/bayurstarcool/SigardaPanel`)

This is a **binary-only** distribution repository. It contains:

- `sigardapanel` — compiled binary (Linux amd64, 25MB)
- `sigardapanel-darwin-amd64` — macOS Intel binary (25MB)
- `sigardapanel-darwin-arm64` — macOS Apple Silicon binary (24MB)
- `deploy/` — installer scripts
- `docs/` — documentation
- `README.md` — project overview
- `LICENSE` — MIT License

**No source code** — all Go source lives in the Enterprise repo.

### Private — Enterprise repo (`github.com/bayurstarcool/SigardaPanel-Enterprise`)

Full source code for panel API, dashboard, job worker, CLI, and agent:

- `internal/apihttp/` — Panel API handlers
- `internal/backendserver/` — API server runner (serves embedded dashboard)
- `internal/cli/` — CLI helpers
- `internal/commands/` — CLI commands
- `internal/jobs/` — Job worker and scheduler
- `internal/agenthttp/` — Agent task handlers
- `internal/agentserver/` — Agent HTTP server
- `internal/agentclient/` — Agent client
- `internal/auth/` — Authentication and RBAC
- `internal/store/` — SQLite data store
- `internal/nginx/` — Nginx configuration templates
- `internal/db/` — Database layer
- `internal/config/` — Configuration management
- `internal/migrations/` — Database migrations
- `internal/cloudflare/` — Cloudflare DNS API
- `internal/notify/` — Notification system
- `internal/features/` — Feature flags
- `internal/dbserver/` — Database server management
- `internal/audit/` — Audit logging
- `internal/executor/` — Shell command executor
- `web/` — SvelteKit dashboard (embedded in binary)
- `cmd/sigardapanel/` — Full CLI entry point
- `build.sh` — Enterprise build script

## Runtime Contracts

### Ports

| Service | Port | Description |
|---------|------|-------------|
| API | `:7700` | Panel API server + Dashboard |
| Agent | `:7790` | Agent HTTP server |

### Commands

```bash
sigardapanel api        # Run API server + Dashboard
sigardapanel agent      # Run agent service
sigardapanel dev        # Run API + agent locally
sigardapanel init       # Initialize configuration
sigardapanel login      # Authenticate CLI
sigardapanel logout     # Revoke token
sigardapanel server     # Manage servers
sigardapanel site       # Manage sites
sigardapanel ssl        # Manage SSL certificates
sigardapanel job        # Manage jobs
sigardapanel backup     # Manage backups
sigardapanel db         # Manage databases
sigardapanel user       # Manage users
sigardapanel doctor     # Diagnostics
sigardapanel version    # Show version
```

### Database

- **Path:** `/root/sigardapanel-data/sigardapanel.db`
- **Engine:** SQLite with WAL mode
- **ENV:** `SIGARDAPANEL_DB_PATH`

### Agent Authentication

- Bearer token: `Authorization: Bearer <agent_token>`
- Token generated per server (32-byte hex)
- Agent validates token before executing tasks

### Filesystem

- Site root: `/home/<system_user>/htdocs/<domain>`
- **Never** use `/var/www/`
- Logs: `/var/log/nginx/<domain>.access.log`

### Nginx

- Config path: `/etc/nginx/sites-enabled/<domain>.conf`
- Markers: `# BEGIN SIGARDAPANEL MANAGED BLOCK` / `# END SIGARDAPANEL MANAGED BLOCK`
- Validate with `nginx -t` before reload

## Security

### Trust Boundary

- Browser/CLI → Not trusted without valid token
- Backend → Policy authority
- Agent → Limited executor per server
- Database → Source of truth

### Auth

- Roles: `super_admin` > `admin` > `user`
- Check roles: `auth.HasRole(userRole, requiredRole)`
- Never use `==` for role comparison

### File Operations

- All file operations scoped to site root
- Validate paths with `safePath()`
- No global file manager endpoints

### Secrets

- Never store in plaintext
- Mask in UI and CLI output
- Encrypt at rest when possible

## Build

### Enterprise Binary (includes dashboard)

```bash
cd /root/SigardaPanel-Enterprise
./build.sh
```

### Cross-compile for macOS

```bash
cd /root/SigardaPanel-Enterprise
GOOS=darwin GOARCH=amd64 go build -o sigardapanel-darwin-amd64 ./cmd/sigardapanel
GOOS=darwin GOARCH=arm64 go build -o sigardapanel-darwin-arm64 ./cmd/sigardapanel
```

## Deployment

- Target: Ubuntu/Debian, macOS
- Systemd for services (Linux)
- SSL: Let's Encrypt via certbot
- Never run `rm -rf` without guards
- Validate config before service reload
