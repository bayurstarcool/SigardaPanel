# Security

SigardaPanel v1.0.0 has been audited. This document describes the threat model, controls, and known limitations.

## Threat Model

### In scope (we defend against)

1. **Brute force** login → rate-limited (10 attempts / 15min per IP)
2. **Stolen password** → Argon2id hash, optional 2FA
3. **Token leakage** → 64-char session tokens, hashed in DB, revocable
4. **CSRF** → bearer token auth, no cookies
5. **Path traversal** in file manager → `safePath()` validates
6. **Command injection** in agent → allowlist, args array (no shell)
7. **SQL injection** → all queries parameterized
8. **XSS** in SvelteKit → auto-escaping + `localStorage` token (single domain)
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
- **HSTS** `max-age=31536000; includeSubDomains`
- **Let's Encrypt** via DNS-01 (Cloudflare) or HTTP-01
- **Self-signed** as fallback (browsers warn, but functional)

### HTTP Headers

```
X-Frame-Options: DENY
X-Content-Type-Options: nosniff
Referrer-Policy: no-referrer-when-downgrade
Strict-Transport-Security: max-age=31536000; includeSubDomains
```

### Network & Anti-DDoS

- **All backend services bind 127.0.0.1** — only nginx exposes 80/443
- **UFW** default deny, allows 22/80/443
- **fail2ban** with 3 jails: sshd, nginx-http-auth, nginx-botsearch
- **Anti-DDoS & Slowloris:** SYN-cookies enabled (`net.ipv4.tcp_syncookies=1`), Nginx client timeouts (15s), per-IP connection limits (50 max), and request rate limiting
- **Anti-loop:** HTTP → HTTPS redirect skipped when `CF-Ray` header present

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

### v1.0.0

| Issue | Severity | Mitigation |
|---|---|---|
| Token stored in `localStorage` (XSS-amplified) | Medium | SvelteKit auto-escapes; HSTS; CSP not yet set |
| No CSP header | Low | Recommended: `default-src 'self'` |
| Registration endpoint blocked at nginx (403) | Low | By design — admin creates users |
| `test_db.php` may exist on web server (172.104.62.193) | High (out of scope) | Remove from that host |
| Self-signed cert warning in browsers | Low | Use Let's Encrypt with `DOMAIN` + `CF_TOKEN` |
| HTTP still works for direct IP (anti-loop only checks CF-Ray) | Low | `iptables` block if not needed |

### Future hardening

- [ ] Migrate session token to HttpOnly cookie (eliminate XSS-amplified localStorage theft)
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
- `internal/agenthttp/stack.go` — `managePHP/Nginx/etc` (no shell)
- `internal/agenthttp/router.go` — `requireAgentAuth` (constant-time compare)
