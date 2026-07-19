# CLI Reference

This document defines the unified binary and CLI interface for `sigardapanel`.

## Purpose

- Single binary for API, agent, development runner, and CLI
- Supports automation and terminal workflows
- Uses the same API as the dashboard
- Safe for scripting with clear exit codes
- Never leaks secrets to stdout or logs

## Configuration

### Default Locations

| Platform | Path |
|----------|------|
| Linux/macOS | `~/.config/sigardapanel/config.yaml` |
| Project local | `.sigardapanel.yaml` (optional) |

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `SIGARDAPANEL_API_ADDR` | `:7700` | API server address |
| `SIGARDAPANEL_AGENT_ADDR` | `:7710` | Agent address |
| `SIGARDAPANEL_DB_PATH` | `sigardapanel.db` | Database file path |
| `SIGARDAPANEL_API_URL` | - | Panel API URL |
| `SIGARDAPANEL_TOKEN` | - | API token from login |
| `SIGARDAPANEL_AGENT_TOKEN` | - | Agent authentication token |
| `SIGARDAPANEL_OUTPUT` | `table` | Output format: `table`, `json`, `yaml` |
| `SIGARDAPANEL_DEV` | `false` | Dev mode (hot reload, no rate limiters) |

### Example Config

```yaml
api_url: https://panel.example.com
output: table
timeout: 30s
```

Tokens are stored via keychain/secret store when available. Otherwise, config files must have strict permissions.

## Global Flags

| Flag | Description |
|------|-------------|
| `--api-url <url>` | Override API URL |
| `--token <token>` | Override token |
| `--output table\|json\|yaml` | Output format |
| `--timeout <duration>` | Request timeout |
| `--yes` | Skip confirmation for non-interactive operations |
| `--verbose` | Safe detailed output |
| `--debug` | Development mode (still masks secrets) |

## Exit Codes

| Code | Description |
|------|-------------|
| `0` | Success |
| `1` | General error |
| `2` | Invalid input or usage |
| `3` | Authentication or permission failure |
| `4` | Resource not found |
| `5` | Server or agent unreachable |
| `6` | Job failure |

---

## Service Commands

### `sigardapanel api`

Run the panel API server.

```bash
sigardapanel api
```

### `sigardapanel agent`

Run the agent service on a managed VPS.

```bash
SIGARDAPANEL_AGENT_TOKEN=<token> sigardapanel agent
```

### `sigardapanel dev`

Run both API and agent simultaneously for local development.

```bash
sigardapanel dev
```

### `sigardapanel install`

Interactive setup wizard (first-time install).

```bash
sigardapanel install
```

---

## Authentication Commands

### `sigardapanel init`

Create initial admin user (direct DB, no API needed).

```bash
sigardapanel init --api-url https://panel.example.com
```

### `sigardapanel login`

Login and save CLI token/session.

```bash
sigardapanel login --api-url https://panel.example.com
```

### `sigardapanel logout`

Revoke local token or current session.

```bash
sigardapanel logout
```

---

## Server Commands

### `sigardapanel server add`

Register a new server or create agent bootstrap token.

```bash
sigardapanel server add --name prod-1 --host 10.0.0.10
```

### `sigardapanel server list`

List all servers.

```bash
sigardapanel server list --output table
```

### `sigardapanel server update`

Update server details.

```bash
sigardapanel server update --server prod-1 --name new-name
```

### `sigardapanel server remove`

Remove server from panel.

```bash
sigardapanel server remove --server prod-1 --yes
```

### `sigardapanel server doctor`

Run diagnostics on server and agent.

```bash
sigardapanel server doctor --server prod-1
```

---

## Agent Commands

### `sigardapanel agent install`

Display or run agent installation flow.

```bash
sigardapanel agent install --server prod-1
```

### `sigardapanel agent status`

Check agent health and capabilities.

```bash
sigardapanel agent status
```

### `sigardapanel agent logs`

Retrieve agent logs.

```bash
sigardapanel agent logs
```

---

## Site Commands

### `sigardapanel site create`

Create a new site.

```bash
sigardapanel site create --server prod-1 --domain example.com --runtime node
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--server <name\|id>` | Target server |
| `--domain <domain>` | Site domain |
| `--runtime static\|node\|go\|php\|python\|proxy\|docker` | Runtime type |
| `--root <path>` | Custom root path (optional, validated) |
| `--ssl` | Issue SSL after creation |

### `sigardapanel site list`

List all sites.

```bash
sigardapanel site list
```

### `sigardapanel site update`

Update site configuration.

```bash
sigardapanel site update --site example.com --runtime node
```

### `sigardapanel site delete`

Delete a site. Requires confirmation or `--yes`.

```bash
sigardapanel site delete --site example.com --yes
```

### `sigardapanel site deploy`

Deploy a site.

```bash
sigardapanel site deploy --site example.com --git https://github.com/org/app.git --branch main
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--site <domain\|id>` | Target site |
| `--git <url>` | Git repository URL |
| `--branch <branch>` | Git branch |
| `--artifact <path>` | Artifact file path |
| `--build-command <command>` | Custom build command (optional, gated) |
| `--restart` | Restart service after deploy |

### `sigardapanel site config`

View/update site config.

```bash
sigardapanel site config --site example.com
```

### `sigardapanel site setup-logrotate`

Setup logrotate for site.

```bash
sigardapanel site setup-logrotate --site example.com
```

---

## Git Commands

### `sigardapanel git branches`

List git branches for a site.

```bash
sigardapanel git branches --site example.com
```

### `sigardapanel git log`

View git commit log.

```bash
sigardapanel git log --site example.com
```

### `sigardapanel git rollback`

Rollback to a previous commit.

```bash
sigardapanel git rollback --site example.com --commit abc123
```

### `sigardapanel git checkout`

Checkout a specific branch.

```bash
sigardapanel git checkout --site example.com --branch main
```

---

## SSL Commands

### `sigardapanel ssl issue`

Issue SSL certificate for a site/domain.

```bash
sigardapanel ssl issue --site example.com
```

### `sigardapanel ssl renew`

Renew SSL certificate manually.

```bash
sigardapanel ssl renew --site example.com
```

### `sigardapanel ssl renew-all`

Renew all SSL certificates.

```bash
sigardapanel ssl renew-all
```

### `sigardapanel ssl status`

View SSL expiry and status.

```bash
sigardapanel ssl status --site example.com
```

---

## Log Commands

### `sigardapanel logs tail`

Tail logs for site, service, or job.

```bash
sigardapanel logs tail --site example.com --type error
```

**Log Types:**

| Type | Description |
|------|-------------|
| `access` | Access logs |
| `error` | Error logs |
| `app` | Application logs |
| `agent` | Agent logs |
| `job` | Job logs |

---

## Job Commands

### `sigardapanel job list`

List all jobs.

```bash
sigardapanel job list
```

### `sigardapanel job watch`

Watch job progress in real-time.

```bash
sigardapanel job watch --job 123
```

### `sigardapanel job cancel`

Cancel a job if possible.

```bash
sigardapanel job cancel --job 123
```

### `sigardapanel job logs`

View job logs.

```bash
sigardapanel job logs --job 123
```

---

## Backup Commands

### `sigardapanel backup create`

Create a backup for a site.

```bash
sigardapanel backup create --site example.com
```

### `sigardapanel backup list`

List all backups.

```bash
sigardapanel backup list --site example.com
```

### `sigardapanel backup delete`

Delete a backup.

```bash
sigardapanel backup delete --backup 123 --yes
```

### `sigardapanel backup restore`

Restore a backup. Requires confirmation or `--yes`.

```bash
sigardapanel backup restore --backup 123 --yes
```

---

## Database Commands

### `sigardapanel db create`

Create a new database.

```bash
sigardapanel db create --site example.com --name mydb
```

### `sigardapanel db list`

List databases.

```bash
sigardapanel db list --site example.com
```

### `sigardapanel db delete`

Delete a database.

```bash
sigardapanel db delete --database 123 --yes
```

### `sigardapanel db user create`

Create a database user.

```bash
sigardapanel db user create --database 123 --username dbuser
```

### `sigardapanel db user list`

List database users.

```bash
sigardapanel db user list --database 123
```

### `sigardapanel db user rotate-password`

Rotate database user password.

```bash
sigardapanel db user rotate-password --user 123
```

### `sigardapanel db user delete`

Delete a database user.

```bash
sigardapanel db user delete --user 123 --yes
```

---

## Docker Commands

### `sigardapanel docker ps`

List Docker containers.

```bash
sigardapanel docker ps --server prod-1
```

### `sigardapanel docker start`

Start a container.

```bash
sigardapanel docker start --server prod-1 --container myapp
```

### `sigardapanel docker stop`

Stop a container.

```bash
sigardapanel docker stop --server prod-1 --container myapp
```

### `sigardapanel docker restart`

Restart a container.

```bash
sigardapanel docker restart --server prod-1 --container myapp
```

### `sigardapanel docker rm`

Remove a container.

```bash
sigardapanel docker rm --server prod-1 --container myapp --yes
```

### `sigardapanel docker logs`

View container logs.

```bash
sigardapanel docker logs --server prod-1 --container myapp
```

### `sigardapanel docker images`

List Docker images.

```bash
sigardapanel docker images --server prod-1
```

### `sigardapanel docker compose`

Docker compose operations.

```bash
sigardapanel docker compose up --server prod-1 --path /path/to/compose
sigardapanel docker compose down --server prod-1 --path /path/to/compose
```

---

## Firewall Commands

### `sigardapanel firewall status`

View firewall status.

```bash
sigardapanel firewall status --server prod-1
```

### `sigardapanel firewall enable`

Enable firewall.

```bash
sigardapanel firewall enable --server prod-1
```

### `sigardapanel firewall disable`

Disable firewall.

```bash
sigardapanel firewall disable --server prod-1
```

### `sigardapanel firewall rules`

List firewall rules.

```bash
sigardapanel firewall rules --server prod-1
```

### `sigardapanel firewall reset`

Reset firewall to defaults.

```bash
sigardapanel firewall reset --server prod-1 --yes
```

---

## Alert Commands

### `sigardapanel channels add`

Add an alert channel.

```bash
sigardapanel channels add --type telegram --name alerts
```

### `sigardapanel channels list`

List alert channels.

```bash
sigardapanel channels list
```

### `sigardapanel channels remove`

Remove an alert channel.

```bash
sigardapanel channels remove --channel 123 --yes
```

### `sigardapanel alerts list`

View alert history.

```bash
sigardapanel alerts list
```

---

## System Commands

### `sigardapanel doctor`

Diagnose CLI configuration and connectivity.

```bash
sigardapanel doctor
```

### `sigardapanel version`

Print version info.

```bash
sigardapanel version
```

---

## JSON Output

All list and detail commands support `--output json` for automation.

```json
{
  "id": 1,
  "domain": "example.com",
  "runtime": "node",
  "status": "active"
}
```

## Secret Masking

- Tokens displayed as `sp_abc...xyz`
- Environment variable values not shown in full
- Deploy keys not displayed after creation
- Debug logs still mandatory to mask secrets
