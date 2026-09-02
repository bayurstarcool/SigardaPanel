# Database SigardaPanel

Dokumen ini mendefinisikan rancangan schema awal SigardaPanel.

## Database Target
- PostgreSQL sebagai database utama.
- UUID/ULID string sebagai ID publik direkomendasikan.
- Timestamp wajib pakai timezone.
- Soft delete untuk resource penting bila relevan.
- Migration wajib versioned.

## Konvensi
- Primary key: `id`.
- Public ID prefix opsional: `usr_`, `srv_`, `site_`, `job_`.
- Timestamp umum:
  - `created_at`
  - `updated_at`
  - `deleted_at` nullable untuk soft delete.
- Status gunakan enum string atau constrained text.
- Secret value tidak disimpan plaintext.

## Core Tables

## `users`
Menyimpan akun user.

Kolom:
- `id` text primary key.
- `email` text unique not null.
- `name` text not null.
- `password_hash` text not null.
- `status` text not null: active, disabled.
- `last_login_at` timestamptz nullable.
- `created_at` timestamptz not null.
- `updated_at` timestamptz not null.
- `deleted_at` timestamptz nullable.

Index:
- unique lower(email).
- status.

## `roles`
Role bawaan dan custom.

Kolom:
- `id` text primary key.
- `name` text unique not null.
- `description` text nullable.
- `is_system` boolean not null default false.
- `created_at` timestamptz not null.
- `updated_at` timestamptz not null.

Role awal:
- owner.
- admin.
- operator.
- viewer.

## `permissions`
Daftar permission granular.

Kolom:
- `id` text primary key.
- `key` text unique not null.
- `description` text nullable.
- `created_at` timestamptz not null.

## `role_permissions`
Relasi role-permission.

Kolom:
- `role_id` text references roles(id).
- `permission_id` text references permissions(id).
- primary key (`role_id`, `permission_id`).

## `user_roles`
Relasi user-role global.

Kolom:
- `user_id` text references users(id).
- `role_id` text references roles(id).
- `created_at` timestamptz not null.
- primary key (`user_id`, `role_id`).

## `sessions`
Session dashboard.

Kolom:
- `id` text primary key.
- `user_id` text references users(id).
- `token_hash` text unique not null.
- `user_agent` text nullable.
- `ip_address` inet nullable.
- `expires_at` timestamptz not null.
- `revoked_at` timestamptz nullable.
- `created_at` timestamptz not null.

Index:
- user_id.
- token_hash.
- expires_at.

## `api_tokens`
Token CLI/API.

Kolom:
- `id` text primary key.
- `user_id` text references users(id).
- `name` text not null.
- `token_hash` text unique not null.
- `scopes` jsonb not null default `[]`.
- `last_used_at` timestamptz nullable.
- `expires_at` timestamptz nullable.
- `revoked_at` timestamptz nullable.
- `created_at` timestamptz not null.

Index:
- user_id.
- token_hash.
- revoked_at.

## Server & Agent Tables

## `servers`
Server/VPS yang dikelola.

Kolom:
- `id` text primary key.
- `name` text unique not null.
- `host` text nullable.
- `provider` text nullable.
- `status` text not null: pending, online, offline, error, disabled.
- `os_name` text nullable.
- `os_version` text nullable.
- `arch` text nullable.
- `last_seen_at` timestamptz nullable.
- `created_by` text references users(id).
- `created_at` timestamptz not null.
- `updated_at` timestamptz not null.
- `deleted_at` timestamptz nullable.

Index:
- status.
- last_seen_at.

## `agents`
Agent identity per server.

Kolom:
- `id` text primary key.
- `server_id` text references servers(id).
- `version` text nullable.
- `fingerprint` text unique not null.
- `auth_hash` text nullable.
- `status` text not null: pending, active, revoked.
- `registered_at` timestamptz nullable.
- `last_heartbeat_at` timestamptz nullable.
- `created_at` timestamptz not null.
- `updated_at` timestamptz not null.

Index:
- server_id.
- fingerprint.
- status.

## `agent_bootstrap_tokens`
Token sekali pakai untuk register agent.

Kolom:
- `id` text primary key.
- `server_id` text references servers(id).
- `token_hash` text unique not null.
- `expires_at` timestamptz not null.
- `used_at` timestamptz nullable.
- `created_by` text references users(id).
- `created_at` timestamptz not null.

## `server_capabilities`
Capability report terakhir.

Kolom:
- `server_id` text primary key references servers(id).
- `capabilities` jsonb not null.
- `reported_at` timestamptz not null.

## Site Tables

## `sites`
Site/app yang dikelola.

Kolom:
- `id` text primary key.
- `server_id` text references servers(id).
- `primary_domain` text not null.
- `runtime` text not null: static, node, go, php, python, docker.
- `status` text not null: provisioning, active, disabled, error, deleting.
- `site_user` text nullable.
- `root_path` text nullable.
- `current_release_path` text nullable.
- `ssl_enabled` boolean not null default false.
- `created_by` text references users(id).
- `created_at` timestamptz not null.
- `updated_at` timestamptz not null.
- `deleted_at` timestamptz nullable.

Index:
- server_id.
- primary_domain.
- status.
- runtime.

Constraint:
- unique active domain via partial unique index on lower(primary_domain) where deleted_at is null.

## `site_domains`
Additional domains/aliases.

Kolom:
- `id` text primary key.
- `site_id` text references sites(id).
- `domain` text not null.
- `type` text not null: primary, alias, redirect.
- `created_at` timestamptz not null.

Index:
- site_id.
- lower(domain) unique where active.

## `site_env_vars`
Metadata env var per site.

Kolom:
- `id` text primary key.
- `site_id` text references sites(id).
- `key` text not null.
- `value_ref` text not null.
- `is_secret` boolean not null default true.
- `created_at` timestamptz not null.
- `updated_at` timestamptz not null.

Constraint:
- unique (`site_id`, `key`).

## `site_permissions`
Akses user tertentu ke site.

Kolom:
- `site_id` text references sites(id).
- `user_id` text references users(id).
- `role_id` text references roles(id).
- `created_at` timestamptz not null.
- primary key (`site_id`, `user_id`, `role_id`).

## Deployment Tables

## `deployments`
Riwayat deploy.

Kolom:
- `id` text primary key.
- `site_id` text references sites(id).
- `job_id` text nullable references jobs(id).
- `source_type` text not null: git, artifact, manual.
- `source_ref` text nullable.
- `commit_sha` text nullable.
- `release_path` text nullable.
- `status` text not null: pending, running, succeeded, failed, rolled_back.
- `started_at` timestamptz nullable.
- `finished_at` timestamptz nullable.
- `created_by` text references users(id).
- `created_at` timestamptz not null.

Index:
- site_id.
- status.
- created_at.

## `deployment_artifacts`
Metadata artifact deploy.

Kolom:
- `id` text primary key.
- `deployment_id` text references deployments(id).
- `storage_path` text not null.
- `checksum` text nullable.
- `size_bytes` bigint nullable.
- `created_at` timestamptz not null.

## Job Tables

## `jobs`
Async job.

Kolom:
- `id` text primary key.
- `type` text not null.
- `status` text not null: queued, running, succeeded, failed, canceled.
- `priority` int not null default 0.
- `target_type` text nullable.
- `target_id` text nullable.
- `server_id` text nullable references servers(id).
- `payload` jsonb not null default `{}`.
- `result` jsonb nullable.
- `error_code` text nullable.
- `error_message` text nullable.
- `attempts` int not null default 0.
- `max_attempts` int not null default 1.
- `available_at` timestamptz not null.
- `started_at` timestamptz nullable.
- `finished_at` timestamptz nullable.
- `created_by` text nullable references users(id).
- `created_at` timestamptz not null.
- `updated_at` timestamptz not null.

Index:
- status, priority, available_at.
- server_id.
- target_type, target_id.
- created_at.

Payload tidak boleh berisi secret plaintext.

## `job_logs`
Log per job.

Kolom:
- `id` bigserial primary key.
- `job_id` text references jobs(id).
- `level` text not null: debug, info, warn, error.
- `message` text not null.
- `metadata` jsonb nullable.
- `created_at` timestamptz not null.

Index:
- job_id, created_at.

## SSL Tables

## `ssl_certificates`
Metadata SSL.

Kolom:
- `id` text primary key.
- `site_id` text references sites(id).
- `domain` text not null.
- `issuer` text nullable.
- `status` text not null: pending, active, expired, failed, revoked.
- `not_before` timestamptz nullable.
- `not_after` timestamptz nullable.
- `last_renewed_at` timestamptz nullable.
- `created_at` timestamptz not null.
- `updated_at` timestamptz not null.

Index:
- site_id.
- domain.
- not_after.
- status.

## Backup Tables

## `backups`
Backup metadata.

Kolom:
- `id` text primary key.
- `site_id` text nullable references sites(id).
- `server_id` text nullable references servers(id).
- `job_id` text nullable references jobs(id).
- `type` text not null: site, database, full.
- `status` text not null: pending, running, succeeded, failed, deleted.
- `storage` text not null: local, s3, sftp.
- `storage_path` text nullable.
- `checksum` text nullable.
- `size_bytes` bigint nullable.
- `created_by` text references users(id).
- `created_at` timestamptz not null.
- `completed_at` timestamptz nullable.
- `expires_at` timestamptz nullable.

Index:
- site_id.
- server_id.
- status.
- created_at.
- expires_at.

## Secret Tables

## `secrets`
Secret metadata dan encrypted value.

Kolom:
- `id` text primary key.
- `scope_type` text not null: global, server, site, user.
- `scope_id` text nullable.
- `key` text not null.
- `encrypted_value` bytea not null.
- `value_hash` text nullable.
- `created_by` text nullable references users(id).
- `created_at` timestamptz not null.
- `updated_at` timestamptz not null.

Index:
- scope_type, scope_id.
- key.

Constraint:
- unique (`scope_type`, `scope_id`, `key`).

## Audit Tables

## `audit_logs`
Audit event.

Kolom:
- `id` bigserial primary key.
- `request_id` text nullable.
- `actor_id` text nullable references users(id).
- `actor_type` text not null: user, token, agent, system.
- `action` text not null.
- `target_type` text nullable.
- `target_id` text nullable.
- `server_id` text nullable references servers(id).
- `status` text not null: success, failure.
- `ip_address` inet nullable.
- `user_agent` text nullable.
- `metadata` jsonb nullable.
- `created_at` timestamptz not null.

Index:
- actor_id.
- action.
- target_type, target_id.
- server_id.
- status.
- created_at.

Metadata wajib masked dari secret.

## Metrics Tables

## `server_metrics`
Metrics ringan per server.

Kolom:
- `id` bigserial primary key.
- `server_id` text references servers(id).
- `cpu_percent` numeric nullable.
- `memory_used_bytes` bigint nullable.
- `memory_total_bytes` bigint nullable.
- `disk_used_bytes` bigint nullable.
- `disk_total_bytes` bigint nullable.
- `load_average` numeric nullable.
- `created_at` timestamptz not null.

Index:
- server_id, created_at.

Retention diperlukan agar tabel tidak membengkak.

## `site_metrics`
Metrics ringan per site jika tersedia.

Kolom:
- `id` bigserial primary key.
- `site_id` text references sites(id).
- `cpu_percent` numeric nullable.
- `memory_bytes` bigint nullable.
- `requests_count` bigint nullable.
- `error_count` bigint nullable.
- `created_at` timestamptz not null.

Index:
- site_id, created_at.

## Notification Tables

## `notification_channels`
Channel alert.

Kolom:
- `id` text primary key.
- `name` text not null.
- `type` text not null: telegram, discord, email, webhook.
- `config_secret_ref` text nullable.
- `enabled` boolean not null default true.
- `created_by` text references users(id).
- `created_at` timestamptz not null.
- `updated_at` timestamptz not null.

## `alerts`
Alert event.

Kolom:
- `id` text primary key.
- `type` text not null.
- `severity` text not null: info, warning, critical.
- `target_type` text nullable.
- `target_id` text nullable.
- `message` text not null.
- `status` text not null: open, acknowledged, resolved.
- `created_at` timestamptz not null.
- `resolved_at` timestamptz nullable.

Index:
- status.
- severity.
- target_type, target_id.
- created_at.

## Migration Rules
- Migration harus versioned dan reviewable.
- Jangan drop kolom/data tanpa migration plan.
- Tambahkan index untuk query list/filter utama.
- Default value harus aman.
- Migration besar harus dipikirkan untuk downtime.

## Data Retention
Rekomendasi awal:
- Audit logs: 180 hari default, configurable.
- Job logs: 30 hari default, configurable.
- Metrics raw: 7-30 hari, lalu aggregate jika perlu.
- Backups: sesuai retention policy per target.

## Seed Data
Seed awal:
- Roles: owner, admin, operator, viewer.
- Permissions granular dasar.
- Owner user dibuat saat `sigardapanel init` atau installer panel.

## Open Questions
- Pakai UUIDv7/ULID/custom prefixed ID.
- Queue berbasis PostgreSQL atau Redis untuk MVP.
- Secret encryption key disimpan via env atau KMS lokal.
- Caddy atau Nginx sebagai reverse proxy awal.
