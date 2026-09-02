#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BUILD_DIR="/tmp/sigardapanel-build-$$"
OUTPUT="${1:-$SCRIPT_DIR/sigardapanel}"

echo "=== SigardaPanel Enterprise Builder ==="

mkdir -p "$BUILD_DIR"
trap "rm -rf $BUILD_DIR" EXIT

echo "[1/3] Copying enterprise source..."
cp -r "$SCRIPT_DIR"/internal "$BUILD_DIR/"
cp -r "$SCRIPT_DIR"/cmd "$BUILD_DIR/"
cp -r "$SCRIPT_DIR"/deploy "$BUILD_DIR/" 2>/dev/null || true
cp -r "$SCRIPT_DIR"/docs "$BUILD_DIR/" 2>/dev/null || true
cp "$SCRIPT_DIR"/go.mod "$BUILD_DIR/"
cp "$SCRIPT_DIR"/go.sum "$BUILD_DIR/"
cp "$SCRIPT_DIR"/LICENSE "$BUILD_DIR/" 2>/dev/null || true
cp "$SCRIPT_DIR"/README.md "$BUILD_DIR/" 2>/dev/null || true

echo "[2/3] Building..."
cd "$BUILD_DIR"
go build -buildvcs=false -o sigardapanel ./cmd/sigardapanel

echo "[3/3] Done!"
cp sigardapanel "$OUTPUT" 2>/dev/null || mv sigardapanel "$OUTPUT"
chmod +x "$OUTPUT"
echo "Binary: $OUTPUT ($(du -h "$OUTPUT" | cut -f1))"
