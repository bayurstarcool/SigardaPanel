# Security Model

This document defines the security baseline for the SigardaPanel VPS management system, including the panel server, agent, CLI, and dashboard.

## Core Principles

- The backend is the policy authority
- The agent is an executor, not a permission decision-maker
- All input is treated as untrusted
- All server operations must be audited
- Secrets must never appear in logs, UI, CLI output, or audit payloads
- Destructive operations require guards and confirmation

## Threat Model

### Protected Assets

- Admin and user accounts
- API tokens and session tokens
- Server/VPS root access
- Site files and databases
- Environment variables and deployment secrets
- SSL private keys
- Backup archives
- Audit logs

### Threat Actors

- Regular users attempting privilege escalation
- Operators with limited access trying to access other sites
- External attackers on dashboard/API
- Attackers who compromise a site application
- Attackers who steal CLI/API tokens
- Compromised agents or servers

### Primary Risks

- Command injection
- Path traversal
- Privilege escalation
- Secret leakage
- CSRF/XSS in dashboard
- Broken access control
- Reverse proxy configuration injection
- Unsafe backup/restore overwrites
- SSRF from fetch/deploy webhook features

## Authentication

### Implemented

- **Password hashing:** Argon2id or bcrypt
- **Session tokens:** Cryptographically random, expire after configured period
- **Service tokens:** Stored hashed, for agent-to-panel communication
- **Token display:** Shown only once at creation time
- **2FA/TOTP:** Fully implemented with recovery codes
- **Rate limiting:** Login (10 attempts/15min per IP), API (100 req/min per IP)
- **Dev mode bypass:** Rate limiters disabled when `SIGARDAPANEL_DEV=true`

## Role-Based Access Control

### Implemented Roles

| Role | Description |
|------|-------------|
| `super_admin` | Full system access, sees all sites and users |
| `admin` | Manage servers, sites, users (except owner actions) |
| `user` | Deploy, restart, view logs for owned/assigned sites |

### Site Isolation Model

- **Owner-based:** User who creates a site becomes the owner
- **Assignment:** Owner can assign other users with `read`, `write`, or `admin` permission
- **Linux user:** Each panel user gets a matching Linux system user
- **Per-site terminal:** SSH terminal runs as `system_user` (not root)

### Permission Matrix

| Action | super_admin | Owner | admin (assigned) | write (assigned) | read (assigned) |
|--------|:-----------:|:-----:|:----------------:|:----------------:|:---------------:|
| View site | ✅ | ✅ | ✅ | ✅ | ✅ |
| Edit config | ✅ | ✅ | ✅ | ✅ | ❌ |
| Deploy | ✅ | ✅ | ✅ | ✅ | ❌ |
| Delete site | ✅ | ✅ | ❌ | ❌ | ❌ |
| Manage users | ✅ | ✅ | ❌ | ❌ | ❌ |

## Agent Security

### Implemented

- Agent must authenticate all panel requests via registration token
- Agent exposes endpoints only to required networks
- Agent validates task schema
- Agent enforces allowed paths
- Agent enforces allowed operations
- Agent does not execute raw shell strings from user input
- Agent has per-operation timeouts
- Agent logs operation IDs and job IDs

## Command Execution

### Implemented

- Use `exec` with argument arrays, not `sh -c`, unless absolutely necessary
- If shell is required, use internal templates with validated variables
- Domains validated with strict regex
- Usernames, database names, service names validated with character allowlists
- Paths validated using canonical path resolution and prefix checks
- Custom build commands require approval/allowlist
- Nginx configuration uses safe templates with escaped variables

## Path Safety

### Implemented

- All file operations for a site occur under the site root
- Symlink escapes rejected
- `..`, null bytes, and absolute paths rejected from user input
- Canonical paths used before destructive operations
- Delete operations target registered resources, not arbitrary paths

## Secret Handling

### Implemented

- Secrets encrypted at rest when possible
- Secrets never stored in plaintext in logs, audit, or jobs
- Masked in UI and CLI: show only short prefix/suffix
- Deploy tokens and Git tokens scoped and revocable
- Backup secrets and configurations stored separately

## Audit Logging

### Mandatory Events (Implemented)

- Login failures and successes
- User creation, update, and deletion
- Server creation and deletion
- Agent registration
- Site creation, update, and deletion
- Deployments
- SSL issuance, renewal, and failures
- Backup and restore operations
- Service restart and reload

### Audit Payload Safety

- Never store passwords, tokens, or private keys
- Never store full environment variable values
- Store request IDs and job IDs for correlation

## Dashboard Security

### Implemented

- CSRF protection via cookie sessions
- Cookies set with `HttpOnly`, `Secure`, and `SameSite` attributes
- All user-generated output escaped
- Client-provided role/permission values never trusted
- Security headers set

## Backup and Restore Security

### Implemented

- Restore operations require specific permissions
- Backup archives validated before extraction
- Remote backup credentials masked
- Deduplication prevents concurrent backup conflicts
- Stuck jobs (>30min) auto-marked as failed

## Network Security

### Implemented

- Agent endpoints not publicly exposed
- Webhook endpoints have secret/signature validation
- IP restrictions per site
- Basic auth per site
- Blocked IPs per site

## IP Restrictions

### Implemented

- Per-site IP restrictions (CRUD)
- Block/allow specific IPs per site

## Secure Defaults

- Destructive actions disabled without confirmation
- Custom build commands disabled by default without sandbox/approval
- Default role for new users is `user`, not `admin`
- Site isolation enabled by default
- Secret masking enabled by default
- Audit logging enabled by default

## License & Feature Gating

- Feature flags per tier (free, pro, etc.)
- License activation/deactivation
- Server limits per license tier
- Feature-gated endpoints (planned)
