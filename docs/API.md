# API Reference

SigardaPanel API v0.5.0 — ~150+ endpoints.

## Base URL

```
https://panel.sigarda.dev/api/v1
```

## Authentication

All endpoints require `Authorization: Bearer <token>` header except public endpoints.

## Response Format

```json
{
  "data": { ... },
  "request_id": "req_xxx"
}
```

Error format:
```json
{
  "error": {
    "code": "unauthorized",
    "message": "invalid token"
  },
  "request_id": "req_xxx"
}
```

## Endpoints

### Auth
| Method | Path | Description |
|--------|------|-------------|
| POST | /auth/login | Login |
| GET | /auth/me | Current user |
| PUT | /auth/profile | Update profile |
| POST | /auth/logout | Logout |

### Servers
| Method | Path | Description |
|--------|------|-------------|
| GET | /servers | List servers |
| POST | /servers | Create server |
| GET | /servers/:id | Server detail |
| PATCH | /servers/:id | Update server |
| DELETE | /servers/:id | Delete server |
| GET | /servers/health | All servers health |
| GET | /servers/:id/health | Server health |
| POST | /servers/:id/token | Regenerate agent token |

### Sites
| Method | Path | Description |
|--------|------|-------------|
| GET | /sites | List sites |
| POST | /sites | Create site |
| GET | /sites/:id | Site detail |
| DELETE | /sites/:id | Delete site |
| POST | /sites/:id/deploy | Deploy site |
| POST | /sites/:id/config | Update config |
| POST | /sites/:id/reload-nginx | Reload nginx |
| GET | /sites/:id/vhost | Get vhost |
| PUT | /sites/:id/vhost | Update vhost |

### Docker
| Method | Path | Description |
|--------|------|-------------|
| GET | /servers/:id/docker/containers | List containers |
| POST | /servers/:id/docker/container/start | Start container |
| POST | /servers/:id/docker/container/stop | Stop container |
| POST | /servers/:id/docker/container/restart | Restart container |
| POST | /servers/:id/docker/container/remove | Remove container |
| GET | /servers/:id/docker/container/logs | Container logs |
| GET | /servers/:id/docker/images | List images |
| POST | /servers/:id/docker/image/pull | Pull image |
| POST | /servers/:id/docker/image/remove | Remove image |
| GET | /servers/:id/docker/volumes | List volumes |
| POST | /servers/:id/docker/volume/create | Create volume |
| POST | /servers/:id/docker/volume/remove | Remove volume |
| GET | /servers/:id/docker/networks | List networks |
| POST | /servers/:id/docker/network/create | Create network |
| POST | /servers/:id/docker/network/remove | Remove network |
| GET | /servers/:id/docker/info | Docker info |
| GET | /servers/:id/docker/disk-usage | Disk usage |

### Firewall
| Method | Path | Description |
|--------|------|-------------|
| GET | /servers/:id/firewall/status | Firewall status |
| POST | /servers/:id/firewall/enable | Enable firewall |
| POST | /servers/:id/firewall/disable | Disable firewall |
| POST | /servers/:id/firewall/allow | Allow port |
| POST | /servers/:id/firewall/deny | Deny port |
| POST | /servers/:id/firewall/delete | Delete rule |
| GET | /servers/:id/firewall/rules | List rules |
| POST | /servers/:id/firewall/reset | Reset firewall |

### Redis
| Method | Path | Description |
|--------|------|-------------|
| GET | /servers/:id/redis/stats | Redis stats |
| POST | /servers/:id/redis/flush | Flush all |
| GET | /servers/:id/redis/info | Redis info |
| POST | /servers/:id/redis/flushdb | Flush database |

### Git
| Method | Path | Description |
|--------|------|-------------|
| GET | /sites/:id/git/branches | List branches |
| GET | /sites/:id/git/log | Commit history |
| POST | /sites/:id/git/rollback | Rollback to commit |
| POST | /sites/:id/git/checkout | Checkout branch |

### SSL
| Method | Path | Description |
|--------|------|-------------|
| GET | /sites/:id/ssl | SSL status |
| POST | /sites/:id/ssl/issue | Issue cert |
| POST | /sites/:id/ssl/renew | Renew cert |
| POST | /ssl/renew-all | Batch renew all |

### Databases
| Method | Path | Description |
|--------|------|-------------|
| GET | /databases | List all databases |
| POST | /sites/:id/databases | Create database |
| GET | /sites/:id/databases | List site databases |
| GET | /databases/:id | Database detail |
| PATCH | /databases/:id | Update database |
| DELETE | /databases/:id | Delete database |
| POST | /databases/:id/users | Create DB user |
| GET | /databases/:id/users | List DB users |
| DELETE | /databases/:id/users/:user_id | Delete DB user |
| POST | /databases/:id/users/:user_id/rotate-password | Rotate password |

### Backups
| Method | Path | Description |
|--------|------|-------------|
| POST | /backups/bulk | Bulk create |
| POST | /sites/:id/backups | Create backup |
| POST | /sites/:id/backups/database | Database backup |
| GET | /backups | List backups |
| GET | /backups/:id/download | Download backup |
| DELETE | /backups/:id | Delete backup |
| POST | /backups/:id/restore | Restore backup |

### Jobs
| Method | Path | Description |
|--------|------|-------------|
| GET | /jobs | List jobs |
| GET | /jobs/:id | Job detail |
| POST | /jobs/:id/cancel | Cancel job |

### Cloudflare
| Method | Path | Description |
|--------|------|-------------|
| GET | /cloudflare/config | Get config |
| PUT | /cloudflare/config | Save config |
| DELETE | /cloudflare/config | Delete config |
| GET | /cloudflare/verify | Verify token |
| GET | /cloudflare/zones | List zones |
| GET | /cloudflare/zones/:id/dns | List DNS records |
| POST | /cloudflare/zones/:id/dns | Create DNS record |
| DELETE | /cloudflare/zones/:id/dns/:recordId | Delete DNS record |

### Stack
| Method | Path | Description |
|--------|------|-------------|
| GET | /servers/:id/stack | Stack info |
| POST | /servers/:id/stack/action | Install/manage |
| POST | /servers/:id/stack/restart | Restart services |
| POST | /servers/:id/command | Execute command |
