# Installing SigardaPanel

This is the complete installation and operations guide for a production panel:
requirements, install, first login, release verification, update, rollback,
remote servers, uninstall and troubleshooting.

Everything here uses the signed release assets from
[GitHub Releases](https://github.com/bayurstarcool/SigardaPanel/releases/latest)
and the installer [`install.sh`](../install.sh) (the same file is kept at
[`installer/install.sh`](../installer/install.sh)). The installer does not build
anything: the Go binary and the web bundle are prebuilt, so you need neither Go
nor npm on the server.

- [Requirements](#requirements)
- [Install](#install)
- [Installer options](#installer-options)
- [What the installer does](#what-the-installer-does)
- [First login](#first-login)
- [Verifying releases manually](#verifying-releases-manually)
- [Ports and firewall](#ports-and-firewall)
- [Configuration files](#configuration-files)
- [Using a domain and Let's Encrypt later](#using-a-domain-and-lets-encrypt-later)
- [Updating](#updating)
- [Rolling back](#rolling-back)
- [Backing up the panel](#backing-up-the-panel)
- [Adding remote servers](#adding-remote-servers)
- [Uninstalling](#uninstalling)
- [Troubleshooting](#troubleshooting)
- [Building from source](#building-from-source)

---

## Requirements

| | |
|---|---|
| **OS** | Ubuntu 24.04 LTS (tested) or Debian 12. Other distributions are refused. |
| **CPU architecture** | `x86_64`/`amd64` or `aarch64`/`arm64` |
| **Resources** | 1 vCPU, 1 GB RAM, 10 GB disk minimum |
| **Access** | root (`sudo`) |
| **Inbound ports** | 22 (SSH), 80 and 443 |
| **Outbound HTTPS** | `github.com` and `objects.githubusercontent.com` (release downloads), your apt mirror, `api.ipify.org` (public IP detection) |

Use a **fresh server**. The installer takes over nginx's default server on
ports 80/443 and removes `/etc/nginx/sites-enabled/default`, so it is not meant
for a machine that already serves other sites through nginx.

The installer installs these packages from the distribution:
`nginx`, `nodejs` (>= 18.13, runs the prebuilt web bundle), `sqlite3`,
`minisign`, `openssl`, `curl`, `ca-certificates`, `tar`, `certbot` and
`python3-certbot-nginx` (Let's Encrypt certificates for the panel and for every
site you create); plus `ufw` with `INSTALL_FIREWALL=yes`.

## Install

```bash
curl -fsSLO https://raw.githubusercontent.com/bayurstarcool/SigardaPanel/main/install.sh
less install.sh                     # read it first - it is a plain shell script
sudo bash install.sh
```

The installer downloads the latest release for your architecture, verifies its
minisign signature and SHA-256 checksums (there is no option to skip this),
installs it, starts the services and prints the panel URL and the admin
credentials. On a typical VPS this takes under a minute.

With options (see the next section), pass them to `sudo` as environment variables:

```bash
sudo DOMAIN=panel.example.com LETSENCRYPT_EMAIL=you@example.com \
     ADMIN_USER=alice ADMIN_EMAIL=alice@example.com INSTALL_FIREWALL=yes \
     bash install.sh
```

The installer refuses to run on a server where SigardaPanel is already
installed; use [`update`](#updating) there instead.

## Installer options

`install.sh` takes one command and is configured with environment variables.

| Command | Meaning |
|---|---|
| `install` (default) | Fresh install of the latest (or `SIGARDA_VERSION`) release. |
| `update` | Update an existing install with backup and automatic rollback. |
| `verify` | Print the release-signing public key the script trusts. |
| `help` | Show the built-in usage text. |

| Variable | Default | Meaning |
|---|---|---|
| `SIGARDA_VERSION` | latest release | Release tag to install or update to, e.g. `v1.5.0`. |
| `DOMAIN` | empty | Public hostname for the panel. Used as the TLS name, the web `ORIGIN` and `SIGARDAPANEL_PUBLIC_URL`. Without it the panel URL is `https://<public-ip>`. |
| `LETSENCRYPT_EMAIL` | empty | With `DOMAIN`: request a Let's Encrypt certificate (DNS must already point to the server and port 80 must be reachable). Falls back to the self-signed certificate if it fails. |
| `ADMIN_USER` | `admin` | First account (3-32 characters: letters, digits, `_ . -`). |
| `ADMIN_EMAIL` | `admin@localhost` | E-mail of the first account. |
| `ADMIN_PASS` | random, 20 characters | Password of the first account (min. 8 characters). |
| `INSTALL_FIREWALL` | `no` | `yes`: install `ufw`, allow your SSH port, 80 and 443, and enable it. Existing rules are kept. |
| `SIGARDA_ASSET_BASE` | GitHub release URL | Download the assets from another location (mirror, `file:///path`). Requires `SIGARDA_VERSION`. Signature verification still applies. |
| `FORCE` | empty | With `update`: `FORCE=1` reinstalls even when already on the target version. |

There are **no default credentials**. If you do not set `ADMIN_PASS`, a random
password is generated and printed once.

## What the installer does

1. Checks for root, a supported OS and CPU architecture.
2. Installs the packages listed under [Requirements](#requirements).
3. Resolves the release tag (latest, or `SIGARDA_VERSION`) and downloads
   `sigardapanel-linux-<arch>`, `sigardapanel-web.tar.gz`, `SHA256SUMS` and
   `SHA256SUMS.minisig`; verifies the signature with the pinned key, then the checksums.
4. Creates the system user `sigardapanel` (no login shell) and the directories below.
5. Installs the binary and unpacks the web bundle.
6. Writes `/etc/sigardapanel/panel.env` and `agent.env` with a random agent
   token and a random encryption key, with public sign-up disabled.
7. Writes the three systemd units.
8. Creates the first account with `sigardapanel init` (this also creates the
   SQLite database and applies all migrations) and makes it `super_admin`.
9. Starts `sigardapanel-api`, `sigardapanel-agent` and `sigardapanel-web`.
10. Writes the nginx site (self-signed certificate, HTTP to HTTPS redirect,
    rate limits on the API and the login endpoint, security headers) and, with
    `DOMAIN` + `LETSENCRYPT_EMAIL`, runs `certbot --nginx`.
11. Waits for the API (`/api/v1/health`) and the web app (`/login`) to answer.
12. With `INSTALL_FIREWALL=yes`, configures `ufw`.
13. Saves the URL and credentials to `/root/.sigardapanel_admin_cred` (mode 600) and prints them.

### Installed files

| Path | Purpose |
|---|---|
| `/usr/local/bin/sigardapanel` | Single binary: API, agent and CLI |
| `/opt/sigardapanel/web/` | Prebuilt SvelteKit web app (owned by root, read-only for the service) |
| `/etc/sigardapanel/panel.env` | API configuration and secrets (mode 600) |
| `/etc/sigardapanel/agent.env` | Local agent configuration (mode 600) |
| `/etc/sigardapanel/selfsigned.{crt,key}` | Self-signed TLS certificate for nginx |
| `/var/lib/sigardapanel/sigardapanel.db` | SQLite database (WAL mode) |
| `/var/log/sigardapanel/`, `/var/backups/sigardapanel/` | Logs and panel-made backups |
| `/var/backups/sigardapanel-updates/<timestamp>/` | Pre-update backups made by `install.sh update` |
| `/etc/systemd/system/sigardapanel-{api,agent,web}.service` | systemd units, see [SYSTEMD.md](SYSTEMD.md) |
| `/etc/nginx/sites-available/sigardapanel`, `/etc/nginx/conf.d/sigardapanel-limits.conf` | nginx site and rate-limit zones |
| `/root/.sigardapanel_admin_cred` | First-login credentials (delete after the first login) |

| Service | Listens on | Runs as |
|---|---|---|
| `sigardapanel-api` | `127.0.0.1:7700` | `sigardapanel` (sandboxed) |
| `sigardapanel-agent` | `127.0.0.1:7710` | `root` (manages nginx, packages, firewall, Docker) |
| `sigardapanel-web` | `127.0.0.1:7720` | `sigardapanel` (sandboxed) |
| nginx | `0.0.0.0:80`, `0.0.0.0:443` | public entry point |

## First login

1. Open the URL the installer printed: `https://<server-ip>` or `https://<DOMAIN>`.
   Without a Let's Encrypt certificate it is self-signed, so the browser shows a warning.
2. Log in with the printed username and password (also in `/root/.sigardapanel_admin_cred`).
   This account is the `super_admin`.
3. The panel asks for a new password first (**Profile**); it does the same for every
   account an administrator creates. Then enable two-factor authentication under
   **Settings → Two-factor authentication** and keep the recovery codes.
4. Delete the credentials file: `sudo rm /root/.sigardapanel_admin_cred`.
5. The panel host is already managed as **Local Server** (Servers page, online after
   the first health check). Create sites on it right away, or
   [add more servers](#adding-remote-servers).

Public sign-up is **disabled** on installer-made panels
(`SIGARDAPANEL_ALLOW_REGISTRATION=false`, `SIGARDAPANEL_ALLOW_MEMBER_SIGNUP`
unset); `/register` says so. Create further accounts under **Users**. The panel
is self-hosted by default: opening `/` goes straight to the login, without the
Sigarda SaaS landing and pricing pages (`SIGARDAPANEL_LANDING_PAGE`, see
[Configuration files](#configuration-files)).

Command-line access on the server:

```bash
sigardapanel login --api-url http://127.0.0.1:7700 --user admin   # prompts for the password
sigardapanel server list
sigardapanel doctor --api-url http://127.0.0.1:7700
```

`login` stores the token in `~/.config/sigardapanel/`. See [CLI.md](CLI.md).
`sigardapanel install` is an interactive setup wizard for development checkouts;
do not use it on a server installed with `install.sh`.

## Verifying releases manually

Every release ships `SHA256SUMS` and `SHA256SUMS.minisig`, signed with this
[minisign](https://jedisct1.github.io/minisign/) public key (also pinned in `install.sh`):

```
RWTNXRQ/oh97GPfWH0tDwxbnMTe/WTZeoJdZtGTW3ESF0OpTSlNXpFXI
```

```bash
V=v1.5.0          # or the tag you want
ARCH=amd64        # or arm64
base=https://github.com/bayurstarcool/SigardaPanel/releases/download/$V
for f in SHA256SUMS SHA256SUMS.minisig "sigardapanel-linux-$ARCH" sigardapanel-web.tar.gz; do
  curl -fsSLO "$base/$f"
done
minisign -Vm SHA256SUMS -P RWTNXRQ/oh97GPfWH0tDwxbnMTe/WTZeoJdZtGTW3ESF0OpTSlNXpFXI
sha256sum -c --ignore-missing SHA256SUMS
```

The signature check must print `Signature and comment signature verified` and a
trusted comment naming the release; every downloaded file must report `OK`. If
anything fails, do not install the files and report it (see [SECURITY.md](../SECURITY.md)).

To install from verified local files (for example on a server without access to
GitHub), put the four files in one directory and point the installer at it:

```bash
sudo SIGARDA_VERSION=v1.5.0 SIGARDA_ASSET_BASE=file:///root/sigardapanel-v1.5.0 bash install.sh
```

## Ports and firewall

| Port | Exposure | Purpose |
|---|---|---|
| 22/tcp | public (or your SSH port) | SSH |
| 80/tcp | public | Redirect to HTTPS, Let's Encrypt HTTP-01 challenges |
| 443/tcp | public | Panel (nginx) |
| 7700/tcp | localhost only | API |
| 7720/tcp | localhost only | Web app |
| 7710/tcp | localhost only on the panel; on a [remote server](#adding-remote-servers) only from the panel's IP | Agent |

`INSTALL_FIREWALL=yes` does this for you. Manually:

```bash
sudo ufw allow OpenSSH          # or: ufw allow <your-ssh-port>/tcp
sudo ufw allow 80,443/tcp
sudo ufw enable
```

Behind Cloudflare use SSL mode **Full (strict)** (with a Let's Encrypt
certificate) or **Full**; "Flexible" causes a redirect loop.

## Configuration files

`/etc/sigardapanel/panel.env` (read by `sigardapanel-api`):

| Key | Installer value | Meaning |
|---|---|---|
| `SIGARDAPANEL_API_ADDR` | `127.0.0.1:7700` | API listen address |
| `SIGARDAPANEL_PUBLIC_URL` | `https://<DOMAIN or IP>` | Public panel URL |
| `SIGARDAPANEL_DB_PATH` | `/var/lib/sigardapanel/sigardapanel.db` | SQLite database |
| `SIGARDAPANEL_LOG_DIR` | `/var/log/sigardapanel` | Log directory (reserved; the services log to the journal) |
| `SIGARDAPANEL_AGENT_URL` | `http://127.0.0.1:7710` | Local agent |
| `SIGARDAPANEL_AGENT_TOKEN` | random 64 hex | Shared secret with the local agent (same value in `agent.env`) |
| `SIGARDAPANEL_ENCRYPTION_KEY` | random 64 hex | AES-256 key for secrets stored in the database (database passwords, job credentials, 2FA secrets) |
| `SIGARDAPANEL_ALLOW_REGISTRATION` | `false` | Public sign-up on `/register` |
| `SIGARDAPANEL_LANDING_PAGE` | not set (`false`) | `true`: `/` and `/pricing` show the Sigarda SaaS landing page. Otherwise they redirect to the login (self-hosted). |
| `SIGARDAPANEL_ALLOW_MEMBER_SIGNUP` | not set (`false`) | `true`: member (SaaS customer) sign-up on `/register` (also needs the member database, `MEMBER_DB_*`). |
| `SIGARDAPANEL_ALLOW_PRIVATE_AGENTS` | not set (`false`) | `true`: servers added with the [install command](#option-a-install-command) may register a private (RFC 1918 / ULA) address. See [Private networks](#private-networks). |
| `MEMBER_DB_*` | commented out | Optional member area (PostgreSQL) |

`/etc/sigardapanel/agent.env` (read by `sigardapanel-agent`):
`SIGARDAPANEL_AGENT_ADDR=127.0.0.1:7710` and `SIGARDAPANEL_AGENT_TOKEN`.

All variables and the unit files are described in [SYSTEMD.md](SYSTEMD.md).
After editing a file, restart the service that reads it, e.g.
`sudo systemctl restart sigardapanel-api`.

**The encryption key.** Secrets in the database can only be decrypted with the
key they were encrypted with. The API takes the key from
`SIGARDAPANEL_ENCRYPTION_KEY`; if that is not set (custom installs) it uses the
file `encryption.key` next to the database (or `SIGARDAPANEL_ENCRYPTION_KEY_FILE`),
which it generates with mode 600 on the first start of a new database. Never
change or lose the key, and always back it up together with the database.

## Using a domain and Let's Encrypt later

If you installed without `DOMAIN`, point the DNS record at the server, then:

```bash
D=panel.example.com
sudo sed -i "s|^SIGARDAPANEL_PUBLIC_URL=.*|SIGARDAPANEL_PUBLIC_URL=https://$D|" /etc/sigardapanel/panel.env
sudo sed -i "s|^Environment=ORIGIN=.*|Environment=ORIGIN=https://$D|" /etc/systemd/system/sigardapanel-web.service
sudo sed -i "s|server_name _;|server_name $D;|" /etc/nginx/sites-available/sigardapanel
sudo nginx -t && sudo systemctl reload nginx
sudo apt-get install -y certbot python3-certbot-nginx
sudo certbot --nginx -d "$D" -m you@example.com --agree-tos -n
sudo systemctl daemon-reload && sudo systemctl restart sigardapanel-api sigardapanel-web
```

## Updating

```bash
curl -fsSLO https://raw.githubusercontent.com/bayurstarcool/SigardaPanel/main/install.sh
sudo bash install.sh update                          # to the latest release
sudo SIGARDA_VERSION=v1.5.0 bash install.sh update   # to a specific release
sudo FORCE=1 bash install.sh update                  # reinstall the current release
```

Fetch the current `install.sh` first, as above: newer releases may need
installer fixes. The updater:

1. verifies the new release (signature and checksums) before touching anything;
2. saves the current binary, web bundle, a consistent copy of the database
   (`sqlite3 .backup`), `panel.env`/`agent.env` and `encryption.key` (if present)
   to `/var/backups/sigardapanel-updates/<UTC timestamp>/` (mode 700);
3. swaps in the new binary and web bundle and restarts agent, API and web;
4. waits for the API and web health checks and **rolls back automatically** if
   anything fails: the services are stopped, the binary, web bundle and database
   are restored, and the services are started again.

Database migrations run automatically when the new API starts. They are
forward-only; going back to an older version means restoring the database
from the pre-update backup (the automatic rollback does that for you).

The updater never edits your configuration files. Public sign-up is closed by
default since v1.6.0 (earlier versions left it open unless
`SIGARDAPANEL_ALLOW_REGISTRATION=false` was set); the updater warns if your
`panel.env` explicitly opens it with `SIGARDAPANEL_ALLOW_REGISTRATION=true`.
Since v1.6.1 the Sigarda SaaS landing page and member sign-up are off unless
`panel.env` sets `SIGARDAPANEL_LANDING_PAGE=true` and
`SIGARDAPANEL_ALLOW_MEMBER_SIGNUP=true`; a panel that is a public SaaS site
adds those lines (and restarts `sigardapanel-api`) when updating.

Old update backups are not deleted automatically; remove the ones you no longer need.

## Rolling back

To return to the state before an update that succeeded but that you do not want
to keep, restore its backup by hand (`<ts>` = the backup directory):

```bash
B=/var/backups/sigardapanel-updates/<ts>
sudo systemctl stop sigardapanel-web sigardapanel-api sigardapanel-agent
sudo install -m 755 "$B/sigardapanel.bin" /usr/local/bin/sigardapanel
sudo rm -rf /opt/sigardapanel/web && sudo cp -a "$B/web" /opt/sigardapanel/web
sudo rm -f /var/lib/sigardapanel/sigardapanel.db-wal /var/lib/sigardapanel/sigardapanel.db-shm
sudo install -m 640 -o sigardapanel -g sigardapanel "$B/sigardapanel.db" /var/lib/sigardapanel/sigardapanel.db
sudo systemctl start sigardapanel-agent sigardapanel-api sigardapanel-web
sigardapanel version
```

Everything changed in the panel after that update is lost with the database
restore. Installing an older release with
`sudo SIGARDA_VERSION=<older tag> FORCE=1 bash install.sh update` replaces only
the binary and web bundle and keeps the newer database schema, so prefer the
backup restore above.

## Backing up the panel

Back up these together (for example nightly, off the server):

```bash
sudo sqlite3 /var/lib/sigardapanel/sigardapanel.db ".backup '/root/sigardapanel-$(date +%F).db'"
sudo tar -czf "/root/sigardapanel-config-$(date +%F).tgz" /etc/sigardapanel \
     $(ls /var/lib/sigardapanel/encryption.key 2>/dev/null)
```

A database copy without the matching `SIGARDAPANEL_ENCRYPTION_KEY`
(`panel.env`) or `encryption.key` cannot decrypt the stored secrets.
Site backups made from inside the panel are a separate feature (**Backups**).

## Adding remote servers

The panel manages its own host through the local agent. To manage another VPS,
run the agent there and register it in the panel. The agent is the same binary
(`sigardapanel agent`); it runs as root and accepts requests only with the
server's agent token.

The panel connects to the agent at `http://<server>:7710` and sends the token
with every request, so keep that traffic off the open internet: use a private
network or VPN (WireGuard, provider VLAN) if you can, and in any case allow port
7710 only from the panel's IP address. Keep agents on the panel's version; the
**Servers** page shows each agent's version and warns about a mismatch.

There are two ways, both under **Servers → Add server**:

| | Install command | Manual setup |
|---|---|---|
| What you do | paste one command on the new server | register it, then install and configure the agent |
| Panel needs | `SIGARDAPANEL_PUBLIC_URL=https://…` (the installer default) whose certificate the new server trusts | nothing special |
| New server reaches the panel? | yes, over HTTPS | not needed |
| Private/VPC address | only with `SIGARDAPANEL_ALLOW_PRIVATE_AGENTS=true` | yes |

### Option A: install command

1. **Servers → Add server → Install command → Generate install command.** The
   panel creates a one-time registration token (valid for one hour) and shows:

   ```bash
   curl -fsSL -H 'X-Sigarda-Registration-Token: <token>' 'https://panel.example.com/api/v1/agents/install' | sudo bash
   ```

   (API: `POST /api/v1/agents/tokens` returns the same command as `install_command`.)
2. Run it on the new server (Ubuntu/Debian, root or sudo). The script installs
   nginx, certbot and the other dependencies, downloads the panel's release
   version for the server's architecture, verifies it against the signed
   `SHA256SUMS`, writes `/etc/sigardapanel/agent.env`, starts `sigardapanel-agent`
   and registers the server with its public IP. The dialog notices the new server
   by itself.
3. Allow port 7710 only from the panel:
   `ufw allow from <panel-ip> to any port 7710 proto tcp`.

Options, set between `sudo` and `bash` (e.g. `| sudo SIGARDAPANEL_AGENT_IP=10.0.0.5 bash`):

| Variable | Meaning |
|---|---|
| `SIGARDAPANEL_AGENT_IP` | Address the panel uses to reach the agent. Default: the server's public IP (from api.ipify.org). The dialog's *Address* field adds it for you. |
| `SIGARDAPANEL_PANEL_CA` | CA/certificate file that signs the panel's HTTPS certificate, for a panel with a self-signed or private certificate. Copy the panel's `/etc/sigardapanel/selfsigned.crt` to the server and also pass `--cacert <file>` to the first `curl`. |
| `SIGARDAPANEL_VERSION` | Install another release tag than the panel's. |
| `SIGARDAPANEL_BINARY_URL` + `SIGARDAPANEL_BINARY_SHA256` | Download the binary from a mirror (HTTPS only) and check this SHA-256 instead of the signed `SHA256SUMS`. |
| `SIGARDAPANEL_AGENT_SITE_ROOT` | Restrict site files to this directory (default: no restriction; sites live in `/home/<user>/htdocs/<domain>`). |

If anything fails, the script restores the previous state and prints the panel's
error message. A token can only be used once; generate a new command to retry.

#### Private networks

By default a self-registering agent must report a public (global unicast)
address: anyone holding a registration token could otherwise make the panel
connect to internal addresses. If your panel reaches its servers over a VPC or
VPN, allow private addresses on the panel:

```bash
echo 'SIGARDAPANEL_ALLOW_PRIVATE_AGENTS=true' | sudo tee -a /etc/sigardapanel/panel.env
sudo systemctl restart sigardapanel-api
```

and pass the private address: `| sudo SIGARDAPANEL_AGENT_IP=10.0.0.5 bash`.
Loopback, link-local (cloud metadata) and multicast addresses are always refused.
The one-line installer still needs an `https://` panel URL; without one use
[option B](#option-b-manual-setup).

### Option B: manual setup

**1. In the panel:** **Servers → Add server → Manual setup**. Enter a name, the
hostname and the IP address; the agent URL `http://<server-ip>:7710` is filled in
(use the private/VPN address if you have one). After saving, the dialog shows the
**agent token** once, already inserted into the `agent.env` commands of step 3.
CLI: `sigardapanel server add --name web1 --hostname web1.example.com --ip 203.0.113.10 --agent-url http://203.0.113.10:7710`.

**2. On the remote server** (Ubuntu/Debian, as root), download and verify the binary:

```bash
apt-get update && apt-get install -y curl ca-certificates minisign nginx certbot python3-certbot-nginx
V=v1.6.0                           # the panel's version: sigardapanel version, or Settings -> System info
ARCH=$(dpkg --print-architecture)  # amd64 or arm64
base=https://github.com/bayurstarcool/SigardaPanel/releases/download/$V
for f in SHA256SUMS SHA256SUMS.minisig "sigardapanel-linux-$ARCH"; do curl -fsSLO "$base/$f"; done
minisign -Vm SHA256SUMS -P RWTNXRQ/oh97GPfWH0tDwxbnMTe/WTZeoJdZtGTW3ESF0OpTSlNXpFXI
sha256sum -c --ignore-missing SHA256SUMS
install -m 755 "sigardapanel-linux-$ARCH" /usr/local/bin/sigardapanel
```

**3. Configure and start the agent** (insert the token; to listen on a private
address only, use that address instead of `0.0.0.0`):

```bash
install -d -m 750 /etc/sigardapanel
cat > /etc/sigardapanel/agent.env <<'EOF'
SIGARDAPANEL_AGENT_ADDR=0.0.0.0:7710
SIGARDAPANEL_AGENT_TOKEN=<agent token from step 1>
EOF
chmod 600 /etc/sigardapanel/agent.env

cat > /etc/systemd/system/sigardapanel-agent.service <<'EOF'
[Unit]
Description=SigardaPanel Agent
After=network-online.target nginx.service
Wants=network-online.target

[Service]
Type=simple
User=root
EnvironmentFile=/etc/sigardapanel/agent.env
ExecStart=/usr/local/bin/sigardapanel agent
Restart=always
RestartSec=3
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
LockPersonality=true
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload
systemctl enable --now sigardapanel-agent
curl -fsS http://127.0.0.1:7710/health

ufw allow from <panel-ip> to any port 7710 proto tcp
```

**4. Check from the panel:** the server shows as online after the next health
check (within about a minute), and the server's **Stack** page lists the
installed software. *Token rejected* means the agent runs but its token differs
from the panel's (see [Troubleshooting](#troubleshooting)).

The agent can also serve HTTPS itself (`SIGARDAPANEL_AGENT_TLS_CERT` and
`SIGARDAPANEL_AGENT_TLS_KEY` in `agent.env`, agent URL `https://...`). The panel
verifies the certificate, so it must be valid for the host name in the agent URL.

### After adding a server

- **Sites** on the new server: **Sites → Create site** and pick the server. The
  agent writes the vhost there; until a Let's Encrypt certificate is issued the
  site's HTTPS uses a self-signed placeholder (`/etc/nginx/ssl/sigardapanel.crt`,
  created by the agent on first use). Let's Encrypt is requested automatically
  and needs public DNS for the domain pointing at that server; without it the
  request fails (visible under **Jobs**) and the site keeps working over HTTP.
- **Wrong agent URL:** **Edit** the server (or `sigardapanel server update <id> --agent-url URL`).
- **Rotate the token:** **Edit → Regenerate** (or `sigardapanel server token <id>`) shows a
  new token once; put it into the server's `agent.env` and run
  `systemctl restart sigardapanel-agent` there. Until then the server shows *Token rejected*.
  For the panel's own server use `sudo sigardapanel server token` on the panel host,
  which also updates `/etc/sigardapanel/{panel,agent}.env`.
- **Update an agent:** repeat step 2 of option B with the panel's new version, then
  `systemctl restart sigardapanel-agent`.
- **Remove a server:** delete or transfer its sites first, then **Edit → Remove**
  (`sigardapanel server remove <id>`). The panel's own server cannot be removed.
  Nothing on the machine is changed; to uninstall the agent there:
  `systemctl disable --now sigardapanel-agent && rm -f /etc/systemd/system/sigardapanel-agent.service /usr/local/bin/sigardapanel && rm -rf /etc/sigardapanel`.

## Uninstalling

This removes the panel and all of its data (database, configuration, update backups).
Sites, databases and packages that the panel created on the server are left in place.

```bash
sudo systemctl disable --now sigardapanel-web sigardapanel-api sigardapanel-agent
sudo rm -f /etc/systemd/system/sigardapanel-api.service /etc/systemd/system/sigardapanel-agent.service \
           /etc/systemd/system/sigardapanel-web.service
sudo systemctl daemon-reload
sudo rm -f /etc/nginx/sites-enabled/sigardapanel /etc/nginx/sites-available/sigardapanel \
           /etc/nginx/conf.d/sigardapanel-limits.conf
sudo nginx -t && sudo systemctl reload nginx
sudo rm -f /usr/local/bin/sigardapanel /root/.sigardapanel_admin_cred
sudo rm -rf /opt/sigardapanel/web /etc/sigardapanel /var/lib/sigardapanel /var/log/sigardapanel \
            /var/backups/sigardapanel /var/backups/sigardapanel-updates
sudo userdel sigardapanel
```

Take a [backup](#backing-up-the-panel) first if you might want the data back.
If you used Let's Encrypt, `sudo certbot delete --cert-name <domain>` removes the certificate.

## Troubleshooting

**Service status and logs**

```bash
systemctl status sigardapanel-api sigardapanel-agent sigardapanel-web nginx
journalctl -u sigardapanel-api -n 100 --no-pager
curl -s http://127.0.0.1:7700/api/v1/version     # {"data":{"service":"sigardapanel-api","version":"1.5.0"},...}
curl -s http://127.0.0.1:7700/api/v1/health
curl -s http://127.0.0.1:7710/health             # local agent
```

**`Unsupported OS` / `Unsupported CPU architecture`** - only Ubuntu/Debian on amd64/arm64 are supported.

**`Could not reach GitHub` / `Download failed`** - the server needs outbound HTTPS
to github.com. Without it, verify and copy the files yourself and use
`SIGARDA_ASSET_BASE=file:///...` (see [Verifying releases manually](#verifying-releases-manually)).

**`SIGNATURE VERIFICATION FAILED` / `CHECKSUM MISMATCH`** - the download is
corrupt or was tampered with. Retry once; if it persists, do not work around it
and report it.

**`SigardaPanel is already installed`** - use `sudo bash install.sh update`.

**`nginx configuration test failed`** - another nginx site conflicts (for example a
second `default_server`). The installer is meant for a fresh server.

**`Health check failed`** - check `journalctl -u sigardapanel-api -u sigardapanel-web`.
A port that is already taken is logged as `port 7700 already in use`:
`ss -tlnp | grep -E ':(7700|7710|7720)'`.

**Browser shows 502** - the API or web service is down; see the logs above.

**Redirect loop behind Cloudflare** - set the SSL mode to Full (strict).

**Forgot the admin password**

```bash
sudo -u sigardapanel env SIGARDAPANEL_DB_PATH=/var/lib/sigardapanel/sigardapanel.db \
     sigardapanel user reset-password <username> --direct     # prompts for the new password
```

Run it as the `sigardapanel` user so the database files keep their owner.

**Local agent token mismatch** - check with:

```bash
sudo bash -c 'set -a; . /etc/sigardapanel/panel.env; sigardapanel doctor --api-url http://127.0.0.1:7700'
```

The API copies `SIGARDAPANEL_AGENT_TOKEN` from `panel.env` into the database when
it starts. Make sure `panel.env` and `agent.env` contain the same token and
restart `sigardapanel-agent` and `sigardapanel-api`.

**"SIGARDAPANEL_ENCRYPTION_KEY missing or invalid: using derived fallback key"** -
the API found neither a valid key in `panel.env` nor an `encryption.key` file. Do
not simply add a new key if the panel already stores secrets: they were encrypted
with the fallback key and would become unreadable. Plan a re-encryption instead.

**Remote server stays offline** - from the panel host run
`curl -s http://<server>:7710/health`. No answer: the agent is not running or
port 7710 is blocked.

**Remote server shows "token rejected"** - the agent answers but its
`SIGARDAPANEL_AGENT_TOKEN` differs from the panel's (typo, or the token was
regenerated). Regenerate it under **Servers → Edit**, put it into the server's
`/etc/sigardapanel/agent.env` and run `systemctl restart sigardapanel-agent`.

**Install command: "cannot be used as the agent address"** - the server's
detected address is private (or you passed one). Pass its public address with
`SIGARDAPANEL_AGENT_IP`, or allow [private networks](#private-networks).

**Install command: "SSL certificate problem"** - the server does not trust the
panel's certificate (self-signed). Use a Let's Encrypt certificate for the panel,
or `SIGARDAPANEL_PANEL_CA` (see [option A](#option-a-install-command)).

## Building from source

Releases are built and signed with [`scripts/release.sh`](../scripts/release.sh)
(needs Go, Node.js/npm and the private signing key). For a development setup see
[GUIDE.md](GUIDE.md) and [AGENTS.md](../AGENTS.md). Production servers should
always use the signed releases and `install.sh`.
