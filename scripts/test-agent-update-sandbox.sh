#!/usr/bin/env bash
# Sandbox test of installer/agent-update.sh (paths redirected, system tools stubbed).
set -uo pipefail
SRC="${1:-$(dirname "$0")/../installer/agent-update.sh}"; T="$(mktemp -d)"; trap 'rm -rf "$T"' EXIT; export T
mkdir -p "$T/bin" "$T/usr/local/bin" "$T/etc" "$T/backups" "$T/assets"
sed -e "s|^INSTALL_BIN=.*|INSTALL_BIN=\"$T/usr/local/bin/sigardapanel\"|" -e "s|^ENV_FILE=.*|ENV_FILE=\"$T/etc/agent.env\"|" \
    -e "s|^BACKUP_DIR=.*|BACKUP_DIR=\"$T/backups\"|" "$SRC" > "$T/agent-update.sh"
cat > "$T/bin/systemctl" <<'S'
#!/bin/sh
echo "systemctl $*" >> "$T/systemctl.log"
S
cat > "$T/bin/curl" <<'S'
#!/bin/sh
out=""; url=""
while [ $# -gt 0 ]; do case "$1" in -o) out="$2"; shift;; http*|file*) url="$1";; esac; shift; done
if [ -n "$out" ]; then cp "$T/assets/${url##*/}" "$out"; exit; fi
echo "$url" > "$T/health-url"
grep -q broken "$T/usr/local/bin/sigardapanel" && exit 7
printf '{"data":{"status":"ok","version":"%s"}}' "$("$T/usr/local/bin/sigardapanel")"
S
# Runs unprivileged in CI: "id -u" reports root, anything else is the real id.
cat > "$T/bin/id" <<'S'
#!/bin/sh
[ "$*" = "-u" ] && { echo 0; exit 0; }
exec /usr/bin/id "$@"
S
for f in sleep minisign; do printf '#!/bin/sh\nexit 0\n' > "$T/bin/$f"; done
chmod +x "$T/bin/"*; export PATH="$T/bin:$PATH"
case "$(uname -m)" in aarch64|arm64) A=arm64;; *) A=amd64;; esac
mkbin() { printf '#!/bin/sh\necho %s\n# %s\n' "$1" "${2:-}" > "$3"; chmod +x "$3"; }
release() { mkbin "$1" "${2:-}" "$T/assets/sigardapanel-linux-$A"; ( cd "$T/assets" && sha256sum "sigardapanel-linux-$A" > SHA256SUMS && echo s > SHA256SUMS.minisig ); }
ver() { "$T/usr/local/bin/sigardapanel"; }
fail=0; check() { local n="$1"; shift; if "$@"; then echo "PASS  $n"; else echo "FAIL  $n"; fail=1; fi; }
echo "SIGARDAPANEL_AGENT_ADDR=:7711" > "$T/etc/agent.env"
mkbin 1.6.0 "" "$T/usr/local/bin/sigardapanel"

release 1.7.0
SIGARDA_ASSET_BASE=file:///x bash "$T/agent-update.sh" v1.7.0 >"$T/out" 2>&1; rc=$?
check "update exits 0" test "$rc" = 0; [ "$rc" = 0 ] || cat "$T/out"
check "agent binary is 1.7.0" test "$(ver)" = 1.7.0
check "agent restarted" grep -q "restart sigardapanel-agent" "$T/systemctl.log"
check "health uses the port from agent.env" grep -q "http://127.0.0.1:7711/health" "$T/health-url"
check "backup of 1.6.0 kept" sh -c "grep -q 1.6.0 $T/backups/*/VERSION"

release 1.8.0 broken
SIGARDA_ASSET_BASE=file:///x bash "$T/agent-update.sh" v1.8.0 >"$T/out" 2>&1; rc=$?
check "unhealthy update exits non-zero" test "$rc" != 0
check "1.7.0 restored" test "$(ver)" = 1.7.0
check "message says restored and healthy" grep -q "restored 1.7.0 and the agent is healthy again" "$T/out"

mkbin 1.8.5 "" "$T/assets/sigardapanel-linux-$A"; ( cd "$T/assets" && sha256sum "sigardapanel-linux-$A" > SHA256SUMS )
SIGARDA_ASSET_BASE=file:///x bash "$T/agent-update.sh" v1.9.0 >"$T/out" 2>&1; rc=$?
check "binary reporting another version is refused" sh -c "[ $rc != 0 ] && grep -q \"expected 1.9.0\" $T/out && [ \"\$($T/usr/local/bin/sigardapanel)\" = 1.7.0 ]"

release 1.9.0; echo "deadbeef  sigardapanel-linux-$A" > "$T/assets/SHA256SUMS"
SIGARDA_ASSET_BASE=file:///x bash "$T/agent-update.sh" v1.9.0 >"$T/out" 2>&1; rc=$?
check "checksum mismatch refused" sh -c "[ $rc != 0 ] && grep -q 'CHECKSUM MISMATCH' $T/out"

for v in 1.9.1 1.9.2 1.9.3 1.9.4; do release $v; SIGARDA_ASSET_BASE=file:///x bash "$T/agent-update.sh" v$v >/dev/null 2>&1; /bin/sleep 1; done
check "only 3 backups kept" test "$(ls "$T/backups" | wc -l)" = 3
bash "$T/agent-update.sh" '1.9.4;rm -rf /' >/dev/null 2>&1; check "bad tag refused" test $? != 0
exit $fail
