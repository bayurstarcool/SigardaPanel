# CLI Reference

SigardaPanel CLI v0.5.0 — 55+ subcommands.

## Usage

```bash
sigardapanel <command> [subcommand] [flags]
```

## Global Commands

| Command | Description |
|---------|-------------|
| `api` | Run API server |
| `agent` | Run agent service |
| `dev` | Run API + agent locally |
| `install` | Setup wizard (interactive) |
| `init` | Create admin user (direct DB) |
| `login` | Authenticate CLI |
| `logout` | Revoke current token |
| `doctor` | Diagnose CLI configuration |
| `version` | Print version |

## Server Management

```bash
sigardapanel server add --name <name> --hostname <host> [--ip <ip>]
sigardapanel server list [--output json]
sigardapanel server update --id <id> [--name N] [--hostname H] [--ip I] [--status S]
sigardapanel server remove <id>
sigardapanel server doctor
```

## Site Management

```bash
sigardapanel site list [--server ID] [--output json]
sigardapanel site create --server ID --name X --domain Y [--runtime R] [--root PATH]
sigardapanel site update --id ID [--name N] [--domain D] [--runtime R] [--git-repo URL]
sigardapanel site delete <id>
sigardapanel site deploy <id>
sigardapanel site config --id ID --git-repo URL [--git-branch BRANCH]
sigardapanel site setup-logrotate --id ID
```

## SSL Management

```bash
sigardapanel ssl status --site ID
sigardapanel ssl issue --site ID --email E
sigardapanel ssl renew --site ID
sigardapanel ssl renew-all [--dry-run]
```

## Job Management

```bash
sigardapanel job list [--status S] [--type T] [--output json]
sigardapanel job watch --id ID
sigardapanel job cancel --id ID
sigardapanel job logs --id ID [--follow] [--since N]
```

## Backup Management

```bash
sigardapanel backup create --site ID
sigardapanel backup list [--site ID] [--output json]
sigardapanel backup delete <id>
sigardapanel backup restore <id>
```

## Database Management

```bash
sigardapanel db create --site ID --name X [--type mysql|postgres]
sigardapanel db list --site ID [--output json]
sigardapanel db update --id ID [--name N] [--charset C]
sigardapanel db delete <id>

sigardapanel db user create --database ID --username U [--password P]
sigardapanel db user list --database ID [--output json]
sigardapanel db user rotate-password --database DB_ID --user USER_ID [--password P]
sigardapanel db user delete <database_id> <user_id>
```

## Docker Management

```bash
sigardapanel docker ps --server ID [--output json]
sigardapanel docker start --server ID --id CID
sigardapanel docker stop --server ID --id CID
sigardapanel docker restart --server ID --id CID
sigardapanel docker rm --server ID --id CID [--force]
sigardapanel docker logs --server ID --id CID [--lines N]
sigardapanel docker images --server ID [--output json]
sigardapanel docker compose up --server ID [--path PATH] [--build]
sigardapanel docker compose down --server ID [--path PATH]
```

## Git Deploy

```bash
sigardapanel git branches --site ID [--output json]
sigardapanel git log --site ID [--lines N] [--output json]
sigardapanel git rollback --site ID --commit HASH
sigardapanel git checkout --site ID --branch NAME
```

## Firewall Management

```bash
sigardapanel firewall status --server ID
sigardapanel firewall enable --server ID
sigardapanel firewall disable --server ID
sigardapanel firewall allow --server ID --port PORT [--protocol PROTO] [--from IP]
sigardapanel firewall deny --server ID --port PORT [--protocol PROTO] [--from IP]
sigardapanel firewall rules --server ID [--output json]
sigardapanel firewall delete --server ID --number N
sigardapanel firewall reset --server ID
```

## Redis Management

```bash
sigardapanel redis stats --server ID [--output json]
sigardapanel redis info --server ID
sigardapanel redis flush --server ID
sigardapanel redis flushdb --server ID --db N
```

## System Management

```bash
sigardapanel system disk-usage --server ID [--output json]
sigardapanel system updates --server ID
```

## Alert Channels

```bash
sigardapanel channels add --name <name> --type <type> --config '{...}'
sigardapanel channels list [--output json]
sigardapanel channels remove <id>
```

## User Management

```bash
sigardapanel user create [--username U] [--email E] [--password P] [--role R]
sigardapanel user list [--output json]
sigardapanel user reset-password <id> [--password P]
sigardapanel user delete <id>
```

## Global Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| SIGARDAPANEL_API_ADDR | :8080 | API listen address |
| SIGARDAPANEL_AGENT_ADDR | :9090 | Agent listen address |
| SIGARDAPANEL_API_URL | - | API base URL |
| SIGARDAPANEL_TOKEN | - | API token |
| SIGARDAPANEL_OUTPUT | table | Output format (table|json|yaml) |
