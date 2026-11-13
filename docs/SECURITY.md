# Security

SigardaPanel was audited at v1.0.0 and reviewed again at v1.2.0. This document describes the threat model, controls, and known limitations. Where a control depends on how the server was set up, the behaviour of the official installer (`install.sh`, see [INSTALL.md](INSTALL.md)) is noted.

## Threat Model

### In scope (we defend against)

1. **Brute force** login → rate-limited (10 attempts / 15min per IP)
2. **Stolen password** → Argon2id hash, optional 2FA
3. **Token leakage** → 64-char session tokens, hashed in DB, revocable
4. **CSRF** → browser sessions use SameSite=Strict cookies; API clients use Bearer headers
5. **Path traversal** in file manager → `safePath()` validates
6. **Command injection** in agent → allowlist, args array (no shell)
7. **SQL injection** → all queries parameterized
8. **XSS** in SvelteKit → auto-escaping; session token lives in an HttpOnly cookie, never in JS (since v1.2.0)
9. **Network sniffing** → TLS 1.2/1.3, HSTS
10. **Privilege escalation** → role hierarchy + ownership checks
11. **Brute force on SSH** → fail2ban + key-only auth
12. **Cloudflare loop** → anti-loop with `CF-Ray` detection

### Out of scope (user must mitigate)

1. **Compromise of server root** — game over for any software
2. **DNS hijack** — use DNSSEC if available
3. **Insider threat** — admin who can impersonate any user
4. **Supply chain** on `go.sum` / `package-lock.json` (use `go mod verify` / `npm ci`)

---

## Controls

### Authentication

| Layer | Mechanism |
|---|---|
| **Password hash** | Argon2id (m=64MB, t=1, p=2) |
| **Session token** | 64-char random, SHA-256 hashed in DB |
| **Session TTL** | 7 days (90 with remember_me) |
| **2FA** | TOTP SHA-1 / 6 digits / 30s window |
| **Recovery codes** | 8 one-time codes (stored hashed) |
| **Service tokens** | Scope-based: `admin`, `user`, `sites`, `*` |
| **Rate limit** | Login: 10 / 15 min/IP · Register: Strict |

### Transport

- **TLS 1.2/1.3** only (no TLS 1.0/1.1)
- **HSTS** `max-age=31536000` (installer nginx site)
- **Let's Encrypt** via DNS-01 (Cloudflare) or HTTP-01
- **Self-signed** as fallback (browsers warn, but functional)

### HTTP Headers

```
X-Frame-Options: DENY
X-Content-Type-Options: nosniff
Referrer-Policy: strict-origin-when-cross-origin
Strict-Transport-Security: max-age=31536000
```

### Network & Anti-DDoS

- **All backend services bind 127.0.0.1** — only nginx exposes 80/443
- **Rate limiting** in nginx (installer): 15 req/s per IP on the API, 5 req/min on the login endpoint; the API adds its own login limiter
- **UFW** default deny, allows SSH/80/443 - the installer configures it only with `INSTALL_FIREWALL=yes`
- **fail2ban** (sshd, nginx jails), SYN cookies and connection limits are recommended host hardening; the installer does not configure them (the panel's Security Shield / Fail2ban pages can)
- **Cloudflare:** use SSL mode Full (strict); "Flexible" loops on the installer's HTTP → HTTPS redirect

### System Hardening

| Service | User | Sandbox |
|---|---|---|
| API | `sigardapanel` | `ReadWritePaths` only to DB/logs |
| Agent | `root` | Required for apt/systemctl |
| Web | `sigardapanel` | Read-only on web/ |

All services use `NoNewPrivileges`, `ProtectSystem=strict` (api/web) or full access (agent), `ProtectKernelTunables`, `LimitNOFILE=65536`.

### Agent Command Allowlist

Only 50+ pre-approved binaries may execute. Each is invoked with args array (no shell interpretation):

```go
// internal/executor/executor.go
"systemctl", "service", "nginx", "certbot", "git", "tar", "tail",
"openssl", "mysql", "mysqldump", "psql", "pg_dump", "stat", "mkdir",
"echo", "aws", "sh", "bash", "cp", "mv", "find", "du", "cat", "wc",
"head", "sleep", "curl", "apt-get", "apt", "apt-cache", "yum", "dnf",
"apk", "docker", "node", "npm", "pip", "pip3", "go", "redis-server",
"redis-cli", "memcached", "ufw", "supervisord", "mariadb", "php*",
"fail2ban-client", "which", "test", "make", "gcc", "python3", "python"
```

To add a command: edit `internal/executor/executor.go` and rebuild.

---

## Audit Log

All state-changing actions recorded in `audit_logs` table:

```sql
SELECT user_id, action, resource, resource_id, details, created_at
FROM audit_logs
ORDER BY created_at DESC LIMIT 50;
```

Actions tracked: `site_create`, `site_delete`, `site_update`, `user_create`, `user_delete`, `user_update`, `site_transfer`, `config_updated`, `backup_*`, `stack_*`, etc.

---

## Known Issues / Limitations

### Reviewed at v1.2.0

| Issue | Severity | Mitigation |
|---|---|---|
| ~~Token stored in `localStorage`~~ | — | **Resolved in v1.2.0**: HttpOnly + Secure + SameSite=Strict cookie |
| No CSP header | Low | Recommended: `default-src 'self'` |
| Public registration | Low | Closed by default since v1.6.0; open with `SIGARDAPANEL_ALLOW_REGISTRATION=true` |
| `test_db.php` may exist on web server (172.104.62.193) | High (out of scope) | Remove from that host |
| Self-signed cert warning in browsers | Low | Use Let's Encrypt with `DOMAIN` + `CF_TOKEN` |
| HTTP still works for direct IP (anti-loop only checks CF-Ray) | Low | `iptables` block if not needed |

### Future hardening

- [x] Migrate session token to HttpOnly cookie (done in v1.2.0)
- [ ] Add CSP header
- [ ] Add 2FA enforcement for admin role (currently optional)
- [ ] Add per-IP session limit
- [ ] Add WebAuthn (passkey) support
- [ ] Add OIDC SSO

---

## Reporting Vulnerabilities

File issues privately. Do not disclose publicly until patched.

---

## References

- `internal/apihttp/middleware.go` — auth middleware
- `internal/auth/password.go` — Argon2id implementation
- `internal/executor/executor.go` — command allowlist
- `internal/apihttp/filemanager.go` — `safePath()` (path traversal guard)
- `internal/agenthttp/stack*.go` — stack package operations: validated versions/extension names, apt
  argument arrays (no shell interpolation of input), removal simulated first (`apt-get -s`) so no other
  package goes with it; `internal/apihttp/stack_manage.go` — site/panel-host guards and audit
- `internal/agenthttp/router.go` — `requireAgentAuth` (constant-time compare)
