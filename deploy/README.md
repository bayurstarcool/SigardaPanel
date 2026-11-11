# deploy/

Supporting files for operators. **To install or update SigardaPanel, use
`install.sh` - see [docs/INSTALL.md](../docs/INSTALL.md).**

| File | Purpose |
|---|---|
| `sigardapanel-info.sh` | SSH status dashboard (`sigardapanel-info`, `--watch`). A copy is embedded in the binary and served at `/api/v1/agents/status-dashboard`. To use it on a server, install it as `/etc/profile.d/sigardapanel-info.sh` (mode 755) and link it to `/usr/local/bin/sigardapanel-info`. |
| `nginx/pma-per-database.conf` | Example nginx snippet for the per-database phpMyAdmin bridge. |

The former `install-panel.sh` and `install-agent.sh` were removed: they
downloaded release assets (v0.5.5) that no longer exist. Panels are installed
with `install.sh`; remote agents as described in
[Adding remote servers](../docs/INSTALL.md#adding-remote-servers).
