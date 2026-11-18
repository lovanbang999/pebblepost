#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_ROOT"

TAURI_DIR="$PROJECT_ROOT/apps/pebblepost-desktop/src-tauri"
BUNDLE_DIR="$TAURI_DIR/target/release/bundle/deb"
DEST_DIR="$PROJECT_ROOT/build/bin"

if [ "${1:-}" = "--build" ] || ! find "$BUNDLE_DIR" -maxdepth 1 -name '*.deb' -print -quit 2>/dev/null | grep -q .; then
  cargo tauri build --manifest-path "$TAURI_DIR/Cargo.toml" --bundles deb
fi

DEB_FILE="$(find "$BUNDLE_DIR" -maxdepth 1 -name '*.deb' -print -quit)"
if [ -z "$DEB_FILE" ]; then
  echo "Tauri did not produce a Debian package under $BUNDLE_DIR" >&2
  exit 1
fi

mkdir -p "$DEST_DIR"
cp -f "$DEB_FILE" "$DEST_DIR/"
echo "Debian package: $DEST_DIR/$(basename "$DEB_FILE")"
