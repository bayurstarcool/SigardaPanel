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

| Variable | Description |
|----------|-------------|
| `SIGARDAPANEL_API_URL` | Panel API URL |
| `SIGARDAPANEL_TOKEN` | Authentication token |
| `SIGARDAPANEL_OUTPUT` | Output format: `table`, `json`, `yaml` |

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

## Runtime Commands

### `sigardapanel api`

Run the panel API server. Used on the main panel server.

### `sigardapanel agent`

Run the agent service. Used on managed VPS targets.

### `sigardapanel dev`

Run both API and agent simultaneously for local development.

## Authentication Commands

### `sigardapanel init`

Create initial configuration.

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

### `sigardapanel server doctor`

Run diagnostics on server and agent.

```bash
sigardapanel server doctor --server prod-1
```

### `sigardapanel server remove`

Remove server from panel. Requires `--yes` for non-interactive mode.

## Agent Commands

### `sigardapanel agent install`

Display or run agent installation flow.

```bash
sigardapanel agent install --server prod-1
```

### `sigardapanel agent status`

Check agent health and capabilities.

### `sigardapanel agent logs`

Retrieve agent logs.

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
| `--runtime static\|node\|go\|php\|python` | Runtime type |
| `--root <path>` | Custom root path (optional, validated) |
| `--ssl` | Issue SSL after creation |

### `sigardapanel site list`

List all sites.

### `sigardapanel site detail`

Show site details.

### `sigardapanel site delete`

Delete a site. Requires confirmation or `--yes`.

### `sigardapanel site enable|disable`

Enable or disable a site.

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

## SSL Commands

### `sigardapanel ssl issue`

Issue SSL certificate for a site/domain.

### `sigardapanel ssl renew`

Renew SSL certificate manually.

### `sigardapanel ssl status`

View SSL expiry and status.

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

## Job Commands

### `sigardapanel job list`

List all jobs.

### `sigardapanel job watch`

Watch job progress in real-time.

### `sigardapanel job cancel`

Cancel a job if possible.

## Backup Commands

### `sigardapanel backup create`

Create a backup for a site or server.

### `sigardapanel backup list`

List all backups.

### `sigardapanel backup restore`

Restore a backup. Requires confirmation or `--yes`.

## JSON Output

All list and detail commands support `--output json` for automation.

```json
{
  "id": "site_123",
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
