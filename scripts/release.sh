#!/usr/bin/env bash
# Build, checksum and sign a SigardaPanel release.
#
#   scripts/release.sh 1.2.0                 build into $OUT/v1.2.0 (default OUT=/root/release-dist)
#   scripts/release.sh 1.2.0 --publish       ...and create the GitHub release on the public repo
#
# Needs: go, node/npm, minisign, and the signing key (MINISIGN_KEY, default
# /root/release-signing/sigardapanel.key). MINISIGN_PASSWORD, when set, is
# fed to minisign for an encrypted key. The same script runs in the Release
# workflow (.github/workflows/release.yml) with the key from a secret.
# Release notes: RELEASE_NOTES, else docs/releases/<tag>.md, else a default.
# The public installer (installer/install.sh) verifies SHA256SUMS against the
# matching public key, so a release without a valid signature cannot be installed.
set -euo pipefail

VERSION="${1:?usage: release.sh <version> [--publish]}"
VERSION="${VERSION#v}"
[[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "version must look like 1.2.3" >&2; exit 1; }
TAG="v${VERSION}"
PUBLISH="${2:-}"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="${OUT:-/root/release-dist}/${TAG}"
KEY="${MINISIGN_KEY:-/root/release-signing/sigardapanel.key}"
PUBLIC_REPO="${PUBLIC_REPO:-bayurstarcool/SigardaPanel}"

[ -f "$KEY" ] || { echo "signing key not found: $KEY" >&2; exit 1; }
cd "$ROOT"
if [ -n "$(git status --porcelain)" ]; then echo "working tree is dirty - commit first" >&2; exit 1; fi

echo "==> tests"
go vet ./internal/... ./cmd/...
go test ./internal/... 2>&1 | grep -v 'no test files' | grep -v '^ok' && { echo "tests failed" >&2; exit 1; } || true

echo "==> web build"
( cd web && npm ci --silent && npm run build >/dev/null )

rm -rf "$OUT"; mkdir -p "$OUT"
echo "==> binaries"
for arch in amd64 arm64; do
  CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -buildvcs=false -trimpath \
    -ldflags "-s -w -X sigardapanel/internal/buildinfo.Version=${VERSION}" \
    -o "$OUT/sigardapanel-linux-${arch}" ./cmd/sigardapanel
done
tar -czf "$OUT/sigardapanel-web.tar.gz" -C web/build .

echo "==> checksums + signature"
( cd "$OUT" && sha256sum sigardapanel-linux-amd64 sigardapanel-linux-arm64 sigardapanel-web.tar.gz > SHA256SUMS )
if [ -n "${MINISIGN_PASSWORD:-}" ]; then
  printf '%s\n' "$MINISIGN_PASSWORD" | minisign -S -s "$KEY" -m "$OUT/SHA256SUMS" -x "$OUT/SHA256SUMS.minisig" -t "SigardaPanel ${TAG}"
else
  minisign -S -s "$KEY" -m "$OUT/SHA256SUMS" -x "$OUT/SHA256SUMS.minisig" -t "SigardaPanel ${TAG}"
fi
# Verify against the public key installers pin: a release signed with any
# other key would be uninstallable, so never publish one.
PUBKEY="$(sed -n 's/^PUBKEY="\(.*\)"$/\1/p' installer/install.sh)"
[ -n "$PUBKEY" ] || { echo "public key not found in installer/install.sh" >&2; exit 1; }
minisign -V -P "$PUBKEY" -m "$OUT/SHA256SUMS" -x "$OUT/SHA256SUMS.minisig" >/dev/null \
  || { echo "signature does not verify with the installer's public key" >&2; exit 1; }
ls -la "$OUT"

if [ "$PUBLISH" = "--publish" ]; then
  echo "==> publishing ${TAG} to ${PUBLIC_REPO}"
  NOTES="${RELEASE_NOTES:-}"
  if [ -z "$NOTES" ] && [ -f "docs/releases/${TAG}.md" ]; then NOTES="$(cat "docs/releases/${TAG}.md")"; fi
  NOTES="${NOTES:-SigardaPanel ${TAG}. Install with the script in the repository README; every file is signed (see 'Verifying releases').}"
  # RELEASE_TARGET (a commit): create the tag there when it does not exist yet
  # (manual workflow runs); otherwise the existing tag is used.
  gh release create "$TAG" -R "$PUBLIC_REPO" ${RELEASE_TARGET:+--target "$RELEASE_TARGET"} \
    --title "SigardaPanel ${TAG}" --notes "$NOTES" --latest "$OUT"/*
fi
