#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_ROOT"

TAURI_DIR="$PROJECT_ROOT/apps/pebblepost-desktop/src-tauri"
BUNDLE_DIR="$TAURI_DIR/target/release/bundle/nsis"
DEST_DIR="$PROJECT_ROOT/build/bin"

cargo tauri build --manifest-path "$TAURI_DIR/Cargo.toml" --bundles nsis
mkdir -p "$DEST_DIR"
find "$BUNDLE_DIR" -maxdepth 1 -name '*.exe' -exec cp -f {} "$DEST_DIR/" \;

if ! find "$DEST_DIR" -maxdepth 1 -name '*.exe' -print -quit | grep -q .; then
  echo "Tauri did not produce a Windows installer under $BUNDLE_DIR" >&2
  exit 1
fi
echo "Windows installer copied to $DEST_DIR"
