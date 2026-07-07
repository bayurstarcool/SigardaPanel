# API Reference

This document defines the public API for SigardaPanel. The dashboard and CLI use the same API endpoints.

## API Principles

- All public endpoints use the `/api/v1` prefix
- Consistent JSON response format
- All responses include a `request_id` for tracing
- All mutation endpoints perform RBAC checks and audit logging
- All list endpoints support pagination
- Secrets are never returned in full after creation

## Authentication

### Methods

- **Cookie session** — for dashboard access
- **Bearer token** — for CLI and API access

### Headers

```http
Authorization: Bearer <token>
Content-Type: application/json
```

## Response Format

### Success (Single Resource)

```json
{
  "data": {},
  "request_id": "req_123"
}
```

### Success (List)

```json
{
  "data": [],
  "pagination": {
    "page": 1,
    "per_page": 25,
    "total": 100
  },
  "request_id": "req_123"
}
```

### Error

```json
{
  "error": {
    "code": "permission_denied",
    "message": "Permission denied",
    "details": null
  },
  "request_id": "req_123"
}
```

## HTTP Status Codes

| Code | Description |
|------|-------------|
| `200` | Success |
| `201` | Resource created |
| `202` | Job accepted (async) |
| `400` | Invalid input |
| `401` | Not authenticated |
| `403` | Permission denied |
| `404` | Resource not found |
| `409` | Conflict / invalid state |
| `422` | Domain, path, or runtime validation failed |
| `429` | Rate limited |
| `500` | Server error |
| `502` | Agent or target server error |
| `504` | Agent or job timeout |

## Pagination

| Parameter | Default | Description |
|-----------|---------|-------------|
| `page` | `1` | Page number |
| `per_page` | `25` | Items per page (max: 100) |
| `sort` | `created_at` | Sort field |
| `order` | `desc` | Sort order |
| `q` | - | Search query |

## Authentication Endpoints

### Login

```
POST /api/v1/auth/login
```

**Request:**

```json
{
  "email": "admin@example.com",
  "password": "secret"
}
```

**Response:**

```json
{
  "data": {
    "user": {
      "id": "usr_123",
      "email": "admin@example.com",
      "name": "Admin"
    },
    "token": "shown_once_for_cli_if_requested"
  },
  "request_id": "req_123"
}
```

### Logout

```
POST /api/v1/auth/logout
```

Revoke current session or token.

### Current User

```
GET /api/v1/auth/me
```

Returns current user and permissions.

## User Endpoints

| Method | Endpoint | Permission | Description |
|--------|----------|------------|-------------|
| `GET` | `/api/v1/users` | `user.manage` | List users |
| `POST` | `/api/v1/users` | `user.manage` | Create user |
| `PATCH` | `/api/v1/users/{user_id}` | `user.manage` | Update user |
| `POST` | `/api/v1/users/{user_id}/disable` | `user.manage` | Disable user |

## API Token Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/api-tokens` | List token metadata |
| `POST` | `/api/v1/api-tokens` | Create token (shown once) |
| `DELETE` | `/api/v1/api-tokens/{token_id}` | Revoke token |

## Server Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/servers` | List servers |
| `POST` | `/api/v1/servers` | Create server and bootstrap token |
| `GET` | `/api/v1/servers/{server_id}` | Server details |
| `DELETE` | `/api/v1/servers/{server_id}` | Remove server (guards active sites) |
| `GET` | `/api/v1/servers/{server_id}/health` | Agent health status |
| `GET` | `/api/v1/servers/{server_id}/capabilities` | Capability report |
| `POST` | `/api/v1/servers/{server_id}/doctor` | Run diagnostics |

## Agent Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/v1/agents/register` | Register agent with bootstrap token |
| `POST` | `/api/v1/agents/heartbeat` | Agent heartbeat |
| `POST` | `/api/v1/agents/capabilities` | Update capabilities |

Agent endpoints require dedicated authentication middleware.

## Site Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/sites` | List sites (filters: `server_id`, `runtime`, `status`, `q`) |
| `POST` | `/api/v1/sites` | Create site (returns job) |
| `GET` | `/api/v1/sites/{site_id}` | Site details |
| `PATCH` | `/api/v1/sites/{site_id}` | Update site configuration |
| `DELETE` | `/api/v1/sites/{site_id}` | Delete site (requires confirmation) |
| `POST` | `/api/v1/sites/{site_id}/enable` | Enable site |
| `POST` | `/api/v1/sites/{site_id}/disable` | Disable site |

## Deployment Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/v1/sites/{site_id}/deployments` | Create deployment job |
| `GET` | `/api/v1/sites/{site_id}/deployments` | List deployments |
| `GET` | `/api/v1/deployments/{deployment_id}` | Deployment details |
| `POST` | `/api/v1/deployments/{deployment_id}/rollback` | Rollback to previous deployment |

## SSL Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/v1/sites/{site_id}/ssl/issue` | Issue SSL certificate |
| `POST` | `/api/v1/sites/{site_id}/ssl/renew` | Renew SSL certificate |
| `GET` | `/api/v1/sites/{site_id}/ssl/status` | SSL status and expiry |

## Log Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/sites/{site_id}/logs` | Query logs (`type`: access, error, app) |
| `GET` | `/api/v1/jobs/{job_id}/logs` | Job logs |

Streaming support via SSE/WebSocket planned:
- `GET /api/v1/jobs/{job_id}/stream`
- `GET /api/v1/sites/{site_id}/logs/stream`

## Job Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/jobs` | List jobs |
| `GET` | `/api/v1/jobs/{job_id}` | Job details |
| `POST` | `/api/v1/jobs/{job_id}/cancel` | Cancel job (best-effort) |
| `POST` | `/api/v1/jobs/{job_id}/retry` | Retry job (if safe) |

## Backup Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/v1/backups` | Create backup job |
| `GET` | `/api/v1/backups` | List backups |
| `POST` | `/api/v1/backups/{backup_id}/restore` | Restore backup (requires confirmation) |

## Metrics Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/servers/{server_id}/metrics` | Server metrics (recent/history) |
| `GET` | `/api/v1/sites/{site_id}/metrics` | Site metrics |

## Audit Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/audit-logs` | List audit logs (filters: `actor_id`, `action`, `target_type`, `target_id`, `server_id`, `status`) |

## Webhook Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/v1/webhooks/git/{site_id}` | Git deploy webhook (requires signature validation) |

## Idempotency

For critical mutations, clients may send:

```http
Idempotency-Key: <unique-key>
```

The backend prevents duplicate creation or deployment if the same key is used.

## Rate Limiting

| Endpoint | Limit |
|----------|-------|
| Login | Strict per IP/email |
| API token creation | Moderate |
| Webhook | Moderate with signature |
| Log streaming | Connection limit |
