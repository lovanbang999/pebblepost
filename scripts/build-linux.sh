#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_ROOT"

required=(cargo rustc node npm pkg-config dpkg-deb)
missing=()
for tool in "${required[@]}"; do
  command -v "$tool" >/dev/null 2>&1 || missing+=("$tool")
done
if [ "${#missing[@]}" -gt 0 ]; then
  printf 'Missing build tools: %s\n' "${missing[*]}" >&2
  exit 1
fi

if ! cargo tauri --version >/dev/null 2>&1; then
  echo "Installing Tauri CLI 2..."
  cargo install tauri-cli --version '^2' --locked
fi

if ! pkg-config --exists webkit2gtk-4.1 gtk+-3.0; then
  cat >&2 <<'EOF'
Tauri Linux development libraries are missing. Install the GTK and WebKitGTK
development packages for your distribution (including webkit2gtk-4.1, gtk-3,
build-essential, curl, wget, file, and librsvg2).
EOF
  exit 1
fi

bash "$PROJECT_ROOT/scripts/package-deb.sh" --build
