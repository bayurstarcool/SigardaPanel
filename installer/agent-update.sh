#!/usr/bin/env bash
# SigardaPanel agent updater for remote servers (servers added with the agent
# install command). Embedded in the binary and run by the agent itself, in its
# own systemd unit, when the panel asks it to update:
#
#   bash agent-update.sh v1.7.0
#
# The release is verified against the pinned minisign key and SHA256SUMS, the
# current binary is backed up, and the previous binary is restored when the
# updated agent does not come back healthy on the expected version.
set -euo pipefail
umask 022

REPO="bayurstarcool/SigardaPanel"
# Must be the same key as installer/install.sh (scripts/test-installers.sh checks it).
PUBKEY="RWTNXRQ/oh97GPfWH0tDwxbnMTe/WTZeoJdZtGTW3ESF0OpTSlNXpFXI"
INSTALL_BIN="/usr/local/bin/sigardapanel"
ENV_FILE="/etc/sigardapanel/agent.env"
BACKUP_DIR="/var/backups/sigardapanel-agent-updates"
SERVICE="sigardapanel-agent"
KEEP_BACKUPS=3

log()  { echo "[agent-update] $*"; }
ok()   { echo "[  OK  ] $*"; }
warn() { echo "[ WARN ] $*"; }
die()  { echo "[ FAIL ] $*" >&2; exit 1; }

TMP=""
cleanup() { if [ -n "$TMP" ]; then rm -rf "$TMP"; fi; }
trap cleanup EXIT

TAG="${1:-}"
[[ "$TAG" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "usage: agent-update.sh vX.Y.Z"
[ "$(id -u)" -eq 0 ] || die "must run as root"
[ -x "$INSTALL_BIN" ] || die "${INSTALL_BIN} not found"
[ -f "$ENV_FILE" ] || die "${ENV_FILE} not found: this server was not set up with the agent install command"

case "$(uname -m)" in
  x86_64|amd64)  ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) die "unsupported CPU architecture: $(uname -m)" ;;
esac

if ! command -v minisign >/dev/null; then
  log "Installing minisign (needed to verify the release)..."
  if command -v apt-get >/dev/null; then
    DEBIAN_FRONTEND=noninteractive apt-get install -y -qq minisign >/dev/null 2>&1 \
      || { apt-get update -qq >/dev/null 2>&1; DEBIAN_FRONTEND=noninteractive apt-get install -y -qq minisign >/dev/null 2>&1; } || true
  elif command -v dnf >/dev/null; then dnf install -y -q minisign >/dev/null 2>&1 || true
  elif command -v yum >/dev/null; then yum install -y -q minisign >/dev/null 2>&1 || true
  fi
  command -v minisign >/dev/null || die "minisign is not installed and could not be installed; install it and retry"
fi

# agent_url: where the local agent listens, from SIGARDAPANEL_AGENT_ADDR.
agent_url() {
  local addr port scheme=http
  addr="$(grep -E '^SIGARDAPANEL_AGENT_ADDR=' "$ENV_FILE" | tail -n1 | cut -d= -f2- | tr -d '"'"'" || true)"
  port="${addr##*:}"; [[ "$port" =~ ^[0-9]+$ ]] || port=7710
  if grep -qE '^SIGARDAPANEL_AGENT_TLS_CERT=.+' "$ENV_FILE"; then scheme=https; fi
  echo "${scheme}://127.0.0.1:${port}"
}

# healthy_on <version>: the agent answers /health with that version (30 s).
healthy_on() {
  local want="$1" url body
  url="$(agent_url)/health"
  for _ in $(seq 1 30); do
    body="$(curl -fsSk --max-time 3 "$url" 2>/dev/null || true)"
    if printf '%s' "$body" | grep -q "\"version\":\"${want}\""; then return 0; fi
    sleep 1
  done
  return 1
}

current="$("$INSTALL_BIN" version --client 2>/dev/null | head -n1 || echo unknown)"
if [ "${current#v}" = "${TAG#v}" ]; then ok "Already on ${TAG}"; exit 0; fi
log "Updating agent ${current} -> ${TAG} (linux-${ARCH})"

TMP="$(mktemp -d)"
base="${SIGARDA_ASSET_BASE:-https://github.com/${REPO}/releases/download/${TAG}}"
for f in "sigardapanel-linux-${ARCH}" SHA256SUMS SHA256SUMS.minisig; do
  curl -fsSL --proto '=https,file' --max-time 600 --retry 3 -o "${TMP}/${f}" "${base}/${f}" || die "download failed: ${base}/${f}"
done
log "Verifying signature and checksum..."
minisign -Vm "${TMP}/SHA256SUMS" -P "$PUBKEY" -x "${TMP}/SHA256SUMS.minisig" >/dev/null \
  || die "SIGNATURE VERIFICATION FAILED - refusing to install"
( cd "$TMP" && grep -E " sigardapanel-linux-${ARCH}\$" SHA256SUMS | sha256sum -c --quiet - ) || die "CHECKSUM MISMATCH - refusing to install"
[ "$(grep -cE " sigardapanel-linux-${ARCH}\$" "${TMP}/SHA256SUMS")" -eq 1 ] || die "SHA256SUMS does not list sigardapanel-linux-${ARCH} exactly once"
chmod 755 "${TMP}/sigardapanel-linux-${ARCH}"
newver="$("${TMP}/sigardapanel-linux-${ARCH}" version --client 2>/dev/null | head -n1 || true)"
[ "${newver#v}" = "${TAG#v}" ] || die "the downloaded binary reports version '${newver}', expected ${TAG#v}"
ok "Release ${TAG} verified"

ts="$(date -u +%Y%m%dT%H%M%SZ)"; bk="${BACKUP_DIR}/${ts}"
install -d -m 700 "$bk"
cp -a "$INSTALL_BIN" "$bk/sigardapanel.bin"
echo "$current" > "$bk/VERSION"
ok "Backup saved to ${bk}"
# Keep the newest backups only.
find "$BACKUP_DIR" -mindepth 1 -maxdepth 1 -type d -name '20*T*Z' -printf '%f\n' | sort -r | tail -n +$((KEEP_BACKUPS + 1)) \
  | while read -r old; do rm -rf "${BACKUP_DIR:?}/${old}"; done

restore() {
  warn "Update failed - restoring ${current}"
  { install -m 755 "$bk/sigardapanel.bin" "${INSTALL_BIN}.rb" && mv -f "${INSTALL_BIN}.rb" "$INSTALL_BIN"; } || warn "could not restore the binary"
  systemctl restart "$SERVICE" || true
  if healthy_on "${current#v}"; then
    die "Agent update to ${TAG} failed; restored ${current} and the agent is healthy again"
  fi
  die "Agent update to ${TAG} failed and the restored agent is NOT healthy. Backup: ${bk}. Check: journalctl -u ${SERVICE}"
}

{ install -m 755 "${TMP}/sigardapanel-linux-${ARCH}" "${INSTALL_BIN}.new" && mv -f "${INSTALL_BIN}.new" "$INSTALL_BIN"; } || restore
systemctl restart "$SERVICE" || restore
healthy_on "${TAG#v}" || restore
ok "Agent updated to ${TAG}"
