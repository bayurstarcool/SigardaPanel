# CLI Reference

The `sigardapanel` binary is the panel API, the server agent and the command-line
client in one. This page documents the client side (talking to the API) and the
local service/bootstrap commands. Every command and subcommand also prints its
own flags with `--help`.

```bash
sigardapanel <command> [subcommand] [args] [flags]
sigardapanel <command> [subcommand] --help
```

- [Configuration](#configuration) · [Authentication](#authentication) ·
  [Output formats](#output-formats) · [Exit codes](#exit-codes) · [Conventions](#conventions)
- Commands: [services & bootstrap](#services--bootstrap) · [session](#session) ·
  [server](#server) · [site](#site) · [ssl](#ssl) · [job](#job) · [backup](#backup) ·
  [db](#db) · [logs](#logs) · [metrics](#metrics) · [system](#system) · [user](#user) ·
  [apikey](#apikey) · [audit](#audit) · [notification](#notification) ·
  [channels](#channels) · [alerts](#alerts) · [docker](#docker) · [git](#git) ·
  [firewall](#firewall) · [redis](#redis) · [cloudpanel](#cloudpanel)
- [Not covered by the CLI](#not-covered-by-the-cli)

## Configuration

`sigardapanel login` stores two files in the config directory
`$XDG_CONFIG_HOME/sigardapanel/` (default `~/.config/sigardapanel/`), both mode `0600`:

| File | Content |
|------|---------|
| `config.json` | `{"api_url": "..."}` — the API the session belongs to |
| `token` | the session bearer token |

Environment variables used by the client:

| Variable | Meaning |
|----------|---------|
| `SIGARDAPANEL_API_URL` | API base URL. Overrides `config.json`; default `http://localhost:7700`. |
| `SIGARDAPANEL_TOKEN` | Bearer token, e.g. a [personal API key](#apikey). Overrides the saved `token` file. |
| `SIGARDAPANEL_OUTPUT` | Default output format: `table` (default), `json` or `yaml`. |
| `SIGARDAPANEL_USER`, `SIGARDAPANEL_PASSWORD` | Defaults for `login --user/--password`. |
| `XDG_CONFIG_HOME` | Moves the config directory. |

Resolution order — API URL: `login --api-url` (login only) → `SIGARDAPANEL_API_URL` →
`config.json` → `http://localhost:7700`. Token: `SIGARDAPANEL_TOKEN` → `token` file.
`logout` always revokes at the URL saved by `login`.

Service-side variables (`SIGARDAPANEL_API_ADDR`, `SIGARDAPANEL_AGENT_ADDR`,
`SIGARDAPANEL_DB_PATH`, `SIGARDAPANEL_AGENT_TOKEN`, `SIGARDAPANEL_ENCRYPTION_KEY`, …)
are described in [AGENTS.md](../AGENTS.md#env-variables).

## Authentication

```bash
sigardapanel login --api-url https://cp.example.com --user admin   # prompts for the password
sigardapanel whoami
```

Accounts with two-factor authentication are prompted for the code, or pass
`--totp 123456` / `--recovery-code CODE`.

For CI and scripts create a personal API key once from an interactive session and
export it:

```bash
sigardapanel apikey create --name ci --expires-in-days 90   # prints the key once
export SIGARDAPANEL_API_URL=https://cp.example.com
export SIGARDAPANEL_TOKEN=<key>
sigardapanel site list -o json
```

A key acts as its owner with the owner's role, except for endpoints that require
an interactive session: user management (`user …`), `apikey …`, `audit purge`,
service tokens, impersonation and `logout`. Those answer `403 api key cannot access
this endpoint`; the CLI then suggests `sigardapanel login`.

Roles matter: most commands work for every user on the sites they own or are
assigned to; commands marked *(admin)* need `admin` or `super_admin`, *(super_admin)*
needs `super_admin`. Non-super-admins see only their own and assigned sites.

## Output formats

List and `get` commands accept `-o` / `--output`:

| Format | Description |
|--------|-------------|
| `table` | Aligned columns (lists) or `key: value` lines (single objects). Default. |
| `json` | The API's objects, pretty-printed. Lists are JSON arrays. |
| `yaml` | The same data as YAML (keys sorted; strings YAML could misread are quoted). |

`--output` beats `SIGARDAPANEL_OUTPUT`. An invalid `--output` is a usage error; an
invalid `SIGARDAPANEL_OUTPUT` falls back to `table`. Commands that only perform an
action (deploy, suspend, revoke, …) print a one-line confirmation including any
`job_id`. Secrets returned by the API (agent tokens, API keys, generated DB passwords,
webhook secrets) are printed once, on stdout.

## Exit codes

| Code | Meaning |
|------|---------|
| `0` | Success, or help was printed. |
| `1` | API error (any non-2xx; the API's message is printed on stderr), network error, a watched job that failed/was canceled/timed out, a declined confirmation prompt. |
| `2` | Usage error: unknown command/subcommand, bad flag, missing or invalid argument. |

Errors go to stderr as `API error (409 conflict): site is not suspended (status is "active")`.
On `401` the CLI hints at `sigardapanel login` (or at an invalid `SIGARDAPANEL_TOKEN`).

## Conventions

- Commands acting on one object take its ID as a positional argument, and accept
  the older `--id ID` (`--site ID` for `ssl`) as well: `site get 5` = `site get --id 5`.
  Flags and positional arguments may be mixed in any order.
- Destructive commands (`backup restore|delete`, `db delete`, `db user rotate-password`,
  `audit purge`, `firewall delete|reset`, `redis flush|flushdb`) ask for confirmation.
  `-y` / `--yes` skips it; without a terminal the prompt is declined (exit 1).
- Most mutating operations are asynchronous: they queue a job and print its `job_id`.
  Follow it with `sigardapanel job watch <id>`.

---

## Services & bootstrap

These run on the panel host or a managed server; they read `SIGARDAPANEL_*`
environment variables and take no arguments (`--help` only prints usage).

| Command | Description |
|---------|-------------|
| `api` | Run the panel API (`SIGARDAPANEL_API_ADDR`, default `:7700`). |
| `agent` | Run the server agent (`SIGARDAPANEL_AGENT_ADDR`, default `:7710`). |
| `dev` | Run API and agent in one process (development). |
| `install` | Interactive setup wizard for development checkouts: migrates `SIGARDAPANEL_DB_PATH`, creates the first super_admin. Production installs use `install.sh` — see [INSTALL.md](INSTALL.md). |
| `init` | Create an admin account directly in the local DB (no API). |

```bash
sigardapanel init [--user NAME] [--password P] [--email E] [--role super_admin|admin] [--must-change-password]
```

`init` runs migrations on `SIGARDAPANEL_DB_PATH`. Defaults: user `admin`
(`$SIGARDAPANEL_ADMIN_USER`), password prompted (`$SIGARDAPANEL_ADMIN_PASSWORD`,
min. 8 chars), email `<user>@localhost`. **Role:** `super_admin` when the database has
no super_admin yet (the first owner), otherwise `admin`; `--role` overrides.
The password is the account's own; `--must-change-password` marks it temporary
(see [Password change required](#password-change-required)). `install.sh` does the
equivalent when it generates the admin password, but not when you pass `ADMIN_PASS`.

## Session

```bash
sigardapanel login [--api-url URL] [--user NAME] [--password P] [--totp CODE | --recovery-code CODE]
sigardapanel logout
sigardapanel passwd [--current P] [--new P]     # also: sigardapanel user passwd
sigardapanel whoami [-o FORMAT]
sigardapanel version [--client] [-o FORMAT]
sigardapanel --version            # same as: version --client
sigardapanel doctor [--api-url URL]
```

- `login` saves `config.json` and `token` (see [Configuration](#configuration)) and
  warns when `SIGARDAPANEL_TOKEN` would shadow the new session.
- `logout` revokes the saved session server-side (best effort, warns on failure) and
  deletes the `token` file. It never revokes `SIGARDAPANEL_TOKEN`.
- `passwd` changes the password of the logged-in account (`PUT /api/v1/auth/profile`
  with the current and the new password; both prompted when not given, the new one
  twice). Any role may use it; it needs a login session, not an API key.

### Password change required

Accounts whose password someone else chose — generated by `install.sh`, created by
an administrator under **Users** / `user create`, or `init --must-change-password` —
must choose their own first. Until then the API answers every request except
`GET /auth/me`, `GET /auth/2fa/status`, `PUT /auth/profile` and `POST /auth/logout`
with `403 password_change_required`, and refuses the account's API keys altogether.
`login` still succeeds and prints how to continue:

```bash
sigardapanel login --api-url http://127.0.0.1:7700 --user admin
# password change required: ... Change it now with:  sigardapanel passwd
sigardapanel passwd
```

Other commands print the same hint on that `403`. The web panel sends the user to
**Profile** instead.
- `whoami` shows the user, the API URL and where the token came from.
- `version` prints the CLI version and asks the API (`GET /api/v1/version`, 3 s
  timeout) for the server version; it exits 0 even if the server is unreachable.
  Scripts that need only the binary's version should use `version --client`.
- `doctor` checks the config file, API reachability, that the token is accepted, and
  (on the panel host) that `SIGARDAPANEL_AGENT_TOKEN` matches the DB. Exit 1 on problems.

## server

```bash
sigardapanel server list [-o FORMAT]
sigardapanel server get <id> [-o FORMAT]
sigardapanel server health [<id>] [-o FORMAT]
sigardapanel server add --name NAME --hostname HOST [--ip IP] [--agent-url URL] [--ssh-port N] [--description TEXT]   # admin
sigardapanel server update <id> [--name N] [--hostname H] [--ip IP|none] [--agent-url URL] [--ssh-port N] [--description TEXT]   # admin
sigardapanel server remove <id>                       # admin (aliases: rm, delete)
sigardapanel server token [<id>] [--no-env-sync]      # admin
sigardapanel server doctor
```

- `add` prints the new server's agent token once.
- `get` never prints the agent token. `health` shows the last health check of one or
  all servers.
- `update` cannot change the status (maintained by health checks). `--agent-url` fixes
  where the panel reaches the agent (not for the panel's own server).
- `remove` refuses the panel's own server and servers that still have sites (delete or
  transfer them first); nothing on the removed machine is changed.
- `token` regenerates the agent token (default server 1). If the server is the local
  one (its `agent_url` is a loopback address) the token is also written to
  `/etc/sigardapanel/agent.env` and `panel.env` when they exist; restart
  `sigardapanel-agent` and `sigardapanel-api` afterwards. For a remote server the local
  files are left alone and the token is printed for that server's `agent.env`.
  `--no-env-sync` never touches local files.
- `server doctor` only checks that the API answers `/api/v1/health`.

## site

```bash
sigardapanel site list [--server ID] [--status S] [--runtime R] [--search Q] [--owner ID] [--limit N] [--offset N] [-o FORMAT]
sigardapanel site get <id> [-o FORMAT]                # alias: show
sigardapanel site create --server ID --name NAME --domain DOMAIN [--runtime static|php|node|go|python]
                         [--root PATH] [--php-version V] [--proxy-url URL] [--system-user U] [-o FORMAT]
sigardapanel site update <id> [--git-repo URL [--git-branch B]] [--proxy-url URL|none] [--backup-provider ID]
sigardapanel site delete <id>
sigardapanel site deploy <id>
sigardapanel site suspend <id>                        # admin
sigardapanel site resume <id>                         # admin
sigardapanel site reload-nginx <id>
sigardapanel site setup-logrotate <id>                # admin
```

- Site statuses: `pending`, `active`, `suspended`, `error`, `deleting`.
- `create` queues a `site.create` job and prints the site ID and job ID. The root path
  defaults to `/home/<system_user>/htdocs/<domain>`; only admins may choose
  `--system-user` or a root outside their home. When Cloudflare is configured the DNS
  record result is shown.
- `update` (aliases `config`, `edit`) changes what `POST /sites/:id/config` supports:
  git repository (HTTPS) and branch (default `main`), the reverse-proxy upstream
  (`none` clears it) and the backup provider (`0` = default). Name, domain, runtime and
  root path are fixed after creation. The first git configuration prints the deploy
  webhook secret once.
- `delete` removes the site, its files and databases (cleanup job).
- `suspend` takes an *active* site offline (vhost and certificate renewal disabled,
  data kept); `resume` reverses it. Both answer `409` when the site is in the wrong
  state or a suspend/resume job is already queued. While suspended, the agent keeps
  the site's vhost in `sites-disabled/` and serves a generated placeholder
  (`sites-enabled/<domain>.conf`, first line `# sigardapanel: suspended placeholder`)
  that answers `503` with a "temporarily suspended" page on port 80 and, if the
  domain has a Let's Encrypt certificate, on 443. `resume` deletes that file only if
  it still carries the marker; a failing `nginx -t` rolls either step back.

```console
$ sigardapanel site suspend 12
suspend job queued for site 12 (job_id=481)
$ sigardapanel site resume 12
API error (409 conflict): site is not suspended (status is "active")
```

## ssl

```bash
sigardapanel ssl status <site-id> [-o FORMAT]
sigardapanel ssl issue <site-id> [--email E]
sigardapanel ssl renew <site-id>
sigardapanel ssl renew-all [--dry-run] [-o FORMAT]     # admin
```

- `issue`'s Let's Encrypt email defaults to the site owner's, then yours.
- `renew-all` calls `POST /api/v1/ssl/renew-all`, which queues renewals for every
  certificate expiring within 30 days on active sites (skipping ones with a renewal
  already pending) and reports `queued/skipped/total`. `--dry-run` lists the
  candidates among the sites you can see without queueing anything.

## job

```bash
sigardapanel job list [--status S] [--type T] [--server ID] [--site ID] [--limit N] [--offset N] [-o FORMAT]
sigardapanel job get <id> [-o FORMAT]
sigardapanel job watch <id> [--timeout 5m]
sigardapanel job logs <id> [--follow|-f] [--since N]
sigardapanel job cancel <id>
```

- Statuses: `queued`, `running`, `succeeded`, `failed`, `canceled`. Types include
  `site.create|delete|deploy|suspend|resume`, `ssl.issue|renew`, `nginx.reload`,
  `backup.create|restore|delete`, `database.create|delete`, `dbuser.create|delete`,
  `app.start|stop|restart`.
- `get` includes the job's last `error`.
- `watch` polls every 2 s and exits 0 when the job succeeds, 1 when it fails, is
  canceled or `--timeout` passes.
- `logs --follow` polls until the job finishes (max 10 min).
- `cancel` answers `409` for jobs that already finished. There is no retry endpoint;
  re-run the original command to queue a new job.

## backup

```bash
sigardapanel backup list [--site ID] [-o FORMAT]
sigardapanel backup create --site ID [--provider ID]
sigardapanel backup restore <id> [--yes]
sigardapanel backup delete <id> [--yes]
```

`create` answers `409` (printed as "backup not queued: …") when a backup of the site
is already running. `restore` overwrites the site's files.

## db

```bash
sigardapanel db list [--site ID] [-o FORMAT]
sigardapanel db create --site ID --name NAME [--type mysql|postgres] [--charset C] [--collation C]
sigardapanel db update <id> [--charset C] [--collation C]
sigardapanel db delete <id> [--yes]
sigardapanel db user list --database ID [-o FORMAT]
sigardapanel db user create --database ID --username U [--password P] [--host H] [--privileges P]
sigardapanel db user rotate-password --database ID --user ID [--yes]
sigardapanel db user delete <database-id> <user-id>
```

`db list` without `--site` lists every database you can access. Databases cannot be
renamed. `rotate-password` lets the API generate a new random password and prints it.

## logs

```bash
sigardapanel logs tail --site ID [--type access|error|php] [--lines N] [--follow|-f] [-o FORMAT]
```

Prints the last N (1–5000, default 100) lines of the site's nginx access/error log or
the PHP-FPM log, oldest first. `--follow` polls every 2 s (max 10 min) and prints only
new lines.

## metrics

```bash
sigardapanel metrics latest --server ID [-o FORMAT]
sigardapanel metrics list --server ID [--range 1h|24h|7d] [-o FORMAT]
sigardapanel metrics gpu --server ID [-o FORMAT]
```

`latest` answers `404 no metrics yet` until the collector has a sample.

## system

```bash
sigardapanel system disk-usage --server ID [-o FORMAT]
sigardapanel system updates --server ID               # admin; runs apt on the server
```

## user

```bash
sigardapanel user list [--direct] [-o FORMAT]
sigardapanel user create [--username U] [--email E] [--password P] [--role super_admin|admin|user] [--disk-limit GB] [--max-sites N]
sigardapanel user update <user> [--role R] [--disk-limit GB] [--max-sites N]
sigardapanel user reset-password <user> [--password P] [--direct]
sigardapanel user delete <user>
sigardapanel user passwd [--current P] [--new P]     # your own password, any role (= sigardapanel passwd)
```

A user created here must choose their own password at the first login
([Password change required](#password-change-required)).

*(admin, interactive session only.)* `<user>` is an ID, username or email. Quotas:
`0` = unlimited. Only a super_admin can promote to `admin`/`super_admin`; admins can
only change users they created. Creating a user also creates the matching Linux
account on the panel host.

`--direct` bypasses the API and reads/writes the local panel SQLite DB
(`SIGARDAPANEL_DB_PATH`, else `/var/lib/sigardapanel/sigardapanel.db`) — the recovery
path when nobody can log in. Quote passwords with special characters:
`sigardapanel user reset-password admin --password "#pass#word" --direct`.

## apikey

```bash
sigardapanel apikey list [-o FORMAT]                   # alias: api-key
sigardapanel apikey create --name NAME [--expires-in-days N] [-o FORMAT]
sigardapanel apikey revoke <id>
```

Personal API keys for the calling user (max 20). `create` prints the key once —
store it, then use it via `SIGARDAPANEL_TOKEN`. `--expires-in-days` is 1–3650, `0`
(default) never expires. `list` shows prefix, creation, last use and expiry, never the
key. Managing keys requires an interactive login, so a leaked key cannot mint or
revoke keys.

## audit

```bash
sigardapanel audit list [--user ID] [--resource R] [--action A] [--limit N] [--offset N] [-o FORMAT]   # admin
sigardapanel audit purge --older-than-days N [--yes]                                                 # super_admin
```

`list` returns newest first (limit 1–1000, default 100). Typical resources: `site`,
`server`, `user`, `session`, `api_key`, `audit_log`; actions e.g. `login`,
`site_create`, `site_suspend`, `api_key.create`, `audit.purge`. `purge` deletes entries
older than N days (1–3650) and is itself recorded; it needs an interactive session.

## notification

```bash
sigardapanel notification list [--unread] [--limit N] [--offset N] [-o FORMAT]   # alias: notifications
sigardapanel notification count
sigardapanel notification read <id> | --all
sigardapanel notification delete <id> | --read
```

Your own in-panel notifications (the bell in the web UI). `count` prints the unread
count. `--unread` filters the fetched page.

## channels

```bash
sigardapanel channels list [-o FORMAT]                                   # admin
sigardapanel channels add --name NAME --type TYPE --config JSON          # admin
sigardapanel channels remove <id>                                        # admin
```

Alert channels (where server/site alerts are sent). `--config` must be a JSON object:

| Type | Config |
|------|--------|
| `telegram` | `{"bot_token":"…","chat_id":"…"}` |
| `discord` | `{"webhook_url":"https://discord.com/api/webhooks/…"}` |
| `email` | `{"to_address":"ops@example.com","from_address":"…","smtp_tls":"starttls\|ssl\|none"}` |
| `webhook` | `{"url":"https://example.com/hook"}` |

## alerts

```bash
sigardapanel alerts list [--limit N] [-o FORMAT]                         # admin
```

## docker

Proxied to the server's agent *(admin; `ps` and `logs` are limited to your sites'
containers for non-admins)*.

```bash
sigardapanel docker ps --server ID [-o FORMAT]
sigardapanel docker start|stop|restart --server ID --id CONTAINER
sigardapanel docker rm --server ID --id CONTAINER [--force]
sigardapanel docker logs --server ID --id CONTAINER [--lines N]
sigardapanel docker images --server ID [-o FORMAT]
sigardapanel docker compose up --server ID [--path FILE] [--build]
sigardapanel docker compose down --server ID [--path FILE]
```

## git

```bash
sigardapanel git branches --site ID [-o FORMAT]
sigardapanel git log --site ID [--lines N] [-o FORMAT]
sigardapanel git rollback --site ID --commit HASH       # admin
sigardapanel git checkout --site ID --branch NAME       # admin
```

## firewall

UFW on a managed server, through its agent *(admin)*.

```bash
sigardapanel firewall status --server ID
sigardapanel firewall enable|disable --server ID
sigardapanel firewall allow --server ID --port PORT [--protocol tcp|udp] [--from IP] [--comment TEXT]
sigardapanel firewall deny --server ID --port PORT [--protocol tcp|udp] [--from IP]
sigardapanel firewall rules --server ID [-o FORMAT]
sigardapanel firewall delete --server ID --number N [--yes]
sigardapanel firewall reset --server ID [--yes]
```

## redis

*(admin)*

```bash
sigardapanel redis stats --server ID [-o FORMAT]
sigardapanel redis info --server ID
sigardapanel redis flush --server ID [--yes]
sigardapanel redis flushdb --server ID --db N [--yes]
```

## cloudpanel

Local migration from CloudPanel on the same host (reads CloudPanel's files and writes
the panel DB directly; no API).

```bash
sigardapanel cloudpanel scan
sigardapanel cloudpanel import-sites [--dry-run] [--db-path PATH] [--server-id ID] [--owner-id ID]
sigardapanel cloudpanel import-site <domain>
sigardapanel cloudpanel import-ssl [--dry-run=false] [--db-path PATH]
sigardapanel cloudpanel doctor
```

---

## Not covered by the CLI

The CLI covers day-to-day operations. These API areas are deliberately left to the
web UI or direct API calls (see [API.md](API.md)):

- **File manager, vhost editor, PHP config, cron, SSH/FTP accounts, IP restrictions,
  Varnish, app runtime (start/stop/limits)** — interactive editing; awkward and risky as
  one-shot commands.
- **Cloudflare DNS/zones, license, plans and orders, member area** — business/UI
  features, not operator tasks on a server.
- **Backup providers, backup configs/schedules, bulk backups, retention settings** —
  multi-field configuration best done in the UI; `backup create --provider` covers
  ad-hoc use.
- **Service tokens, impersonation, 2FA setup, terminal tickets/WebSocket terminal** —
  session-bound or interactive by design (personal API keys replace service tokens for
  scripting).
- **Fail2ban, security monitor, PM2/email/bot-blocker managers, stack actions, raw
  `servers/:id/command`** — the raw command endpoint is used internally by
  `site setup-logrotate` and `system updates` only.
- **Agent registration/install script, metrics ingest, job log append, deploy webhook**
  — called by agents and Git hosts, not by people.
- **Site users/permissions (`/users/:id/sites`), site transfer, DB grant/phpMyAdmin,
  database-server import** — planned candidates for a later CLI round.
