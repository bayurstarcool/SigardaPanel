# Integrasi & Migrasi CloudPanel → SigardaPanel

Dokumen ini menjelaskan cara menjalankan SigardaPanel **aman tanpa bentrok** dengan CloudPanel yang sudah ada, serta fitur migrasi dari CloudPanel ke SigardaPanel.

## Status Sistem Saat Ini

| Komponen | CloudPanel | Port | Status |
|----------|-----------|------|--------|
| Nginx | ✅ CloudPanel | 80 / 443 / 8080 / 8443 | Active |
| MariaDB | ✅ CloudPanel | 3306 | Active |
| PostgreSQL | ✅ Sistem | 5432 | Active |
| Redis | ✅ Sistem | 6379 | Active |
| SigardaPanel Web | ✅ Next.js | 3000 | Running |
| Sites terdaftar | ±19 domain | - | Active |

## Port Aman untuk SigardaPanel

| Service | Default | Ganti Jadi | Alasan |
|---------|---------|-----------|--------|
| API Backend | `:7700` | `:7700` | 8080 dipakai CloudPanel nginx |
| Agent | `:7710` | `:7710` | Bebas ✅ |
| DB | SQLite file | SQLite file | Aman ✅ |
| Nginx/Caddy | managed | **TIDAK disentuh** | CloudPanel sudah manage nginx |

## Strategi Aman (No-Conflict)

### 1. Tidak sentuh Nginx CloudPanel
- SigardaPanel **tidak mengelola /etc/nginx/** CloudPanel
- Nginx template SigardaPanel hanya dipakai internal
- Sites buatan SigardaPanel path terpisah: `/var/www/sigarda/`

### 2. Mode jalan SigardaPanel
```bash
# API mode — port aman 8090
SIGARDAPANEL_API_ADDR=:7700 \
SIGARDAPANEL_DB_PATH=/opt/sigardapanel/data/sigardapanel.db \
sigardapanel api

# Agent mode — port 7710
SIGARDAPANEL_AGENT_ADDR=:7710 \
sigardapanel agent

# Web dashboard — sudah jalan di port 3000
cd web && npm run dev
```

### 3. Database
- Pakai SQLite → tidak sentuh MariaDB/PostgreSQL CloudPanel
- File DB di `/opt/sigardapanel/data/`

## Fitur Migrasi CloudPanel → SigardaPanel

### Yang bisa dimigrasi
- ✅ Sites (domain, root path, nginx config)
- ✅ SSL certificates (Let's Encrypt)
- ✅ PHP version & settings
- ✅ env vars per site
- ⏳ Database users & grants (future)
- ⏳ Email accounts (future)

### Yang tidak bisa otomatis
- DNS records (harus manual)
- File content (tetap di `/home/` path lama saat cutover)
- Email inboxes

### Alur Migrasi
1. `sigardapanel cloudpanel import-sites` → parse nginx conf CloudPanel
2. `sigardapanel cloudpanel import-ssl` → baca cert dari Let's Encrypt
3. `sigardapanel cloudpanel migrate-site <domain>` → pindah site tertentu
4. `sigardapanel cloudpanel doctor` → cek kesiapan migrasi

## Perintah CLI yang Ditambahkan

```bash
sigardapanel cloudpanel scan                    # scan CloudPanel sites
sigardapanel cloudpanel import-sites            # import semua site ke panel
sigardapanel cloudpanel import-site <domain>    # import satu site
sigardapanel cloudpanel import-ssl              # import SSL certs
sigardapanel cloudpanel doctor                  # diagnosa CloudPanel
```

## File Output Migrasi
- `/opt/sigardapanel/migrations/cloudpanel/sites.json` — data semua site CloudPanel
- `/opt/sigardapanel/migrations/cloudpanel/ssl.json` — data SSL
- `/opt/sigardapanel/migrations/cloudpanel/report.json` — laporan lengkap

---
**Lihat panduan lengkap: [GUIDE.md](GUIDE.md)**
