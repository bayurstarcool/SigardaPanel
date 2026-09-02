# Agent SigardaPanel

Dokumen ini mendefinisikan rancangan awal agent VPS.

## Tujuan
Agent adalah service Go yang berjalan di server target untuk mengeksekusi operasi server secara aman, terukur, dan dapat diaudit.

## Prinsip
- Agent bukan policy authority.
- Backend memutuskan permission.
- Agent memvalidasi task schema, path, dan capability.
- Agent menjalankan operasi dengan timeout.
- Agent tidak menjalankan input mentah user sebagai shell command.

## Mode Komunikasi
Prioritas desain:
1. HTTPS/mTLS antara backend dan agent.
2. Token signed request dengan rotation.
3. SSH mode hanya untuk bootstrap atau fallback bila diminta.

## Registrasi Agent
Flow:
1. Backend membuat bootstrap token.
2. User menjalankan installer di VPS target.
3. Installer mengirim token ke agent setup.
4. Agent register ke backend.
5. Backend menyimpan agent fingerprint.
6. Agent mengirim capability report.
7. Backend menandai server online setelah heartbeat valid.

## Capability Report
Agent harus melaporkan:
- OS name/version.
- Architecture.
- Agent version.
- Hostname.
- CPU/RAM/disk summary.
- Reverse proxy tersedia: nginx/caddy.
- Runtime tersedia: node, go, php, python, docker.
- Systemd availability.
- ACME/SSL capability.
- Allowed roots.

Contoh response:

```json
{
  "agent_version": "0.1.0",
  "os": "ubuntu",
  "os_version": "24.04",
  "arch": "amd64",
  "capabilities": {
    "systemd": true,
    "nginx": true,
    "caddy": false,
    "node": true,
    "go": true,
    "php": false,
    "python": true,
    "docker": false
  }
}
```

## Endpoint Internal Agent
Nama endpoint final bisa berubah, tapi kontrak konsep:
- `GET /health`
- `GET /capabilities`
- `POST /tasks/site/create`
- `POST /tasks/site/delete`
- `POST /tasks/site/deploy`
- `POST /tasks/ssl/issue`
- `POST /tasks/service/reload`
- `GET /logs`
- `GET /metrics`

Semua endpoint mutasi harus authenticate request dari backend.

## Task Envelope
Semua task harus punya envelope:

```json
{
  "task_id": "task_123",
  "job_id": "job_123",
  "requested_by": "user_123",
  "action": "site.create",
  "payload": {},
  "timeout_seconds": 300,
  "issued_at": "2026-06-24T00:00:00Z"
}
```

Agent harus reject task tanpa `task_id`, `job_id`, `action`, atau auth valid.

## Operasi Site Create
Input minimal:
- site_id.
- domain.
- runtime.
- document_root/release_root generated.
- ssl enabled/disabled.

Langkah:
1. Validasi domain dan site_id.
2. Buat direktori site di allowed root.
3. Buat user/group per site jika enabled.
4. Generate runtime config.
5. Generate reverse proxy config.
6. Test proxy config.
7. Reload proxy.
8. Issue SSL jika diminta.
9. Return status dan logs aman.

## Operasi Deploy
Strategi awal:
- release directory per deploy.
- symlink `current` ke release aktif.
- rollback bisa mengganti symlink ke release sebelumnya.
- restart service setelah deploy jika diperlukan.

Deploy source:
- Git repo.
- Artifact upload.

Build command:
- Disabled default atau require allowlist/approval.
- Jika enabled, jalan sebagai site user, bukan root.
- Timeout wajib.

## Reverse Proxy
Jika Nginx:
- Generate config di path terkontrol.
- File harus punya marker SigardaPanel.
- Jalankan `nginx -t` sebelum reload.
- Reload hanya jika test sukses.

Jika Caddy:
- Generate Caddyfile/config JSON terkontrol.
- Validate config sebelum reload jika tersedia.

## Systemd Service
Untuk Node/Go/Python:
- Unit name: `sigardapanel-site-<site_id>.service`.
- Run user: site user jika isolation aktif.
- WorkingDirectory: site current release.
- EnvironmentFile: path env site.
- Restart policy: `on-failure`.

## Logs
Agent harus bisa membaca:
- Access log.
- Error log.
- App/service log.
- Agent operation log.

Log output harus support:
- tail lines.
- follow stream.
- since timestamp.
- max bytes limit.

## Metrics
Metrics MVP:
- CPU usage.
- RAM usage.
- Disk usage.
- Load average.
- Service status.
- Site process status.

Metrics harus ringan. Jangan polling terlalu sering.

## Safety Guard
Agent harus menolak:
- Path di luar allowed root.
- Domain invalid.
- Service name invalid.
- Task expired.
- Task action tidak dikenal.
- Request tanpa auth valid.
- Delete resource yang tidak punya marker SigardaPanel.

## File Marker
Generated file harus punya header komentar:

```text
Managed by SigardaPanel. Do not edit manually unless you know what you are doing.
site_id=<site_id>
```

Agent tidak boleh overwrite file tanpa marker kecuali task eksplisit punya mode import/adopt.

## Error Model
Agent response error:

```json
{
  "error": {
    "code": "proxy_config_invalid",
    "message": "Reverse proxy config validation failed",
    "details": "safe short detail",
    "task_id": "task_123"
  }
}
```

Error detail tidak boleh berisi secret.

## Installer Agent
Installer harus:
- Detect OS/arch.
- Install binary agent.
- Buat config agent.
- Register agent ke panel.
- Buat systemd service.
- Start dan enable service.
- Tampilkan status akhir.

Installer harus idempotent dan aman dijalankan ulang.
