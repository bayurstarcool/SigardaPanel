# Development Setup

This page is for working on SigardaPanel itself. To install a panel on a server,
use the signed releases and `install.sh` as described in [INSTALL.md](INSTALL.md) -
nothing on this page is needed for that.

## Requirements

- Go 1.25 or newer (see `go.mod`)
- Node.js 20+ and npm (only for the web app; releases ship it prebuilt)
- Linux; some agent features (nginx, ufw, systemd) only work on Ubuntu/Debian

## Backend (API + agent)

```bash
git clone https://github.com/bayurstarcool/SigardaPanel.git
cd SigardaPanel
go build -o sigardapanel ./cmd/sigardapanel

export SIGARDAPANEL_DEV=true                 # plain-HTTP cookies, no rate limits - development only
export SIGARDAPANEL_DB_PATH=$PWD/dev.db
./sigardapanel init --user admin --email admin@localhost   # prompts for a password (min. 8 chars)
./sigardapanel dev                            # API on :7700 + agent on :7710
```

No account is created automatically; `init` creates the first one (role `admin`).
`./sigardapanel install` is an interactive variant of the same for a dev checkout.
Without `SIGARDAPANEL_ENCRYPTION_KEY`, the API creates `encryption.key` next to
the dev database on first start.

## Web app

```bash
cd web
npm ci
npx vite dev --host 127.0.0.1 --port 7720     # talks to the API at http://127.0.0.1:7700
```

Set `SIGARDAPANEL_API_URL` if the API runs elsewhere. Checks: `npm run check`.

## Tests

```bash
go vet ./internal/... ./cmd/...
go test ./internal/...
bash -n installer/install.sh && shellcheck -S error installer/install.sh
```

## Releases

`scripts/release.sh <version>` runs the tests, builds the web bundle and the
`linux-amd64`/`linux-arm64` binaries, writes `SHA256SUMS` and signs it with the
release key (`--publish` creates the GitHub release). The installer only accepts
releases signed with that key.

`installer/install.sh` and the top-level `install.sh` must stay identical: the
README tells users to download the top-level copy.

See [AGENTS.md](../AGENTS.md) for the architecture, roles and CLI overview, and
[CLI.md](CLI.md) / [API.md](API.md) for the references.
