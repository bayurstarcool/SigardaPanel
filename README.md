# SigardaPanel

Self-hosted VPS control plane: sites, nginx, databases, SSL, backups, Docker, firewall,
a web terminal and per-site isolation — one panel for one or many servers.

This repository only contains the **installer** and the **signed release binaries**.
The panel's source code is developed privately.

## Install

Supported: Ubuntu 24.04 (tested), Debian 12. Architectures: amd64, arm64. Run as root on a fresh server.

```bash
curl -fsSLO https://raw.githubusercontent.com/bayurstarcool/SigardaPanel/main/install.sh
less install.sh                      # read it first - it is a plain shell script
sudo bash install.sh
```

Options (environment variables):

| Variable | Meaning |
|---|---|
| `DOMAIN=panel.example.com` | Use a real hostname (TLS name + web origin). |
| `LETSENCRYPT_EMAIL=you@example.com` | With `DOMAIN`: request a Let's Encrypt certificate (DNS must already point here). |
| `ADMIN_USER`, `ADMIN_EMAIL`, `ADMIN_PASS` | Admin account (password is random and printed once if unset). |
| `INSTALL_FIREWALL=yes` | Allow SSH, 80 and 443 in `ufw` and enable it. Existing rules are never reset. |
| `SIGARDA_VERSION=v1.1.0` | Install a specific release instead of the latest. |

Without `DOMAIN` the panel is served over HTTPS with a self-signed certificate
(your browser will warn). Plain HTTP is only used to redirect to HTTPS.
Behind Cloudflare use SSL mode **Full (strict)**; "Flexible" would cause a redirect loop.

## Update

```bash
sudo bash install.sh update
```

The updater downloads and verifies the new release, backs up the binary, web bundle and
database to `/var/backups/sigardapanel-updates/<timestamp>`, switches over, runs a health
check and **rolls back automatically** if the new version does not come up healthy.

## Verifying releases

Every release contains `SHA256SUMS` and `SHA256SUMS.minisig`, signed with this
[minisign](https://jedisct1.github.io/minisign/) key:

```
RWTNXRQ/oh97GPfWH0tDwxbnMTe/WTZeoJdZtGTW3ESF0OpTSlNXpFXI
```

The installer verifies the signature and the checksums before it installs anything and has no
option to skip the check. To verify by hand:

```bash
V=v1.1.0
curl -fsSLO https://github.com/bayurstarcool/SigardaPanel/releases/download/$V/SHA256SUMS
curl -fsSLO https://github.com/bayurstarcool/SigardaPanel/releases/download/$V/SHA256SUMS.minisig
minisign -Vm SHA256SUMS -P RWTNXRQ/oh97GPfWH0tDwxbnMTe/WTZeoJdZtGTW3ESF0OpTSlNXpFXI
sha256sum -c --ignore-missing SHA256SUMS      # after downloading the binaries
```

## What gets installed

| Path | Purpose |
|---|---|
| `/usr/local/bin/sigardapanel` | API + agent binary |
| `/opt/sigardapanel/web` | web UI (Node.js, listens on 127.0.0.1:7720) |
| `/etc/sigardapanel/` | configuration and generated secrets (mode 600) |
| `/var/lib/sigardapanel/` | SQLite database |
| `sigardapanel-{api,agent,web}.service` | systemd units (API and web run unprivileged, sandboxed) |

All three services listen on 127.0.0.1 only; nginx is the sole public entry point.

## Security

See [SECURITY.md](SECURITY.md) to report a vulnerability.

## License

MIT - see [LICENSE](LICENSE).
