# installer/

`install.sh` is the SigardaPanel installer and updater:

```bash
sudo bash install.sh            # install the latest signed release
sudo bash install.sh update     # update with backup + automatic rollback
sudo bash install.sh rollback   # restore the state before the last update (list: rollback list)
bash install.sh verify          # print the pinned release-signing key
```

How to use it, all options and what it changes on the server:
**[docs/INSTALL.md](../docs/INSTALL.md)**.

Notes for maintainers:

- The top-level [`install.sh`](../install.sh) is what users download
  (`raw.githubusercontent.com/.../main/install.sh`). It must stay byte-identical
  to `installer/install.sh`; `scripts/test-installers.sh` checks this.
- The minisign public key in `install.sh` (`PUBKEY`) must match the key used by
  `scripts/release.sh` and the key printed in the README. Never change it
  without a key-rotation plan: existing users verify updates with it.
- `install.sh` is also embedded in the binary (`installer/embed.go`): the
  panel's one-click/automatic update (Settings → Panel update) runs that copy
  as `install.sh update`, so `update` must keep working non-interactively with
  only `SIGARDA_VERSION` set, and must exit non-zero on failure.
- `agent-update.sh` is embedded too: remote agents run it to update themselves
  (`bash agent-update.sh vX.Y.Z`). Its `PUBKEY` must equal the one in
  `install.sh` and `releasePublicKey` in `internal/apihttp/agents.go`.
- `scripts/test-installer-sandbox.sh` and `scripts/test-agent-update-sandbox.sh`
  run both scripts against stubbed system tools (update, failed update,
  rollback, failed rollback, checksum/version checks).
- CI runs `bash -n` and `shellcheck -S error` on `installer/install.sh`.
- Changes are tested end-to-end in a throwaway systemd container (Ubuntu 24.04):
  install, `verify`, `update` (including a forced failure to exercise the rollback).

Security reports: see [SECURITY.md](../SECURITY.md).
