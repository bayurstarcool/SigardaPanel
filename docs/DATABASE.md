# Database Schema

This document defines the actual database schema for SigardaPanel v0.5.x.

## Target Database

- **Engine:** SQLite with WAL mode
- **IDs:** `INTEGER PRIMARY KEY AUTOINCREMENT`
- **Timestamps:** `DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP`
- **Migrations:** Versioned via `schema_migrations` table

## Conventions

| Convention | Description |
|------------|-------------|
| Primary key | `id` (integer, autoincrement) |
| Timestamps | `created_at`, `updated_at` |
| Status | Enum string or constrained text |
| Secrets | Encrypted or hashed at rest |

## Core Tables

### `schema_migrations`

Tracks applied migrations.

| Column | Type | Constraints |
|--------|------|-------------|
| `version` | integer | PRIMARY KEY |
| `applied_at` | datetime | NOT NULL DEFAULT CURRENT_TIMESTAMP |

### `users`

Stores user accounts.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | integer | PRIMARY KEY AUTOINCREMENT |
| `username` | text | UNIQUE NOT NULL |
| `email` | text | UNIQUE NOT NULL |
| `password_hash` | text | NOT NULL |
| `role` | text | NOT NULL DEFAULT 'user' |
| `created_at` | datetime | NOT NULL DEFAULT CURRENT_TIMESTAMP |
| `updated_at` | datetime | NOT NULL DEFAULT CURRENT_TIMESTAMP |
| `created_by` | integer | REFERENCES users(id) |
| `disk_limit` | integer | NULLABLE |
| `max_sites` | integer | NULLABLE |
| `plan_id` | integer | REFERENCES plans(id) |

**Roles:** `super_admin`, `admin`, `user`

**Indexes:** `username` (unique), `email` (unique), `created_by`

### `sessions`

Dashboard sessions.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | integer | PRIMARY KEY AUTOINCREMENT |
| `user_id` | integer | NOT NULL REFERENCES users(id) ON DELETE CASCADE |
| `token_hash` | text | UNIQUE NOT NULL |
| `expires_at` | datetime | NOT NULL |
| `created_at` | datetime | NOT NULL DEFAULT CURRENT_TIMESTAMP |
| `revoked_at` | datetime | NULLABLE |
| `impersonated_by` | integer | REFERENCES users(id) |

**Indexes:** `token_hash` (unique), `user_id`

### `service_tokens`

Service-to-service auth tokens (agent, metrics ingestion).

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | integer | PRIMARY KEY AUTOINCREMENT |
| `name` | text | UNIQUE NOT NULL |
| `key_hash` | text | UNIQUE NOT NULL |
| `scopes` | text | NOT NULL DEFAULT '*' |
| `created_at` | datetime | NOT NULL DEFAULT CURRENT_TIMESTAMP |
| `expires_at` | datetime | NULLABLE |
| `last_used_at` | datetime | NULLABLE |

**Indexes:** `key_hash` (unique)

### `registration_tokens`

One-time tokens for agent registration.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | integer | PRIMARY KEY AUTOINCREMENT |
| `token_hash` | text | NOT NULL |
| `created_by` | integer | NULLABLE |
| `expires_at` | datetime | NOT NULL |
| `used_at` | datetime | NULLABLE |
| `created_at` | datetime | NOT NULL DEFAULT CURRENT_TIMESTAMP |

**Indexes:** `token_hash`

---

## Server and Agent Tables

### `servers`

Managed servers/VPS instances.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | integer | PRIMARY KEY AUTOINCREMENT |
| `name` | text | NOT NULL |
| `hostname` | text | NOT NULL |
| `ip_address` | text | NULLABLE |
| `status` | text | NOT NULL DEFAULT 'pending' |
| `agent_token` | text | NOT NULL |
| `created_at` | datetime | NOT NULL DEFAULT CURRENT_TIMESTAMP |
| `updated_at` | datetime | NOT NULL DEFAULT CURRENT_TIMESTAMP |
| `agent_url` | text | NULLABLE |
| `ipv6` | text | NULLABLE |
| `provider` | text | NULLABLE |
| `ssh_port` | integer | DEFAULT 22 |
| `description` | text | DEFAULT '' |
| `last_health_check` | datetime | NULLABLE |
| `health_status` | text | DEFAULT 'unknown' |
| `health_latency_ms` | integer | DEFAULT 0 |

**Status:** `pending`, `online`, `offline`, `error`, `disabled`

---

## Site Tables

### `sites`

Managed sites/applications.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | integer | PRIMARY KEY AUTOINCREMENT |
| `server_id` | integer | NOT NULL REFERENCES servers(id) ON DELETE CASCADE |
| `name` | text | NOT NULL |
| `domain` | text | NOT NULL |
| `runtime` | text | NOT NULL DEFAULT 'static' |
| `root_path` | text | NOT NULL |
| `status` | text | NOT NULL DEFAULT 'pending' |
| `created_at` | datetime | NOT NULL DEFAULT CURRENT_TIMESTAMP |
| `updated_at` | datetime | NOT NULL DEFAULT CURRENT_TIMESTAMP |
| `owner_id` | integer | REFERENCES users(id) |
| `ssl_status` | text | NULLABLE |
| `ssl_issuer` | text | NULLABLE |
| `ssl_expires_at` | datetime | NULLABLE |
| `ssl_last_check` | datetime | NULLABLE |
| `git_repo` | text | NULLABLE |
| `git_branch` | text | DEFAULT 'main' |
| `git_commit` | text | NULLABLE |
| `deploy_key` | text | NULLABLE |
| `webhook_secret` | text | NULLABLE |
| `managed_by` | text | NOT NULL DEFAULT 'sigardapanel' |
| `system_user` | text | NULLABLE |
| `conf_file` | text | NULLABLE |
| `proxy_pass_url` | text | NULLABLE |
| `app_port` | integer | NULLABLE |
| `app_command` | text | DEFAULT '' |
| `app_status` | text | DEFAULT 'stopped' |
| `app_memory_max` | integer | DEFAULT 512 |
| `php_version` | text | DEFAULT '' |
| `php_ini` | text | DEFAULT '' |
| `backup_provider_id` | integer | NULLABLE |

**Runtimes:** `static`, `php`, `node`, `go`, `python`, `proxy`, `docker`

**Indexes:** `server_id`, `domain`, `owner_id`, `managed_by`, `system_user`

### `user_sites`

User-to-site access mapping.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | integer | PRIMARY KEY AUTOINCREMENT |
| `user_id` | integer | NOT NULL REFERENCES users(id) ON DELETE CASCADE |
| `site_id` | integer | NOT NULL REFERENCES sites(id) ON DELETE CASCADE |
| `permission` | text | NOT NULL DEFAULT 'read' CHECK ('read','write','admin') |
| `created_at` | timestamp | DEFAULT CURRENT_TIMESTAMP |

**Constraint:** UNIQUE(`user_id`, `site_id`)

### `site_vhost`

Custom Nginx vhost configuration per site.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | integer | PRIMARY KEY AUTOINCREMENT |
| `site_id` | integer | UNIQUE NOT NULL REFERENCES sites(id) ON DELETE CASCADE |
| `config` | text | NOT NULL DEFAULT '' |
| `updated_at` | datetime | NOT NULL DEFAULT CURRENT_TIMESTAMP |

### `site_varnish`

Varnish cache configuration per site.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | integer | PRIMARY KEY AUTOINCREMENT |
| `site_id` | integer | UNIQUE NOT NULL REFERENCES sites(id) ON DELETE CASCADE |
| `enabled` | integer | NOT NULL DEFAULT 0 |
| `cache_time` | integer | NOT NULL DEFAULT 3600 |
| `hit_rate` | text | NOT NULL DEFAULT '0%' |
| `backend_host` | text | NOT NULL DEFAULT '127.0.0.1' |
| `backend_port` | integer | NOT NULL DEFAULT 80 |
| `updated_at` | datetime | NOT NULL DEFAULT CURRENT_TIMESTAMP |

### `site_security`

Security configuration per site.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | integer | PRIMARY KEY AUTOINCREMENT |
| `site_id` | integer | UNIQUE NOT NULL REFERENCES sites(id) ON DELETE CASCADE |
| `basic_auth_enabled` | integer | NOT NULL DEFAULT 0 |
| `basic_auth_username` | text | NOT NULL DEFAULT '' |
| `basic_auth_password` | text | NOT NULL DEFAULT '' |
| `blocked_ips` | text | NOT NULL DEFAULT '[]' |
| `updated_at` | datetime | NOT NULL DEFAULT CURRENT_TIMESTAMP |

### `site_ssh_ftp`

SSH/FTP user accounts per site.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | integer | PRIMARY KEY AUTOINCREMENT |
| `site_id` | integer | NOT NULL REFERENCES sites(id) ON DELETE CASCADE |
| `username` | text | NOT NULL |
| `password` | text | NOT NULL DEFAULT '' |
| `ssh_keys` | text | NOT NULL DEFAULT '' |
| `home_dir` | text | NOT NULL DEFAULT '' |
| `ssh_private_key` | text | NOT NULL DEFAULT '' |
| `ssh_public_key` | text | NOT NULL DEFAULT '' |
| `created_at` | datetime | NOT NULL DEFAULT CURRENT_TIMESTAMP |

**Constraint:** UNIQUE(`site_id`, `username`)

### `site_cron_jobs`

Cron jobs per site.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | integer | PRIMARY KEY AUTOINCREMENT |
| `site_id` | integer | NOT NULL REFERENCES sites(id) ON DELETE CASCADE |
| `schedule` | text | NOT NULL DEFAULT '* * * * *' |
| `command` | text | NOT NULL |
| `created_at` | datetime | NOT NULL DEFAULT CURRENT_TIMESTAMP |

---

## Job Tables

### `jobs`

Async job queue.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | integer | PRIMARY KEY AUTOINCREMENT |
| `server_id` | integer | REFERENCES servers(id) ON DELETE CASCADE |
| `site_id` | integer | REFERENCES sites(id) ON DELETE CASCADE |
| `type` | text | NOT NULL |
| `status` | text | NOT NULL DEFAULT 'queued' |
| `payload` | text | NULLABLE |
| `result` | text | NULLABLE |
| `created_at` | datetime | NOT NULL DEFAULT CURRENT_TIMESTAMP |
| `updated_at` | datetime | NOT NULL DEFAULT CURRENT_TIMESTAMP |
| `error` | text | NULLABLE |
| `attempts` | integer | NOT NULL DEFAULT 0 |
| `max_attempts` | integer | NOT NULL DEFAULT 3 |
| `started_at` | datetime | NULLABLE |
| `completed_at` | datetime | NULLABLE |
| `next_retry_at` | datetime | NULLABLE |

**Status:** `queued`, `running`, `succeeded`, `failed`, `canceled`

**Indexes:** `status`, `server_id`, (`status`, `type`), partial index on `next_retry_at` WHERE status='queued'

### `job_logs`

Per-line job output.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | integer | PRIMARY KEY AUTOINCREMENT |
| `job_id` | integer | NOT NULL REFERENCES jobs(id) ON DELETE CASCADE |
| `line_number` | integer | NOT NULL |
| `message` | text | NOT NULL |
| `stream` | text | NOT NULL DEFAULT 'stdout' |
| `created_at` | datetime | NOT NULL DEFAULT CURRENT_TIMESTAMP |

**Indexes:** `job_id`, (`job_id`, `line_number`)

---

## Backup Tables

### `backups`

Backup metadata.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | integer | PRIMARY KEY AUTOINCREMENT |
| `site_id` | integer | NOT NULL REFERENCES sites(id) ON DELETE CASCADE |
| `status` | text | NOT NULL DEFAULT 'pending' |
| `storage_type` | text | NOT NULL DEFAULT 'local' |
| `storage_path` | text | NULLABLE |
| `size_bytes` | integer | NOT NULL DEFAULT 0 |
| `file_count` | integer | NOT NULL DEFAULT 0 |
| `error` | text | NULLABLE |
| `created_at` | datetime | NOT NULL DEFAULT CURRENT_TIMESTAMP |
| `completed_at` | datetime | NULLABLE |
| `expires_at` | datetime | NULLABLE |
| `s3_path` | text | NULLABLE |
| `progress` | text | DEFAULT '' |

**Status:** `pending`, `running`, `archiving`, `succeeded`, `failed`, `deleted`

**Indexes:** `site_id`, `status`, `created_at`

### `backup_storage_providers`

External backup storage (S3/B2/R2/Wasabi).

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | integer | PRIMARY KEY AUTOINCREMENT |
| `name` | text | NOT NULL |
| `provider_type` | text | NOT NULL DEFAULT 's3' CHECK('s3','b2','r2','wasabi','digitalocean','generic') |
| `bucket` | text | NOT NULL |
| `endpoint` | text | NOT NULL |
| `region` | text | NOT NULL DEFAULT 'auto' |
| `access_key` | text | NOT NULL |
| `secret_key` | text | NOT NULL |
| `is_default` | integer | NOT NULL DEFAULT 0 |
| `retention_days` | integer | NOT NULL DEFAULT 0 |
| `created_at` | datetime | NOT NULL DEFAULT CURRENT_TIMESTAMP |
| `updated_at` | datetime | NOT NULL DEFAULT CURRENT_TIMESTAMP |

**Indexes:** `is_default`

### `backup_schedules`

Per-site backup schedules.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | integer | PRIMARY KEY AUTOINCREMENT |
| `site_id` | integer | NOT NULL REFERENCES sites(id) ON DELETE CASCADE |
| `provider_id` | integer | NULLABLE |
| `cron_expr` | text | NOT NULL DEFAULT '' |
| `backup_type` | text | NOT NULL DEFAULT 'files' |
| `enabled` | integer | NOT NULL DEFAULT 1 |
| `last_run_at` | datetime | NULLABLE |
| `next_run_at` | datetime | NULLABLE |
| `created_at` | datetime | DEFAULT CURRENT_TIMESTAMP |
| `updated_at` | datetime | DEFAULT CURRENT_TIMESTAMP |

**Indexes:** `site_id`, (`enabled`, `next_run_at`)

### `backup_configs`

Global backup configurations (multi-site).

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | integer | PRIMARY KEY AUTOINCREMENT |
| `name` | text | NOT NULL DEFAULT '' |
| `scope_type` | text | NOT NULL DEFAULT 'files' |
| `site_ids` | text | NOT NULL DEFAULT '[]' |
| `provider_id` | integer | NULLABLE |
| `cron_expr` | text | NOT NULL DEFAULT '' |
| `enabled` | integer | NOT NULL DEFAULT 1 |
| `exclude_patterns` | text | NOT NULL DEFAULT '[]' |
| `database_ids` | text | NOT NULL DEFAULT '[]' |
| `retention_days` | integer | NOT NULL DEFAULT 30 |
| `last_run_at` | datetime | NULLABLE |
| `next_run_at` | datetime | NULLABLE |
| `created_at` | datetime | DEFAULT CURRENT_TIMESTAMP |
| `updated_at` | datetime | DEFAULT CURRENT_TIMESTAMP |

**Indexes:** (`enabled`, `next_run_at`)

---

## Database Management Tables

### `database_servers`

External MySQL/MariaDB server connections.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | integer | PRIMARY KEY AUTOINCREMENT |
| `name` | text | NOT NULL DEFAULT 'Local MySQL' |
| `engine` | text | NOT NULL DEFAULT 'MariaDB' |
| `host` | text | NOT NULL DEFAULT '127.0.0.1' |
| `port` | integer | NOT NULL DEFAULT 3306 |
| `username` | text | NOT NULL DEFAULT 'root' |
| `password_encrypted` | text | NOT NULL |
| `is_active` | integer | NOT NULL DEFAULT 1 |
| `is_default` | integer | NOT NULL DEFAULT 1 |
| `created_at` | datetime | NOT NULL DEFAULT CURRENT_TIMESTAMP |
| `updated_at` | datetime | NOT NULL DEFAULT CURRENT_TIMESTAMP |

### `databases`

Managed databases.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | integer | PRIMARY KEY AUTOINCREMENT |
| `site_id` | integer | NOT NULL REFERENCES sites(id) ON DELETE CASCADE |
| `name` | text | NOT NULL |
| `type` | text | NOT NULL |
| `charset` | text | NOT NULL DEFAULT 'utf8mb4' |
| `collation` | text | NOT NULL DEFAULT 'utf8mb4_unicode_ci' |
| `size_bytes` | integer | NOT NULL DEFAULT 0 |
| `created_at` | datetime | NOT NULL DEFAULT CURRENT_TIMESTAMP |

**Indexes:** `site_id`, `name`

### `db_users`

Database user accounts.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | integer | PRIMARY KEY AUTOINCREMENT |
| `database_id` | integer | NOT NULL REFERENCES databases(id) ON DELETE CASCADE |
| `username` | text | NOT NULL |
| `password_hash` | text | NOT NULL |
| `host_pattern` | text | NOT NULL DEFAULT 'localhost' |
| `privileges` | text | NOT NULL DEFAULT 'ALL' |
| `encrypted_password` | text | NOT NULL DEFAULT '' |
| `created_at` | datetime | NOT NULL DEFAULT CURRENT_TIMESTAMP |

**Indexes:** `database_id`, `username`

---

## Metrics Tables

### `server_metrics`

Per-server resource metrics.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | integer | PRIMARY KEY AUTOINCREMENT |
| `server_id` | integer | NOT NULL REFERENCES servers(id) ON DELETE CASCADE |
| `collected_at` | datetime | NOT NULL DEFAULT CURRENT_TIMESTAMP |
| `cpu_percent` | real | NULLABLE |
| `mem_used` | integer | NULLABLE |
| `mem_total` | integer | NULLABLE |
| `disk_used` | integer | NULLABLE |
| `disk_total` | integer | NULLABLE |
| `net_in_bytes` | integer | NULLABLE |
| `net_out_bytes` | integer | NULLABLE |
| `swap_used` | integer | NULLABLE |
| `swap_total` | integer | NULLABLE |

**Indexes:** `server_id`, `collected_at`, (`server_id`, `collected_at`)

---

## Notification Tables

### `alert_channels`

Alert output channels (Telegram, Discord, email, webhook).

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | integer | PRIMARY KEY AUTOINCREMENT |
| `name` | text | NOT NULL |
| `type` | text | NOT NULL |
| `enabled` | integer | NOT NULL DEFAULT 1 |
| `config` | text | NOT NULL |
| `created_at` | datetime | NOT NULL DEFAULT CURRENT_TIMESTAMP |
| `updated_at` | datetime | NOT NULL DEFAULT CURRENT_TIMESTAMP |

**Indexes:** `type`

### `alerts`

Alert events sent through channels.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | integer | PRIMARY KEY AUTOINCREMENT |
| `channel_id` | integer | NOT NULL REFERENCES alert_channels(id) ON DELETE CASCADE |
| `severity` | text | NOT NULL |
| `subject` | text | NOT NULL |
| `message` | text | NOT NULL |
| `status` | text | NOT NULL DEFAULT 'pending' |
| `error` | text | NULLABLE |
| `created_at` | datetime | NOT NULL DEFAULT CURRENT_TIMESTAMP |
| `sent_at` | datetime | NULLABLE |

**Indexes:** `channel_id`, `status`, `created_at`

### `notifications`

In-app notifications (per-user).

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | integer | PRIMARY KEY AUTOINCREMENT |
| `user_id` | integer | NOT NULL |
| `type` | text | NOT NULL DEFAULT 'info' |
| `title` | text | NOT NULL |
| `message` | text | NOT NULL DEFAULT '' |
| `severity` | text | NOT NULL DEFAULT 'info' |
| `read` | integer | NOT NULL DEFAULT 0 |
| `entity_type` | text | NOT NULL DEFAULT '' |
| `entity_id` | integer | NOT NULL DEFAULT 0 |
| `created_at` | datetime | NOT NULL DEFAULT CURRENT_TIMESTAMP |
| `read_at` | datetime | NULLABLE |

**Indexes:** `user_id`, (`user_id`, `read`), `created_at DESC`

---

## Audit Tables

### `audit_logs`

Audit events.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | integer | PRIMARY KEY AUTOINCREMENT |
| `user_id` | integer | REFERENCES users(id) |
| `action` | text | NOT NULL |
| `resource` | text | NOT NULL |
| `resource_id` | integer | NULLABLE |
| `details` | text | NULLABLE |
| `created_at` | datetime | NOT NULL DEFAULT CURRENT_TIMESTAMP |

**Indexes:** `user_id`, `created_at`

---

## Integration Tables

### `cloudflare_config`

Cloudflare API configuration.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | integer | PRIMARY KEY CHECK (id = 1) |
| `api_token` | text | NOT NULL DEFAULT '' |
| `email` | text | DEFAULT '' |
| `user_id` | integer | DEFAULT NULL |
| `created_at` | timestamp | DEFAULT CURRENT_TIMESTAMP |
| `updated_at` | timestamp | DEFAULT CURRENT_TIMESTAMP |

**Constraint:** UNIQUE(`user_id`) WHERE user_id IS NOT NULL

---

## License & Billing Tables

### `licenses`

System license keys.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | integer | PRIMARY KEY AUTOINCREMENT |
| `key` | text | UNIQUE NOT NULL |
| `tier` | text | NOT NULL DEFAULT 'free' |
| `activated_at` | datetime | NULLABLE |
| `expires_at` | datetime | NULLABLE |
| `max_servers` | integer | DEFAULT 1 |
| `features` | text | DEFAULT '{}' |
| `created_at` | datetime | DEFAULT CURRENT_TIMESTAMP |

### `feature_flags`

Feature gating per tier.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | integer | PRIMARY KEY AUTOINCREMENT |
| `key` | text | UNIQUE NOT NULL |
| `name` | text | NOT NULL |
| `tier` | text | NOT NULL DEFAULT 'pro' |
| `enabled` | integer | DEFAULT 1 |
| `created_at` | datetime | DEFAULT CURRENT_TIMESTAMP |

### `plans`

Billing plans.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | integer | PRIMARY KEY AUTOINCREMENT |
| `name` | text | UNIQUE NOT NULL |
| `slug` | text | UNIQUE NOT NULL |
| `price_idr` | integer | DEFAULT 0 |
| `billing_cycle` | text | DEFAULT 'monthly' |
| `features` | text | DEFAULT '{}' |
| `is_active` | boolean | DEFAULT 1 |
| `created_at` | datetime | DEFAULT CURRENT_TIMESTAMP |
| `updated_at` | datetime | DEFAULT CURRENT_TIMESTAMP |

### `license_orders`

License purchase orders.

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | integer | PRIMARY KEY AUTOINCREMENT |
| `user_id` | integer | REFERENCES users(id) |
| `plan_id` | integer | REFERENCES plans(id) |
| `status` | text | DEFAULT 'pending' |
| `payment_method` | text | NULLABLE |
| `payment_proof` | text | NULLABLE |
| `amount_idr` | integer | DEFAULT 0 |
| `notes` | text | NULLABLE |
| `admin_notes` | text | NULLABLE |
| `approved_by` | integer | REFERENCES users(id) |
| `created_at` | datetime | DEFAULT CURRENT_TIMESTAMP |
| `updated_at` | datetime | DEFAULT CURRENT_TIMESTAMP |

---

## Settings Tables

### `app_settings`

Global application settings (key-value).

| Column | Type | Constraints |
|--------|------|-------------|
| `key` | text | PRIMARY KEY |
| `value` | text | NOT NULL DEFAULT '' |
| `updated_at` | datetime | NOT NULL DEFAULT CURRENT_TIMESTAMP |
