#!/bin/bash
set -e

# ==============================================================================
# PebblePost Studio - Linux .deb Packaging Script
# Packages build/bin/pebblepost into a 1-click installable Debian/Ubuntu .deb
# ==============================================================================

VERSION="0.1.0"
ARCH="amd64"
APP_NAME="pebblepost"
PKG_DIR="build/deb_pkg"
DEB_NAME="${APP_NAME}_${VERSION}_${ARCH}.deb"

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_ROOT"

echo "======================================================================"
echo "  PebblePost Studio - Debian (.deb) Packaging Pipeline (v${VERSION})"
echo "======================================================================"

REBUILD=false
if [ "$1" = "--build" ] || [ ! -f "build/bin/pebblepost" ]; then
  REBUILD=true
elif [ -f "build/bin/pebblepost" ]; then
  NEWER=$(find web/src cmd internal -newer "build/bin/pebblepost" 2>/dev/null | head -n 1)
  if [ -n "$NEWER" ]; then
    echo "Detected source modifications newer than build/bin/pebblepost ($NEWER)."
    REBUILD=true
  fi
fi

export PATH="$PATH:$HOME/go/bin:/usr/local/go/bin"

if [ "$REBUILD" = true ]; then
  echo "Building frontend and desktop binary with Wails..."
  if command -v npm &> /dev/null; then
    (cd web && npm run build)
  elif command -v yarn &> /dev/null; then
    (cd web && yarn build)
  elif command -v pnpm &> /dev/null; then
    (cd web && pnpm build)
  elif command -v bun &> /dev/null; then
    (cd web && bun run build)
  else
    echo "Error: No Node.js package manager found."
    exit 1
  fi
  wails build -s -skipbindings -clean -ldflags "-s -w"
fi

if [ ! -f "build/bin/pebblepost" ]; then
  echo "Error: build/bin/pebblepost not found. Build failed."
  exit 1
fi

echo "Packaging PebblePost Studio v${VERSION} (.deb)..."

rm -rf "$PKG_DIR"
mkdir -p "$PKG_DIR/DEBIAN"
mkdir -p "$PKG_DIR/usr/bin"
mkdir -p "$PKG_DIR/usr/share/applications"
mkdir -p "$PKG_DIR/usr/share/pixmaps"
mkdir -p "$PKG_DIR/usr/share/icons/hicolor/scalable/apps"

# 1. Install binary
cp -f "build/bin/pebblepost" "$PKG_DIR/usr/bin/pebblepost"
chmod 755 "$PKG_DIR/usr/bin/pebblepost"

# 2. Install application icon
if [ -f "web/public/favicon.svg" ]; then
  cp -f "web/public/favicon.svg" "$PKG_DIR/usr/share/icons/hicolor/scalable/apps/pebblepost.svg"
  cp -f "web/public/favicon.svg" "$PKG_DIR/usr/share/pixmaps/pebblepost.svg"
fi

# 3. Install desktop launcher
cat <<EOF > "$PKG_DIR/usr/share/applications/pebblepost.desktop"
[Desktop Entry]
Name=PebblePost Studio
Comment=Modern lightweight Git-friendly API Client
Exec=/usr/bin/pebblepost
Icon=pebblepost
Terminal=false
Type=Application
Categories=Development;Network;IDE;
StartupWMClass=pebblepost
EOF
chmod 644 "$PKG_DIR/usr/share/applications/pebblepost.desktop"

# 3. Control file
cat <<EOF > "$PKG_DIR/DEBIAN/control"
Package: ${APP_NAME}
Version: ${VERSION}
Section: devel
Priority: optional
Architecture: ${ARCH}
Maintainer: PebblePost Team <lovanbangbox9@gmail.com>
Depends: libgtk-3-0, libwebkit2gtk-4.1-0
Description: PebblePost Studio - Modern lightweight Git-friendly API Client
 Native desktop and standalone web API client for REST, GraphQL, and microservices.
 Single binary with embedded Goja JavaScript runtime.
EOF
chmod 644 "$PKG_DIR/DEBIAN/control"

# 4. Maintainer scripts for icon cache & desktop database refresh
cat << 'EOF' > "$PKG_DIR/DEBIAN/postinst"
#!/bin/sh
set -e
if which gtk-update-icon-cache >/dev/null 2>&1; then
    gtk-update-icon-cache -q -t -f /usr/share/icons/hicolor 2>/dev/null || true
fi
if which update-desktop-database >/dev/null 2>&1; then
    update-desktop-database -q 2>/dev/null || true
fi
EOF
chmod 755 "$PKG_DIR/DEBIAN/postinst"

cat << 'EOF' > "$PKG_DIR/DEBIAN/postrm"
#!/bin/sh
set -e
if which gtk-update-icon-cache >/dev/null 2>&1; then
    gtk-update-icon-cache -q -t -f /usr/share/icons/hicolor 2>/dev/null || true
fi
if which update-desktop-database >/dev/null 2>&1; then
    update-desktop-database -q 2>/dev/null || true
fi
EOF
chmod 755 "$PKG_DIR/DEBIAN/postrm"

# 5. Build .deb package with dpkg-deb
dpkg-deb --build --root-owner-group "$PKG_DIR" "build/bin/${DEB_NAME}"

# Clean up staging dir
rm -rf "$PKG_DIR"

echo ""
echo "======================================================================"
echo "Debian packaging complete! Output file located in build/bin/:"
ls -lh "build/bin/${DEB_NAME}"
echo "======================================================================"
