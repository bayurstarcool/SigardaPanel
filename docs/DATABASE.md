# Database Schema

This document defines the database schema for SigardaPanel.

## Target Database

- **Engine:** SQLite with WAL mode
- **IDs:** UUID/ULID strings recommended for public IDs
- **Timestamps:** Must include timezone information
- **Soft delete:** Required for important resources
- **Migrations:** Must be versioned

## Conventions

| Convention | Description |
|------------|-------------|
| Primary key | `id` |
| Public ID prefix | Optional: `usr_`, `srv_`, `site_`, `job_` |
| Timestamps | `created_at`, `updated_at`, `deleted_at` (nullable for soft delete) |
| Status | Enum string or constrained text |
| Secrets | Never stored in plaintext |

## Core Tables

### `users`

Stores user accounts.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | text | PRIMARY KEY |
| `email` | text | UNIQUE NOT NULL |
| `name` | text | NOT NULL |
| `password_hash` | text | NOT NULL |
| `status` | text | NOT NULL (`active`, `disabled`) |
| `last_login_at` | timestamptz | NULLABLE |
| `created_at` | timestamptz | NOT NULL |
| `updated_at` | timestamptz | NOT NULL |
| `deleted_at` | timestamptz | NULLABLE |

**Indexes:** `lower(email)` (unique), `status`

### `roles`

Default and custom roles.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | text | PRIMARY KEY |
| `name` | text | UNIQUE NOT NULL |
| `description` | text | NULLABLE |
| `is_system` | boolean | NOT NULL DEFAULT false |
| `created_at` | timestamptz | NOT NULL |
| `updated_at` | timestamptz | NOT NULL |

**Default roles:** `super_admin`, `admin`, `user`

### `permissions`

Granular permission definitions.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | text | PRIMARY KEY |
| `key` | text | UNIQUE NOT NULL |
| `description` | text | NULLABLE |
| `created_at` | timestamptz | NOT NULL |

### `role_permissions`

Role-permission relationships.

| Column | Type | Constraints |
|--------|------|-------------|
| `role_id` | text | REFERENCES `roles(id)` |
| `permission_id` | text | REFERENCES `permissions(id)` |

**Primary key:** (`role_id`, `permission_id`)

### `user_roles`

Global user-role assignments.

| Column | Type | Constraints |
|--------|------|-------------|
| `user_id` | text | REFERENCES `users(id)` |
| `role_id` | text | REFERENCES `roles(id)` |
| `created_at` | timestamptz | NOT NULL |

**Primary key:** (`user_id`, `role_id`)

### `sessions`

Dashboard sessions.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | text | PRIMARY KEY |
| `user_id` | text | REFERENCES `users(id)` |
| `token_hash` | text | UNIQUE NOT NULL |
| `user_agent` | text | NULLABLE |
| `ip_address` | text | NULLABLE |
| `expires_at` | timestamptz | NOT NULL |
| `revoked_at` | timestamptz | NULLABLE |
| `created_at` | timestamptz | NOT NULL |

**Indexes:** `user_id`, `token_hash`, `expires_at`

### `api_tokens`

CLI and API tokens.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | text | PRIMARY KEY |
| `user_id` | text | REFERENCES `users(id)` |
| `name` | text | NOT NULL |
| `token_hash` | text | UNIQUE NOT NULL |
| `scopes` | text | NOT NULL DEFAULT `[]` |
| `last_used_at` | timestamptz | NULLABLE |
| `expires_at` | timestamptz | NULLABLE |
| `revoked_at` | timestamptz | NULLABLE |
| `created_at` | timestamptz | NOT NULL |

**Indexes:** `user_id`, `token_hash`, `revoked_at`

## Server and Agent Tables

### `servers`

Managed servers/VPS instances.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | text | PRIMARY KEY |
| `name` | text | UNIQUE NOT NULL |
| `host` | text | NULLABLE |
| `provider` | text | NULLABLE |
| `status` | text | NOT NULL (`pending`, `online`, `offline`, `error`, `disabled`) |
| `os_name` | text | NULLABLE |
| `os_version` | text | NULLABLE |
| `arch` | text | NULLABLE |
| `last_seen_at` | timestamptz | NULLABLE |
| `created_by` | text | REFERENCES `users(id)` |
| `created_at` | timestamptz | NOT NULL |
| `updated_at` | timestamptz | NOT NULL |
| `deleted_at` | timestamptz | NULLABLE |

**Indexes:** `status`, `last_seen_at`

### `agents`

Agent identity per server.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | text | PRIMARY KEY |
| `server_id` | text | REFERENCES `servers(id)` |
| `version` | text | NULLABLE |
| `fingerprint` | text | UNIQUE NOT NULL |
| `auth_hash` | text | NULLABLE |
| `status` | text | NOT NULL (`pending`, `active`, `revoked`) |
| `registered_at` | timestamptz | NULLABLE |
| `last_heartbeat_at` | timestamptz | NULLABLE |
| `created_at` | timestamptz | NOT NULL |
| `updated_at` | timestamptz | NOT NULL |

**Indexes:** `server_id`, `fingerprint`, `status`

### `agent_bootstrap_tokens`

One-time tokens for agent registration.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | text | PRIMARY KEY |
| `server_id` | text | REFERENCES `servers(id)` |
| `token_hash` | text | UNIQUE NOT NULL |
| `expires_at` | timestamptz | NOT NULL |
| `used_at` | timestamptz | NULLABLE |
| `created_by` | text | REFERENCES `users(id)` |
| `created_at` | timestamptz | NOT NULL |

### `server_capabilities`

Latest capability report.

| Column | Type | Constraints |
|--------|------|-------------|
| `server_id` | text | PRIMARY KEY REFERENCES `servers(id)` |
| `capabilities` | text | NOT NULL |
| `reported_at` | timestamptz | NOT NULL |

## Site Tables

### `sites`

Managed sites/applications.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | text | PRIMARY KEY |
| `server_id` | text | REFERENCES `servers(id)` |
| `primary_domain` | text | NOT NULL |
| `runtime` | text | NOT NULL (`static`, `node`, `go`, `php`, `python`, `docker`) |
| `status` | text | NOT NULL (`provisioning`, `active`, `disabled`, `error`, `deleting`) |
| `site_user` | text | NULLABLE |
| `root_path` | text | NULLABLE |
| `current_release_path` | text | NULLABLE |
| `ssl_enabled` | boolean | NOT NULL DEFAULT false |
| `created_by` | text | REFERENCES `users(id)` |
| `created_at` | timestamptz | NOT NULL |
| `updated_at` | timestamptz | NOT NULL |
| `deleted_at` | timestamptz | NULLABLE |

**Indexes:** `server_id`, `primary_domain`, `status`, `runtime`

**Constraint:** Unique active domain via partial unique index on `lower(primary_domain)` where `deleted_at` is null.

### `site_domains`

Additional domains and aliases.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | text | PRIMARY KEY |
| `site_id` | text | REFERENCES `sites(id)` |
| `domain` | text | NOT NULL |
| `type` | text | NOT NULL (`primary`, `alias`, `redirect`) |
| `created_at` | timestamptz | NOT NULL |

### `site_env_vars`

Environment variable metadata per site.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | text | PRIMARY KEY |
| `site_id` | text | REFERENCES `sites(id)` |
| `key` | text | NOT NULL |
| `value_ref` | text | NOT NULL |
| `is_secret` | boolean | NOT NULL DEFAULT true |
| `created_at` | timestamptz | NOT NULL |
| `updated_at` | timestamptz | NOT NULL |

**Constraint:** UNIQUE (`site_id`, `key`)

### `site_permissions`

User access to specific sites.

| Column | Type | Constraints |
|--------|------|-------------|
| `site_id` | text | REFERENCES `sites(id)` |
| `user_id` | text | REFERENCES `users(id)` |
| `role_id` | text | REFERENCES `roles(id)` |
| `created_at` | timestamptz | NOT NULL |

**Primary key:** (`site_id`, `user_id`, `role_id`)

## Deployment Tables

### `deployments`

Deployment history.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | text | PRIMARY KEY |
| `site_id` | text | REFERENCES `sites(id)` |
| `job_id` | text | NULLABLE REFERENCES `jobs(id)` |
| `source_type` | text | NOT NULL (`git`, `artifact`, `manual`) |
| `source_ref` | text | NULLABLE |
| `commit_sha` | text | NULLABLE |
| `release_path` | text | NULLABLE |
| `status` | text | NOT NULL (`pending`, `running`, `succeeded`, `failed`, `rolled_back`) |
| `started_at` | timestamptz | NULLABLE |
| `finished_at` | timestamptz | NULLABLE |
| `created_by` | text | REFERENCES `users(id)` |
| `created_at` | timestamptz | NOT NULL |

### `deployment_artifacts`

Deployment artifact metadata.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | text | PRIMARY KEY |
| `deployment_id` | text | REFERENCES `deployments(id)` |
| `storage_path` | text | NOT NULL |
| `checksum` | text | NULLABLE |
| `size_bytes` | integer | NULLABLE |
| `created_at` | timestamptz | NOT NULL |

## Job Tables

### `jobs`

Async job queue.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | text | PRIMARY KEY |
| `type` | text | NOT NULL |
| `status` | text | NOT NULL (`queued`, `running`, `succeeded`, `failed`, `canceled`) |
| `priority` | integer | NOT NULL DEFAULT 0 |
| `target_type` | text | NULLABLE |
| `target_id` | text | NULLABLE |
| `server_id` | text | NULLABLE REFERENCES `servers(id)` |
| `payload` | text | NOT NULL DEFAULT `{}` |
| `result` | text | NULLABLE |
| `error_code` | text | NULLABLE |
| `error_message` | text | NULLABLE |
| `attempts` | integer | NOT NULL DEFAULT 0 |
| `max_attempts` | integer | NOT NULL DEFAULT 1 |
| `available_at` | timestamptz | NOT NULL |
| `started_at` | timestamptz | NULLABLE |
| `finished_at` | timestamptz | NULLABLE |
| `created_by` | text | NULLABLE REFERENCES `users(id)` |
| `created_at` | timestamptz | NOT NULL |
| `updated_at` | timestamptz | NOT NULL |

**Indexes:** (`status`, `priority`, `available_at`), `server_id`, (`target_type`, `target_id`), `created_at`

**Note:** Payload must never contain plaintext secrets.

### `job_logs`

Per-job log entries.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | integer | PRIMARY KEY AUTOINCREMENT |
| `job_id` | text | REFERENCES `jobs(id)` |
| `level` | text | NOT NULL (`debug`, `info`, `warn`, `error`) |
| `message` | text | NOT NULL |
| `metadata` | text | NULLABLE |
| `created_at` | timestamptz | NOT NULL |

**Index:** (`job_id`, `created_at`)

## SSL Tables

### `ssl_certificates`

SSL certificate metadata.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | text | PRIMARY KEY |
| `site_id` | text | REFERENCES `sites(id)` |
| `domain` | text | NOT NULL |
| `issuer` | text | NULLABLE |
| `status` | text | NOT NULL (`pending`, `active`, `expired`, `failed`, `revoked`) |
| `not_before` | timestamptz | NULLABLE |
| `not_after` | timestamptz | NULLABLE |
| `last_renewed_at` | timestamptz | NULLABLE |
| `created_at` | timestamptz | NOT NULL |
| `updated_at` | timestamptz | NOT NULL |

**Indexes:** `site_id`, `domain`, `not_after`, `status`

## Backup Tables

### `backups`

Backup metadata.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | text | PRIMARY KEY |
| `site_id` | text | NULLABLE REFERENCES `sites(id)` |
| `server_id` | text | NULLABLE REFERENCES `servers(id)` |
| `job_id` | text | NULLABLE REFERENCES `jobs(id)` |
| `type` | text | NOT NULL (`site`, `database`, `full`) |
| `status` | text | NOT NULL (`pending`, `running`, `succeeded`, `failed`, `deleted`) |
| `storage` | text | NOT NULL (`local`, `s3`, `sftp`) |
| `storage_path` | text | NULLABLE |
| `checksum` | text | NULLABLE |
| `size_bytes` | integer | NULLABLE |
| `created_by` | text | REFERENCES `users(id)` |
| `created_at` | timestamptz | NOT NULL |
| `completed_at` | timestamptz | NULLABLE |
| `expires_at` | timestamptz | NULLABLE |

**Indexes:** `site_id`, `server_id`, `status`, `created_at`, `expires_at`

## Secret Tables

### `secrets`

Secret metadata and encrypted values.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | text | PRIMARY KEY |
| `scope_type` | text | NOT NULL (`global`, `server`, `site`, `user`) |
| `scope_id` | text | NULLABLE |
| `key` | text | NOT NULL |
| `encrypted_value` | blob | NOT NULL |
| `value_hash` | text | NULLABLE |
| `created_by` | text | NULLABLE REFERENCES `users(id)` |
| `created_at` | timestamptz | NOT NULL |
| `updated_at` | timestamptz | NOT NULL |

**Indexes:** (`scope_type`, `scope_id`), `key`

**Constraint:** UNIQUE (`scope_type`, `scope_id`, `key`)

## Audit Tables

### `audit_logs`

Audit events.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | integer | PRIMARY KEY AUTOINCREMENT |
| `request_id` | text | NULLABLE |
| `actor_id` | text | NULLABLE REFERENCES `users(id)` |
| `actor_type` | text | NOT NULL (`user`, `token`, `agent`, `system`) |
| `action` | text | NOT NULL |
| `target_type` | text | NULLABLE |
| `target_id` | text | NULLABLE |
| `server_id` | text | NULLABLE REFERENCES `servers(id)` |
| `status` | text | NOT NULL (`success`, `failure`) |
| `ip_address` | text | NULLABLE |
| `user_agent` | text | NULLABLE |
| `metadata` | text | NULLABLE |
| `created_at` | timestamptz | NOT NULL |

**Indexes:** `actor_id`, `action`, (`target_type`, `target_id`), `server_id`, `status`, `created_at`

**Note:** Metadata must be masked from secrets.

## Metrics Tables

### `server_metrics`

Lightweight per-server metrics.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | integer | PRIMARY KEY AUTOINCREMENT |
| `server_id` | text | REFERENCES `servers(id)` |
| `cpu_percent` | real | NULLABLE |
| `memory_used_bytes` | integer | NULLABLE |
| `memory_total_bytes` | integer | NULLABLE |
| `disk_used_bytes` | integer | NULLABLE |
| `disk_total_bytes` | integer | NULLABLE |
| `load_average` | real | NULLABLE |
| `created_at` | timestamptz | NOT NULL |

**Index:** (`server_id`, `created_at`)

**Note:** Retention policy required to prevent table bloat.

### `site_metrics`

Lightweight per-site metrics.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | integer | PRIMARY KEY AUTOINCREMENT |
| `site_id` | text | REFERENCES `sites(id)` |
| `cpu_percent` | real | NULLABLE |
| `memory_bytes` | integer | NULLABLE |
| `requests_count` | integer | NULLABLE |
| `error_count` | integer | NULLABLE |
| `created_at` | timestamptz | NOT NULL |

**Index:** (`site_id`, `created_at`)

## Notification Tables

### `notification_channels`

Alert channels.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | text | PRIMARY KEY |
| `name` | text | NOT NULL |
| `type` | text | NOT NULL (`telegram`, `discord`, `email`, `webhook`) |
| `config_secret_ref` | text | NULLABLE |
| `enabled` | boolean | NOT NULL DEFAULT true |
| `created_by` | text | REFERENCES `users(id)` |
| `created_at` | timestamptz | NOT NULL |
| `updated_at` | timestamptz | NOT NULL |

### `alerts`

Alert events.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | text | PRIMARY KEY |
| `type` | text | NOT NULL |
| `severity` | text | NOT NULL (`info`, `warning`, `critical`) |
| `target_type` | text | NULLABLE |
| `target_id` | text | NULLABLE |
| `message` | text | NOT NULL |
| `resolved_at` | timestamptz | NULLABLE |
| `created_at` | timestamptz | NOT NULL |
