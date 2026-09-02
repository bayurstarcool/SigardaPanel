# MVP SigardaPanel

Dokumen ini mendefinisikan scope versi awal agar pengembangan tetap fokus, aman, dan bisa dirilis bertahap.

## Tujuan MVP
- Panel VPS self-hosted berbasis Go.
- Bisa mengelola beberapa server melalui agent.
- Bisa membuat dan mengelola site dasar.
- Bisa deploy app static, Node.js, dan Go.
- Bisa membuat SSL otomatis.
- Bisa dipakai via dashboard dan CLI.
- Semua operasi server tercatat dalam audit log.

## Non-Goal MVP
- Tidak membangun fitur email/mailbox.
- Tidak membangun billing.
- Tidak membangun plugin system.
- Tidak mendukung semua distro Linux.
- Tidak membuat full cPanel clone di rilis pertama.
- Tidak membuat DNS authoritative server.
- Tidak membuat Docker manager penuh di MVP.

## Target OS
- Ubuntu 22.04 LTS.
- Ubuntu 24.04 LTS.
- Debian 12 sebagai target sekunder.

## Komponen MVP
- Backend API Go.
- Agent Go di server target.
- CLI Go.
- Dashboard React/Next.js.
- PostgreSQL untuk database utama.
- Redis atau storage queue sederhana untuk background jobs.
- Nginx atau Caddy sebagai reverse proxy, pilih satu saat implementasi awal.

## Fitur MVP

### Auth & User
- Login/logout.
- Password hashing aman.
- Role dasar: owner, admin, operator, viewer.
- Session management.
- API token scoped permission.

### Server Management
- Register server.
- Install agent.
- Check agent health.
- List capability server.
- View CPU, RAM, disk basic.
- View service status basic.

### Site Management
- Create site dengan domain.
- List/detail site.
- Delete site dengan confirmation guard.
- Enable/disable site.
- Configure reverse proxy.
- View access/error logs.

### Runtime MVP
- Static site.
- Node.js app via systemd service.
- Go binary via systemd service.

### SSL
- Issue SSL Let's Encrypt.
- Renew SSL otomatis.
- Check SSL expiry.
- Force HTTPS toggle.

### Deploy
- Upload/deploy artifact sederhana.
- Git deploy basic: repo URL, branch, deploy key/token via secret.
- Build command optional dengan allowlist/approval.
- Restart service setelah deploy.

### Job Queue
- Async operation untuk create site, deploy, SSL, delete site.
- Job status: queued, running, succeeded, failed, canceled.
- Job logs.
- Retry terbatas.

### Audit Log
- Catat actor, action, target, server, status, timestamp.
- Mask secret di payload.
- Search/filter basic.

### CLI MVP
- `sigardapanel init`
- `sigardapanel login`
- `sigardapanel server add|list|doctor`
- `sigardapanel agent install|status|logs`
- `sigardapanel site create|list|delete|deploy`
- `sigardapanel logs tail`
- `sigardapanel job list|watch`

## Data Model Awal
- users
- roles
- sessions
- api_tokens
- servers
- agents
- sites
- site_domains
- deployments
- jobs
- audit_logs
- secrets

## Definition of Done MVP
- Install panel di satu VPS.
- Add server yang sama atau server lain via agent.
- Create static site dengan domain.
- Issue SSL valid.
- Deploy Node.js app.
- Deploy Go app.
- Lihat logs site/service.
- Operasi async terlihat di job logs.
- Audit log tercatat untuk operasi penting.
- CLI bisa menjalankan flow dasar tanpa dashboard.

## Risiko Utama
- Command injection dari domain/path/build command.
- Agent compromise karena privilege root.
- Data loss saat delete/restore.
- Reverse proxy config rusak dan downtime.
- Secret leakage lewat logs/UI/CLI.

## Strategi Mitigasi MVP
- Validasi ketat domain, path, service name.
- Wrapper command dengan arg array, bukan shell string.
- Config test sebelum reload proxy.
- Dry-run/confirmation untuk delete.
- Secret masking di semua output.
- Audit log wajib.
