#!/usr/bin/env bash
# Sandbox test of installer/install.sh "update" and "rollback": the real script with
# its paths redirected to a temp dir and systemctl/curl/sqlite3/minisign stubbed.
# Usage: scripts/test-installer-sandbox.sh [installer/install.sh]
set -uo pipefail
SRC="${1:-$(dirname "$0")/../installer/install.sh}"; T="$(mktemp -d)"; trap 'rm -rf "$T"' EXIT
mkdir -p "$T/bin" "$T/usr/local/bin" "$T/opt" "$T/etc" "$T/data" "$T/backups" "$T/assets/web"
sed -e "s|^INSTALL_BIN=.*|INSTALL_BIN=\"$T/usr/local/bin/sigardapanel\"|" \
    -e "s|^WEB_DIR=.*|WEB_DIR=\"$T/opt/web\"|" -e "s|^CONF_DIR=.*|CONF_DIR=\"$T/etc\"|" \
    -e "s|^DATA_DIR=.*|DATA_DIR=\"$T/data\"|" -e "s|^UPDATE_BACKUPS=.*|UPDATE_BACKUPS=\"$T/backups\"|" "$SRC" > "$T/install.sh"
export T
cat > "$T/bin/systemctl" <<'S'
#!/bin/sh
echo "systemctl $*" >> "$T/systemctl.log"
S
cat > "$T/bin/curl" <<'S'
#!/bin/sh
out=""; url=""
while [ $# -gt 0 ]; do case "$1" in -o) out="$2"; shift;; http*|file*) url="$1";; esac; shift; done
if [ -n "$out" ] && [ "$out" != /dev/null ]; then cp "$T/assets/${url##*/}" "$out"; exit; fi
[ -f "$T/healthy" ] && ! grep -q broken "$T/usr/local/bin/sigardapanel"
S
cat > "$T/bin/sqlite3" <<'S'
#!/bin/sh
dst=$(echo "$2" | sed -e "s/^.backup '//" -e "s/'$//"); cp "$1" "$dst"
S
# Runs unprivileged in CI: "id -u" reports root, anything else is the real id.
cat > "$T/bin/id" <<'S'
#!/bin/sh
[ "$*" = "-u" ] && { echo 0; exit 0; }
exec /usr/bin/id "$@"
S
for f in chown sleep minisign; do printf '#!/bin/sh\nexit 0\n' > "$T/bin/$f"; done
chmod +x "$T/bin/"*
export PATH="$T/bin:$PATH"
mkbin() { printf '#!/bin/sh\necho %s\n# %s\n' "$1" "${2:-}" > "$3"; chmod +x "$3"; }
ver() { "$T/usr/local/bin/sigardapanel"; }
fail=0
check() { local name="$1"; shift; if "$@"; then echo "PASS  $name"; else echo "FAIL  $name"; fail=1; fi; }

# --- state before the update: 1.6.0
echo "SIGARDAPANEL_DB_PATH=$T/data/sigardapanel.db" > "$T/etc/panel.env"; echo x > "$T/etc/agent.env"
mkbin 1.6.0 "" "$T/usr/local/bin/sigardapanel"; mkdir -p "$T/opt/web"; echo old > "$T/opt/web/index.js"; echo db-1.6.0 > "$T/data/sigardapanel.db"
touch "$T/healthy"

echo "--- update 1.6.0 -> 1.8.0"
case "$(uname -m)" in aarch64|arm64) A=arm64;; *) A=amd64;; esac
mkbin 1.8.0 "" "$T/assets/sigardapanel-linux-$A"; echo v18 > "$T/assets/web/index.js"
# Owner-only modes in the archive (as v1.7.0 shipped) must still install readable.
mkdir -p "$T/assets/web/client"; echo js > "$T/assets/web/client/app.js"
chmod -R go-rwx "$T/assets/web"
tar -czf "$T/assets/sigardapanel-web.tar.gz" -C "$T/assets/web" .
( cd "$T/assets" && sha256sum "sigardapanel-linux-$A" sigardapanel-web.tar.gz > SHA256SUMS && echo sig > SHA256SUMS.minisig )
SIGARDA_VERSION=v1.8.0 SIGARDA_ASSET_BASE=file:///x bash "$T/install.sh" update >/dev/null 2>"$T/err"; rc=$?
check "update exits 0" test "$rc" = 0; [ "$rc" = 0 ] || cat "$T/err"
check "binary is 1.8.0" test "$(ver)" = 1.8.0
check "web is 1.8.0" grep -q v18 "$T/opt/web/index.js"
check "web files are world-readable, directories traversable" \
  sh -c "[ -z \"\$(find '$T/opt/web' \\( -type f ! -perm -o=r \\) -o \\( -type d ! -perm -o=rx \\))\" ]"
upbk="$(ls "$T/backups" | head -1)"
check "update backup records VERSION/TARGET/KIND" sh -c "grep -q 1.6.0 '$T/backups/$upbk/VERSION' && grep -q v1.8.0 '$T/backups/$upbk/TARGET' && grep -q '^update$' '$T/backups/$upbk/KIND'"
echo db-1.8.0 > "$T/data/sigardapanel.db"; touch "$T/data/sigardapanel.db-wal"

echo "--- broken update 1.8.0 -> 1.9.0 rolls back"
mkbin 1.9.0 broken "$T/assets/sigardapanel-linux-$A"
( cd "$T/assets" && sha256sum "sigardapanel-linux-$A" sigardapanel-web.tar.gz > SHA256SUMS )
/bin/sleep 1
SIGARDA_VERSION=v1.9.0 SIGARDA_ASSET_BASE=file:///x bash "$T/install.sh" update >/dev/null 2>"$T/err"; rc=$?
check "broken update exits non-zero" test "$rc" != 0
check "broken update rolled back to 1.8.0" test "$(ver)" = 1.8.0
check "message says healthy after rollback" grep -q 'rolled back to 1.8.0 and the panel is healthy again' "$T/err"
rm -f "$T/data/sigardapanel.db-wal"; echo db-1.8.0 > "$T/data/sigardapanel.db"; touch "$T/data/sigardapanel.db-wal"

echo "--- rollback list + rollback to the newest update backup"
bash "$T/install.sh" rollback list
newest="$(bash "$T/install.sh" rollback list | awk -F'\t' 'NR>1 && $3=="update"{print $1; exit}')"
check "newest update backup is the 1.8.0 one" grep -q 1.8.0 "$T/backups/$newest/VERSION"
bash "$T/install.sh" rollback "$upbk" >/dev/null 2>"$T/err"; rc=$?
check "rollback exits 0" test "$rc" = 0; [ "$rc" = 0 ] || cat "$T/err"
check "binary restored to 1.6.0" test "$(ver)" = 1.6.0
check "web restored" grep -q old "$T/opt/web/index.js"
check "db restored, wal removed" sh -c "grep -q db-1.6.0 '$T/data/sigardapanel.db' && [ ! -e '$T/data/sigardapanel.db-wal' ]"
safety="$(grep -l pre-rollback "$T"/backups/*/KIND | head -1)"; safety="${safety%/KIND}"
check "safety backup holds the 1.8.0 state" sh -c "[ -n '$safety' ] && grep -q 1.8.0 '$safety/VERSION' && grep -q db-1.8.0 '$safety/sigardapanel.db'"
check "services stopped before the restore" grep -q 'stop sigardapanel-web sigardapanel-api sigardapanel-agent' "$T/systemctl.log"
check "default target skips pre-rollback backups" sh -c "bash '$T/install.sh' rollback list | awk -F'\t' 'NR>1 && \$3==\"update\"{print \$1;exit}' | grep -qv \"$(basename "$safety")\""

echo "--- rollback to a broken backup restores the current state"
mkdir -p "$T/backups/20200101T000000Z/web"; echo w > "$T/backups/20200101T000000Z/web/index.js"
mkbin 1.5.0 broken "$T/backups/20200101T000000Z/sigardapanel.bin"; echo db-1.5.0 > "$T/backups/20200101T000000Z/sigardapanel.db"
bash "$T/install.sh" rollback 20200101T000000Z >/dev/null 2>"$T/err"; rc=$?
check "unhealthy rollback exits non-zero" test "$rc" != 0
check "1.6.0 binary is back" test "$(ver)" = 1.6.0
check "1.6.0 db is back" grep -q db-1.6.0 "$T/data/sigardapanel.db"
check "error explains the restore" grep -q 'restored 1.6.0 and the panel is healthy again' "$T/err"

echo "--- invalid names"
bash "$T/install.sh" rollback '../../etc' >/dev/null 2>&1; check "path traversal refused" test $? != 0
bash "$T/install.sh" rollback 20990101T000000Z >/dev/null 2>&1; check "missing backup refused" test $? != 0
exit $fail
