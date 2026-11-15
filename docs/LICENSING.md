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
| `tier` | `free`, `basic`, `pro`, `enterprise` or `max`. Required. |
| `max_servers` | Server limit; `0` = the tier's default. |
| `issued_at` | RFC 3339 time. Required. |
| `expires_at` | RFC 3339 time; omitted or zero = never expires. |
| `install_id` | Binds the key to one installation; another panel refuses it. Keys delivered by check-in are bound. |

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

## Check-in: every installation has a license

Once a day (first a few minutes after start) the panel checks in with the
master panel (`SIGARDAPANEL_MASTER_URL`, default `https://panel.sigarda.com`):

- The first check-in brings a **free license** bound to the installation;
  nothing to do for the admin.
- **Upgrading:** Settings → License → *Upgrade plan* opens the master's
  billing page for this installation. Once the order is approved, the paid
  license arrives with the next check-in (or *Check now*).
- Renewals arrive the same way; a license revoked on the master is dropped.
- The answer also carries the latest version.

What is sent (Settings → License → *What is sent* shows the exact data):

| Field | Example |
|---|---|
| `install_id` | random UUID made at install (no name, domain or email) |
| `version`, `tier`, `license_id` | `1.13.0`, `free`, `SPL-…` |
| `os`, `arch` | `linux`, `amd64` |
| `servers`, `sites` | ranges: `1`, `2-5`, `6-20`, `21-100`, `100+` |

Check-in never blocks the panel: without the master (offline servers,
outages) it keeps running on its current license and retries hourly. Turn
it off with `SIGARDAPANEL_CHECKIN=off` in `/etc/sigardapanel/panel.env`;
licenses are then activated by hand.
