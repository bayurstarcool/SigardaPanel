# Deployment Guide

SigardaPanel deploys external applications through **Sites**. This guide covers
FastAPI applications with static HTML/JavaScript frontends, including the
`pdf-editor` repository pattern.

## Architecture

```text
Browser
  │ HTTPS
  ▼
Cloudflare DNS/proxy
  │
  ▼
Nginx :80/:443
  │ proxy_pass
  ▼
Application service 127.0.0.1:<dedicated-port>
```

SigardaPanel services:

| Service | Port | Role |
|---|---:|---|
| API | 7700 | Panel REST API and job creation |
| Agent | 7710 | Site, Nginx, SSL, Docker, and system actions |
| Web | 7720 | Panel frontend |
| Nginx | 80/443 | Public routing and TLS |

## Panel-First Workflow

1. Clone and inspect repository.
2. Identify runtime from README/manifests.
3. Install dependencies in an isolated environment.
4. Run syntax checks and a local smoke test.
5. Prepare a dedicated service and loopback port.
6. Configure frontend API calls as same-origin paths.
7. Configure Cloudflare token in SigardaPanel for the user creating the site.
8. Open **Sites → Create Site**.
9. Select `proxy` runtime for an already-running backend.
10. Set `proxy_pass_url` to `http://127.0.0.1:<port>`.
11. Enable **Cloudflare DNS** if DNS is managed by the configured Cloudflare account.
12. Wait for the `site.create` job to finish.
13. Issue SSL from the site SSL panel when needed.
14. Verify service, job, Nginx, DNS, and public URL.

Do not manually edit generated files under `/etc/nginx/sites-enabled/` during
normal operation. Use the site vhost/config features so the DB and generated
configuration remain consistent.

## Same-Origin FastAPI Pattern

If frontend calls API paths such as `/upload`, `/sample`, `/pdf/...`, or
`/font/...`, the backend must serve or proxy those paths on the same public
origin. Do not ship `http://localhost:<port>` in browser JavaScript.

Recommended backend:

```bash
cd /path/to/repository/backend
python3.11 -m venv .venv
.venv/bin/pip install -r requirements.txt
.venv/bin/uvicorn app:app --host 127.0.0.1 --port 18080
```

Recommended service properties:

```text
WorkingDirectory=/path/to/repository/backend
ExecStart=/path/to/repository/backend/.venv/bin/uvicorn app:app --host 127.0.0.1 --port 18080
Restart=on-failure
```

Keep service preparation separate from activation. Installing/enabling a
systemd unit, creating a panel site, changing DNS, reloading Nginx, and issuing
SSL are production side effects.

## Cloudflare DNS

Cloudflare configuration is scoped to the logged-in SigardaPanel user.
Automatic DNS provisioning checks that user's stored token.

Panel API sequence:

```http
GET  /api/v1/cloudflare/config
GET  /api/v1/cloudflare/zones
GET  /api/v1/cloudflare/zones/:id/dns
POST /api/v1/cloudflare/zones/:id/dns
POST /api/v1/sites
```

Example site payload:

```json
{
  "server_id": 1,
  "name": "pdf-editor",
  "domain": "pdfedit.sigarda.com",
  "runtime": "proxy",
  "proxy_pass_url": "http://127.0.0.1:18080",
  "cloudflare_provision": true
}
```

`DNS_PROBE_FINISHED_NXDOMAIN` means the public resolver found no record. Check
DNS before debugging the application:

```bash
dig pdfedit.sigarda.com SOA +noall +answer
dig +short pdfedit.sigarda.com @1.1.1.1
dig +short pdfedit.sigarda.com @8.8.8.8
```

If an existing installation has the old `cloudflare_config` table constraint,
run migrations. Migration 049 rebuilds the table without the legacy `id = 1`
constraint and preserves existing per-user tokens.

## Verification Matrix

```bash
# Panel
curl -fsS http://127.0.0.1:7700/health
curl -fsS http://127.0.0.1:7710/health

# Application
ss -tlnp | grep ':18080'
curl -fsS http://127.0.0.1:18080/
curl -fsS -o /dev/null -w '%{http_code}\n' -X POST http://127.0.0.1:18080/sample

# Generated Nginx
nginx -t
curl -sS -H 'Host: pdfedit.sigarda.com' http://127.0.0.1/

# DNS and public route
dig +short pdfedit.sigarda.com @1.1.1.1
curl -skI https://pdfedit.sigarda.com/
```

Representative application checks must include upload, sample load, apply/edit,
and download. A successful `/docs` response alone does not prove the editor
works.

## Rollback

Before replacing a live application artifact or generated configuration:

1. Capture current service status and active process command line.
2. Back up application data and current config.
3. Keep old artifact/service definition available.
4. Change one layer at a time.
5. Verify local origin before public DNS/TLS checks.

Never delete uploaded PDFs or storage while debugging. Quarantine questionable
files instead.
