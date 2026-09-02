# Security Model SigardaPanel

Dokumen ini menjadi baseline keamanan untuk panel VPS, agent, CLI, dan dashboard.

## Prinsip Utama
- Backend adalah policy authority.
- Agent hanya executor, bukan pembuat keputusan permission.
- Semua input dianggap tidak dipercaya.
- Semua operasi server harus diaudit.
- Secret tidak boleh muncul di log, UI, CLI output, atau audit payload.
- Operasi destruktif harus punya guard dan confirmation.

## Threat Model

### Aset Dilindungi
- Akun admin/user.
- API token/session token.
- Server/VPS root access.
- Site files dan database.
- Env vars dan deploy secrets.
- SSL private keys.
- Backup archives.
- Audit logs.

### Aktor Ancaman
- User biasa yang mencoba naik privilege.
- Operator yang punya akses terbatas tapi mencoba akses site lain.
- Penyerang eksternal pada dashboard/API.
- Penyerang yang menguasai salah satu site app.
- Penyerang yang mencuri token CLI/API.
- Agent/server yang sudah compromise.

### Risiko Utama
- Command injection.
- Path traversal.
- Privilege escalation.
- Secret leakage.
- CSRF/XSS di dashboard.
- Broken access control.
- Reverse proxy config injection.
- Unsafe backup/restore overwrite.
- SSRF dari fitur fetch/deploy webhook.

## Auth
- Password wajib hash kuat: Argon2id atau bcrypt.
- Session token harus random kuat dan expire.
- API token harus disimpan hashed, bukan plaintext.
- Token hanya ditampilkan sekali saat dibuat.
- 2FA/TOTP disiapkan untuk Beta/Stable.
- Rate limit endpoint login, token create, password reset.

## RBAC
Role awal:
- owner: semua akses.
- admin: kelola server/site/user kecuali owner action.
- operator: deploy/restart/log untuk site yang diberi akses.
- viewer: read-only.

Permission harus granular:
- `server.read`
- `server.create`
- `server.delete`
- `site.read`
- `site.create`
- `site.update`
- `site.delete`
- `site.deploy`
- `site.logs`
- `ssl.manage`
- `backup.create`
- `backup.restore`
- `user.manage`
- `audit.read`

Semua permission dicek di backend sebelum job dibuat. Agent tidak boleh menerima task tanpa signature/token valid dari backend.

## Agent Security
- Agent harus authenticate panel request.
- Gunakan mTLS atau token kuat dengan rotation plan.
- Agent expose endpoint hanya ke network yang dibutuhkan.
- Agent harus validate task schema.
- Agent harus enforce allowed path.
- Agent harus enforce allowed operations.
- Agent tidak boleh menjalankan shell string langsung dari user input.
- Agent harus punya timeout per operation.
- Agent harus log operation ID/job ID.

## Command Execution
- Gunakan exec dengan arg array, bukan `sh -c`, kecuali benar-benar perlu.
- Jika shell dibutuhkan, command harus template internal dengan variable tervalidasi.
- Validasi domain dengan parser/regex ketat.
- Validasi username/database/service name dengan allowlist character.
- Validasi path pakai clean path dan prefix check ke allowed root.
- Build command custom harus disabled default atau butuh approval/allowlist.
- Jangan interpolasi user input ke Nginx/Caddy config tanpa escaping/template aman.

## Path Safety
- Semua operasi file site harus di bawah site root.
- Tolak symlink escape jika file manager mendukung symlink.
- Tolak `..`, null byte, path absolut dari input user.
- Gunakan canonical path sebelum operasi destruktif.
- Delete harus target resource terdaftar, bukan path bebas.

## Secret Handling
- Secret disimpan encrypted at rest jika memungkinkan.
- Minimal, jangan simpan secret plaintext di logs/audit/jobs.
- Mask secret di UI dan CLI: tampilkan prefix/suffix pendek saja.
- Env vars site jangan ikut tampil full setelah tersimpan.
- Deploy token/Git token harus scoped dan revocable.
- Backup secret/config harus dienkripsi atau disimpan terpisah.

## Audit Log
Audit wajib untuk:
- Login gagal/berhasil penting.
- Create/update/delete user.
- Create/delete server.
- Agent registration.
- Create/update/delete site.
- Deploy.
- SSL issue/renew/fail.
- Backup/restore.
- Service restart/reload.
- API token create/revoke.

Audit payload harus aman:
- Jangan simpan password/token/private key.
- Jangan simpan full env values.
- Simpan request_id/job_id untuk korelasi.

## Dashboard Security
- Enforce CSRF protection jika pakai cookie session.
- Set cookie HttpOnly, Secure, SameSite.
- Escape semua output user-generated.
- Jangan percaya role/permission dari client.
- Terapkan security headers.
- Hindari menyimpan token jangka panjang di localStorage jika bisa.

## Backup & Restore Security
- Restore harus butuh permission khusus.
- Restore overwrite harus butuh konfirmasi eksplisit.
- Backup archive harus divalidasi sebelum extract.
- Extract harus cegah zip-slip/tar path traversal.
- Remote backup credential harus secret-masked.

## Network Security
- Panel HTTPS wajib untuk production.
- Agent endpoint sebaiknya tidak publik luas.
- Support IP allowlist untuk admin/agent pada fase lanjut.
- Webhook endpoint harus punya secret/signature validation.
- Hindari SSRF dari URL input dengan allowlist/block private network bila relevan.

## Secure Defaults
- Disable destructive action tanpa confirmation.
- Disable custom build command default jika belum ada sandbox/approval.
- Default role user baru adalah viewer/operator, bukan admin.
- Default site isolation aktif.
- Default secret masking aktif.
- Default audit log aktif.
