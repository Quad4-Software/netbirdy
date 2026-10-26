#!/bin/sh
# Builds a thin AppImage for netbirdy-ui (amd64). Bundles no GTK or WebKit;
# the host provides them like the deb and rpm packages do.
set -eu

BIN="$1"; VER="$2"; OUT="$3"
RT_URL="https://github.com/AppImage/type2-runtime/releases/download/20251108/runtime-x86_64"
RT_SHA="2fca8b443c92510f1483a883f60061ad09b46b978b2631c807cd873a47ec260d"
AT_URL="https://github.com/AppImage/appimagetool/releases/download/1.9.1/appimagetool-x86_64.AppImage"
AT_SHA="ed4ce84f0d9caff66f50bcca6ff6f35aae54ce8135408b3fa33abfc3cb384eb0"

TOOLS="${APPIMAGE_TOOLS_DIR:-$(mktemp -d)}"
mkdir -p "$TOOLS" "$OUT"
fetch() {
  if [ ! -f "$2" ]; then
    curl --fail --location --proto '=https' --tlsv1.2 -o "$2.part" "$1"
    mv "$2.part" "$2"
  fi
  echo "$3  $2" | sha256sum -c - >/dev/null || { rm -f "$2"; echo "hash mismatch for $1" >&2; exit 1; }
}
fetch "$RT_URL" "$TOOLS/runtime-x86_64" "$RT_SHA"
fetch "$AT_URL" "$TOOLS/appimagetool" "$AT_SHA"
chmod 755 "$TOOLS/appimagetool" "$TOOLS/runtime-x86_64"

D=$(mktemp -d)/netbirdy-ui.AppDir
mkdir -p "$D/usr/bin" "$D/usr/share/applications" "$D/usr/share/icons/hicolor/256x256/apps"
cp "$BIN" "$D/usr/bin/netbirdy-ui"; chmod 755 "$D/usr/bin/netbirdy-ui"
cp client/ui/build/appicon.png "$D/netbirdy.png"
cp "$D/netbirdy.png" "$D/.DirIcon"
cp "$D/netbirdy.png" "$D/usr/share/icons/hicolor/256x256/apps/netbirdy.png"

cat > "$D/netbirdy.desktop" <<'EOD'
[Desktop Entry]
Name=NetBirdy
Comment=NetBirdy desktop client
Exec=netbirdy-ui
Icon=netbirdy
Type=Application
Terminal=false
Categories=Utility;
Keywords=netbirdy;
StartupWMClass=org.wails.netbirdy
EOD
cp "$D/netbirdy.desktop" "$D/usr/share/applications/netbirdy.desktop"
command -v desktop-file-validate >/dev/null 2>&1 && desktop-file-validate "$D/netbirdy.desktop" || true

cat > "$D/AppRun" <<'EOD'
#!/bin/sh
set -eu
HERE="$(cd "$(dirname "$0")" && pwd)"
export WEBKIT_DISABLE_DMABUF_RENDERER="${WEBKIT_DISABLE_DMABUF_RENDERER:-1}"
LIBS="$(ldd "$HERE/usr/bin/netbirdy-ui" 2>/dev/null || true)"
for lib in libgtk-4.so.1 libwebkitgtk-6.0.so.4; do
  if ! printf '%s\n' "$LIBS" | grep -q "^[[:space:]]*$lib => /"; then
    echo "netbirdy-ui requires GTK4 (Debian/Ubuntu: libgtk-4-1, Fedora: gtk4) and WebKitGTK 6.0 (libwebkitgtk-6.0-4 / webkitgtk6.0). The netbirdy daemon package must also be installed and running." >&2
    exit 1
  fi
done
exec "$HERE/usr/bin/netbirdy-ui" "$@"
EOD
chmod 755 "$D/AppRun"

APPIMAGE_EXTRACT_AND_RUN=1 ARCH=x86_64 "$TOOLS/appimagetool" \
  --runtime-file "$TOOLS/runtime-x86_64" "$D" \
  "$OUT/netbirdy-ui_${VER}_linux_amd64.AppImage"
chmod 755 "$OUT/netbirdy-ui_${VER}_linux_amd64.AppImage"
