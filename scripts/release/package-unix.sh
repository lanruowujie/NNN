#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)"
# shellcheck source=../toolchain/platform.sh
source "$REPO_ROOT/scripts/toolchain/platform.sh"
PLATFORM="$(nfcx_platform)"
VERSION="${1:?NFCX version is required}"
DIST="$REPO_ROOT/dist"
mkdir -p "$DIST"

case "$PLATFORM" in
  darwin-arm64)
    APP="$REPO_ROOT/build/bin/NFCX.app"
    UNSIGNED_SUFFIX=""
    [[ "${NFCX_UNSIGNED:-0}" == 1 ]] && UNSIGNED_SUFFIX="-unsigned"
    DMG="$DIST/NFCX-$VERSION-darwin-arm64$UNSIGNED_SUFFIX.dmg"
    DMG_STAGE="$REPO_ROOT/build/dmg-stage/NFCX-$VERSION"
    [[ -d "$APP" ]] || { echo "application bundle is missing: $APP" >&2; exit 1; }
    rm -rf "$DMG_STAGE"
    mkdir -p "$DMG_STAGE"
    cp -R "$APP" "$DMG_STAGE/NFCX.app"
    ln -s /Applications "$DMG_STAGE/Applications"
    cat > "$DMG_STAGE/Install NFCX.txt" <<'EOF'
To install NFCX, drag NFCX.app to the Applications folder in this window.

安装 NFCX：请将 NFCX.app 拖动到本窗口中的 Applications 目录。
EOF
    hdiutil create -volname "NFCX $VERSION" -srcfolder "$DMG_STAGE" -ov -format UDZO "$DMG"
    rm -rf "$DMG_STAGE"
    if [[ -n "${NFCX_CODESIGN_IDENTITY:-}" ]]; then
      codesign --force --sign "$NFCX_CODESIGN_IDENTITY" --timestamp "$DMG"
    fi
    ;;
  linux-amd64)
    source "$REPO_ROOT/third_party/appimage/appimage.lock"
    APP_ROOT="$REPO_ROOT/build/bin/NFCX-linux-amd64"
    APPDIR="$REPO_ROOT/build/bin/NFCX.AppDir"
    TOOL_CACHE="$REPO_ROOT/build/toolchain/appimage"
    TOOL="$TOOL_CACHE/appimagetool-x86_64.AppImage"
	go build -o "$APP_ROOT/NFCX Updater" ./cmd/nfcx-updater
    RUNTIME="$TOOL_CACHE/runtime-x86_64"
    rm -rf "$APPDIR"
    mkdir -p "$APPDIR/usr/bin" "$TOOL_CACHE"
    cp -R "$APP_ROOT"/. "$APPDIR/usr/bin/"
    install -m 0755 "$REPO_ROOT/build/linux/AppRun" "$APPDIR/AppRun"
    install -m 0644 "$REPO_ROOT/build/linux/NFCX.desktop" "$APPDIR/NFCX.desktop"
    install -m 0644 "$REPO_ROOT/build/appicon.png" "$APPDIR/NFCX.png"
    if [[ ! -f "$TOOL" ]]; then nfcx_download_file "$APPIMAGETOOL_URL" "$TOOL"; fi
    if [[ ! -f "$RUNTIME" ]]; then nfcx_download_file "$APPIMAGE_RUNTIME_URL" "$RUNTIME"; fi
    echo "$APPIMAGETOOL_SHA256  $TOOL" | sha256sum -c -
    echo "$APPIMAGE_RUNTIME_SHA256  $RUNTIME" | sha256sum -c -
    chmod 0755 "$TOOL" "$RUNTIME"
    ARCH=x86_64 VERSION="$VERSION" APPIMAGE_EXTRACT_AND_RUN=1 "$TOOL" --runtime-file "$RUNTIME" "$APPDIR" "$DIST/NFCX-$VERSION-linux-amd64.AppImage"
    chmod 0755 "$DIST/NFCX-$VERSION-linux-amd64.AppImage"
    tar -C "$REPO_ROOT/build/bin" -czf "$DIST/NFCX-$VERSION-linux-amd64.tar.gz" NFCX-linux-amd64
    ;;
  *) echo "package-unix does not support $PLATFORM" >&2; exit 1 ;;
esac
