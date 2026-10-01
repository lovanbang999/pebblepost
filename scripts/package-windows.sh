#!/bin/bash
set -e

# ==============================================================================
# PebblePost Studio - Windows Packaging Script (.exe / .zip)
# ==============================================================================

VERSION="0.1.0"
ARCH="amd64"
APP_NAME="pebblepost"
BUILD_DIR="build/bin"
ZIP_NAME="${APP_NAME}_${VERSION}_windows_${ARCH}.zip"

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_ROOT"

echo "======================================================================"
echo "  PebblePost Studio - Windows Packaging Pipeline (v${VERSION})"
echo "======================================================================"

mkdir -p "$BUILD_DIR"

if [ -f "$BUILD_DIR/pebblepost.exe" ]; then
  echo "Compressing Windows executable to $ZIP_NAME..."
  (cd "$BUILD_DIR" && zip -q -9 "$ZIP_NAME" pebblepost.exe)
  echo "Windows zip package created at: $BUILD_DIR/$ZIP_NAME"
else
  echo "Notice: $BUILD_DIR/pebblepost.exe not found. Run 'wails build -platform windows/amd64' first."
fi
