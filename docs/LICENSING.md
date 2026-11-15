# Licensing

The panel is open source; a paid tier is unlocked with a **license key**.
Keys are issued and signed by the master panel (the seller); each panel
verifies them offline, so no license server is contacted and a key cannot be
made up or edited. Selling licenses (plans, orders, payments) lives in the
master panel, not in this repository.

## Key format

```
SPL1.<base64url(JSON claims)>.<base64url(ed25519 signature)>
```

The signature covers `SPL1.` followed by the encoded claims. Claims:

| Field | Meaning |
|---|---|
| `id` | License id (unique, e.g. the order number). Required. |
| `licensee`, `email` | Customer. |
| `tier` | `basic`, `pro`, `enterprise` or `max`. Required. |
| `max_servers` | Server limit; `0` = the tier's default. |
| `issued_at` | RFC 3339 time. Required. |
| `expires_at` | RFC 3339 time; omitted or zero = never expires. |

Unknown fields are ignored, so new claims can be added without breaking older
panels. Implemented in `internal/license` (Go: `license.Sign`,
`license.Verify`).

## The signing key

1. Create the key pair once, on a trusted machine:

   ```bash
   go run ./cmd/licensetool keygen -out license.key
   ```

   It prints the **public key**. `license.key` is the private key: keep it
   with the master panel only (a secret manager), never commit it. Whoever
   holds it can issue licenses.

2. Build the public key into the panel, either:
   - in `internal/license/keys.go` (`defaultPublicKeys`), or
   - as the repository variable `SIGARDAPANEL_LICENSE_PUBLIC_KEYS`: the
     release workflow passes it to the build (`-ldflags -X
     sigardapanel/internal/license.PublicKeys=...`).

   Several keys (comma separated) allow a rotation: add the new key, release,
   start signing with it, and drop the old key once its licenses expired.

A build without a public key cannot verify licenses: every panel runs on the
free tier and activation answers "this build has no license public key".

## Issuing and checking keys

```bash
go run ./cmd/licensetool issue -key license.key -id ORD-2026-001 \
  -licensee "Acme Hosting" -email ops@acme.example -tier pro -servers 5 -days 365
go run ./cmd/licensetool verify -pub <public key> 'SPL1....'
```

## In the panel

Settings → License (super admin): paste the key and activate. The panel
stores it and verifies it (signature and expiry) every time the license is
read, so an expired key returns the panel to the free tier on its own. Keys
activated before v1.13 (plain hex, not signed) are reported as not valid.
