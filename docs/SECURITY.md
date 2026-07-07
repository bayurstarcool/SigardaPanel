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

- Passwords must use strong hashing: Argon2id or bcrypt
- Session tokens must be cryptographically random and expire
- API tokens must be stored hashed, not in plaintext
- Tokens are displayed only once at creation time
- 2FA/TOTP support planned for Beta/Stable releases
- Rate limiting on login, token creation, and password reset endpoints

## Role-Based Access Control

### Default Roles

| Role | Description |
|------|-------------|
| `super_admin` | Full system access |
| `admin` | Manage servers, sites, users (except owner actions) |
| `user` | Deploy, restart, view logs for assigned sites |

### Granular Permissions

- `server.read`, `server.create`, `server.delete`
- `site.read`, `site.create`, `site.update`, `site.delete`, `site.deploy`, `site.logs`
- `ssl.manage`
- `backup.create`, `backup.restore`
- `user.manage`
- `audit.read`

All permissions are validated by the backend before job creation. The agent must not accept tasks without valid signature/token from the backend.

## Agent Security

- Agent must authenticate all panel requests
- Use mTLS or strong tokens with rotation plan
- Agent exposes endpoints only to required networks
- Agent must validate task schema
- Agent must enforce allowed paths
- Agent must enforce allowed operations
- Agent must not execute raw shell strings from user input
- Agent must have per-operation timeouts
- Agent must log operation IDs and job IDs

## Command Execution

- Use `exec` with argument arrays, not `sh -c`, unless absolutely necessary
- If shell is required, use internal templates with validated variables
- Validate domains with strict parsers/regex
- Validate usernames, database names, and service names with character allowlists
- Validate paths using canonical path resolution and prefix checks against allowed roots
- Custom build commands must be disabled by default or require approval/allowlist
- Never interpolate user input into Nginx/Caddy configuration without escaping or safe templates

## Path Safety

- All file operations for a site must occur under the site root
- Reject symlink escapes if the file manager supports symlinks
- Reject `..`, null bytes, and absolute paths from user input
- Use canonical paths before destructive operations
- Delete operations must target registered resources, not arbitrary paths

## Secret Handling

- Secrets must be encrypted at rest when possible
- At minimum, never store secrets in plaintext in logs, audit, or jobs
- Mask secrets in UI and CLI: show only short prefix/suffix
- Site environment variables must not be fully exposed after storage
- Deploy tokens and Git tokens must be scoped and revocable
- Backup secrets and configurations must be encrypted or stored separately

## Audit Logging

### Mandatory Audit Events

- Login failures and successes
- User creation, update, and deletion
- Server creation and deletion
- Agent registration
- Site creation, update, and deletion
- Deployments
- SSL issuance, renewal, and failures
- Backup and restore operations
- Service restart and reload
- API token creation and revocation

### Audit Payload Safety

- Never store passwords, tokens, or private keys
- Never store full environment variable values
- Store request IDs and job IDs for correlation

## Dashboard Security

- Enforce CSRF protection when using cookie sessions
- Set cookies with `HttpOnly`, `Secure`, and `SameSite` attributes
- Escape all user-generated output
- Never trust client-provided role/permission values
- Implement security headers
- Avoid storing long-term tokens in localStorage when possible

## Backup and Restore Security

- Restore operations require specific permissions
- Overwrite restores require explicit confirmation
- Backup archives must be validated before extraction
- Extraction must prevent zip-slip and tar path traversal
- Remote backup credentials must be masked

## Network Security

- Panel HTTPS is mandatory for production
- Agent endpoints should not be publicly exposed
- IP allowlist support for admin/agent endpoints planned for future
- Webhook endpoints must have secret/signature validation
- Prevent SSRF from URL inputs with allowlists and private network blocking

## Secure Defaults

- Destructive actions disabled without confirmation
- Custom build commands disabled by default without sandbox/approval
- Default role for new users is `user`, not `admin`
- Site isolation enabled by default
- Secret masking enabled by default
- Audit logging enabled by default
