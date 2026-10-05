#!/usr/bin/env bash
set -e

# ==============================================================================
# PebblePost Studio - One-Click Linux Build & Package Script
# Builds the native desktop binary and generates an installable Debian/Ubuntu .deb
# ==============================================================================

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_ROOT"

echo "======================================================================"
echo "  🚀 PebblePost Studio - Linux One-Click Build & Package Pipeline"
echo "======================================================================"

# 1. Environment & PATH setup
if [ -d "$HOME/go/bin" ]; then
  export PATH="$PATH:$HOME/go/bin"
fi
if command -v go >/dev/null 2>&1; then
  GOPATH_BIN="$(go env GOPATH 2>/dev/null)/bin"
  if [ -n "$GOPATH_BIN" ] && [ -d "$GOPATH_BIN" ]; then
    export PATH="$PATH:$GOPATH_BIN"
  fi
fi
export PATH="$PATH:/usr/local/go/bin"

# 2. Check essential build tools
MISSING_TOOLS=()
if ! command -v go >/dev/null 2>&1; then
  MISSING_TOOLS+=("go (https://go.dev/dl/)")
fi
if ! command -v node >/dev/null 2>&1; then
  MISSING_TOOLS+=("nodejs (https://nodejs.org/)")
fi
if ! command -v npm >/dev/null 2>&1; then
  MISSING_TOOLS+=("npm")
fi
if ! command -v dpkg-deb >/dev/null 2>&1; then
  MISSING_TOOLS+=("dpkg-deb (sudo apt install dpkg)")
fi

if [ ${#MISSING_TOOLS[@]} -gt 0 ]; then
  echo "❌ Error: Missing required build tool(s):"
  for t in "${MISSING_TOOLS[@]}"; do
    echo "   - $t"
  done
  exit 1
fi

# 3. Check / Install Wails CLI v2
if ! command -v wails >/dev/null 2>&1; then
  echo "⚠️ Wails CLI v2 not found in PATH. Installing Wails CLI..."
  go install github.com/wailsapp/wails/v2/cmd/wails@latest
  export PATH="$PATH:$HOME/go/bin"
fi

if ! command -v wails >/dev/null 2>&1; then
  echo "❌ Error: Could not locate 'wails' CLI after installation. Please ensure ~/go/bin is in your PATH."
  exit 1
fi

# 4. Check Linux system libraries (WebKit2GTK and GTK3)
if ! pkg-config --exists webkit2gtk-4.1 gtk+-3.0 2>/dev/null; then
  echo "⚠️ Warning: GTK3 or WebKit2GTK-4.1 development libraries may be missing."
  echo "If the build fails, install them with:"
  echo "  sudo apt update && sudo apt install -y build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev"
  echo ""
fi

# 5. Build frontend & package .deb installer
echo "📦 Building and packaging PebblePost for Linux..."
bash "$PROJECT_ROOT/scripts/package-deb.sh" --build

echo ""
echo "======================================================================"
echo "  🎉 Build Successful!"
echo "======================================================================"
echo ""
echo "You have two ways to run PebblePost on your system:"
echo ""
echo "1️⃣  Install system-wide (.deb package with Desktop App launcher):"
echo "    sudo apt install ./build/bin/pebblepost_0.1.0_amd64.deb"
echo "    (or: sudo dpkg -i ./build/bin/pebblepost_0.1.0_amd64.deb)"
echo ""
echo "2️⃣  Run directly as standalone portable binary (no install needed):"
echo "    ./build/bin/pebblepost"
echo ""
echo "======================================================================"
