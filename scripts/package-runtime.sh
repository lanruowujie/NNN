#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)"
# shellcheck source=toolchain/platform.sh
source "$SCRIPT_DIR/toolchain/platform.sh"

PLATFORM="$(nfcx_platform)"
EXE_SUFFIX="$(nfcx_exe_suffix "$PLATFORM")"
SOURCE="$REPO_ROOT/runtime/$PLATFORM"
case "$PLATFORM" in
  darwin-*) DEFAULT_APP="$REPO_ROOT/build/bin/NFCX.app" ;;
  windows-*) DEFAULT_APP="$REPO_ROOT/build/bin/NFCX.exe" ;;
  linux-*) DEFAULT_APP="$REPO_ROOT/build/bin/NFCX" ;;
esac
APP_PATH="${1:-$DEFAULT_APP}"

case "$APP_PATH" in
  "$REPO_ROOT/build/bin/"*) ;;
  *) echo "refusing to modify an artifact outside build/bin: $APP_PATH" >&2; exit 1 ;;
esac

for engine in mfoc mfcuk mfoc-hardnested nfc-mfsetuid; do
  [[ -x "$SOURCE/$engine$EXE_SUFFIX" ]] || { echo "$engine runtime is missing for $PLATFORM" >&2; exit 1; }
done

copy_metadata() {
  local destination="$1" metadata patch license
  mkdir -p "$destination/LICENSES"
  for metadata in source.lock mfoc-source.lock mfcuk-source.lock mfoc-hardnested-source.lock; do
    [[ -f "$SOURCE/$metadata" ]] && install -m 0644 "$SOURCE/$metadata" "$destination/$metadata"
  done
  if [[ -d "$SOURCE/patches" ]]; then
    mkdir -p "$destination/patches"
    for patch in "$SOURCE"/patches/*; do
      [[ -f "$patch" ]] && install -m 0644 "$patch" "$destination/patches/$(basename "$patch")"
    done
  fi
  for license in "$SOURCE"/LICENSES/*; do
    [[ -f "$license" ]] && install -m 0644 "$license" "$destination/LICENSES/$(basename "$license")"
  done
}

copy_windows_dependencies() {
  local destination="$1" index=0 binary imported normalized candidate package license_path
  local queue=("$destination"/*.exe "$destination"/*.dll)
  while (( index < ${#queue[@]} )); do
    binary="${queue[$index]}"
    index=$((index + 1))
    [[ -f "$binary" ]] || continue
    while IFS= read -r imported; do
      imported="${imported//$'\r'/}"
      normalized="$(printf '%s' "$imported" | tr '[:upper:]' '[:lower:]')"
      case "$normalized" in
        api-ms-*|ext-ms-*|advapi32.dll|bcrypt.dll|cfgmgr32.dll|comctl32.dll|comdlg32.dll|crypt32.dll|d2d1.dll|d3d11.dll|dcomp.dll|dwmapi.dll|dxgi.dll|gdi32.dll|imm32.dll|iphlpapi.dll|kernel32.dll|mpr.dll|msimg32.dll|msvcrt.dll|ncrypt.dll|netapi32.dll|ntdll.dll|ole32.dll|oleaut32.dll|powrprof.dll|propsys.dll|rpcrt4.dll|secur32.dll|setupapi.dll|shell32.dll|shlwapi.dll|user32.dll|userenv.dll|uxtheme.dll|version.dll|winhttp.dll|winmm.dll|winspool.drv|ws2_32.dll|wtsapi32.dll) continue ;;
      esac
      if [[ -d /c/Windows/System32 ]] && find /c/Windows/System32 -maxdepth 1 -type f -iname "$imported" -print -quit | grep -q .; then
        continue
      fi
      if find "$destination" -maxdepth 1 -type f -iname "$imported" -print -quit | grep -q .; then
        continue
      fi
      candidate=""
      for search_root in "${MINGW_PREFIX:-/mingw64}/bin" "$SOURCE"; do
        candidate="$(find "$search_root" -maxdepth 1 -type f -iname "$imported" -print -quit 2>/dev/null || true)"
        [[ -n "$candidate" ]] && break
      done
      [[ -n "$candidate" ]] || { echo "unresolved private Windows DLL $imported imported by $(basename "$binary")" >&2; return 1; }
      install -m 0755 "$candidate" "$destination/$(basename "$candidate")"
      queue+=("$destination/$(basename "$candidate")")
      if command -v pacman >/dev/null 2>&1; then
        package="$(pacman -Qoq "$candidate" 2>/dev/null | head -n 1 || true)"
        if [[ -n "$package" ]]; then
          while IFS= read -r license_path; do
            [[ -f "$license_path" ]] && install -m 0644 "$license_path" "$destination/LICENSES/${package}-$(basename "$license_path")"
          done < <(pacman -Ql "$package" | awk '$2 ~ /\/share\/licenses\// { if (substr($2, 1, 1) == "/") print $2; else print "/" $2 }')
        fi
      fi
    done < <(objdump -p "$binary" | sed -n 's/^[[:space:]]*DLL Name:[[:space:]]*//p')
  done
}

case "$PLATFORM" in
  darwin-arm64)
    [[ -d "$APP_PATH/Contents/MacOS" ]] || { echo "application bundle is missing: $APP_PATH" >&2; exit 1; }
    DESTINATION="$APP_PATH/Contents/Resources/runtime/$PLATFORM"
    APP_EXECUTABLE="$APP_PATH/Contents/MacOS/NFCX"
    rm -rf "$DESTINATION"
    mkdir -p "$DESTINATION"
    for library in "$SOURCE"/libnfc.*.dylib; do
      [[ -f "$library" ]] && install -m 0755 "$library" "$DESTINATION/$(basename "$library")"
    done
    VERSIONED_LIBRARY="$(find "$DESTINATION" -maxdepth 1 -type f -name 'libnfc.*.dylib' -print | head -n 1)"
    [[ -n "$VERSIONED_LIBRARY" ]] || { echo "versioned libnfc dylib is missing" >&2; exit 1; }
    VERSIONED_NAME="$(basename "$VERSIONED_LIBRARY")"
    ln -sf "$VERSIONED_NAME" "$DESTINATION/libnfc.dylib"
    for engine in mfoc mfcuk mfoc-hardnested nfc-mfsetuid; do install -m 0755 "$SOURCE/$engine" "$DESTINATION/$engine"; done
    copy_metadata "$DESTINATION"
    APP_RPATH="@executable_path/../Resources/runtime/$PLATFORM"
    otool -l "$APP_EXECUTABLE" | grep -A3 LC_RPATH | grep -Fq "$APP_RPATH" || install_name_tool -add_rpath "$APP_RPATH" "$APP_EXECUTABLE"
    ;;
  linux-amd64)
    [[ -x "$APP_PATH" ]] || { echo "application executable is missing: $APP_PATH" >&2; exit 1; }
    APP_ROOT="$(dirname "$APP_PATH")/NFCX-linux-amd64"
    DESTINATION="$APP_ROOT/runtime/$PLATFORM"
    rm -rf "$APP_ROOT"
    mkdir -p "$DESTINATION"
    install -m 0755 "$APP_PATH" "$APP_ROOT/NFCX"
    for library in "$SOURCE"/libnfc.so*; do
      [[ -e "$library" ]] && cp -P "$library" "$DESTINATION/"
    done
    for engine in mfoc mfcuk mfoc-hardnested nfc-mfsetuid; do install -m 0755 "$SOURCE/$engine" "$DESTINATION/$engine"; done
    copy_metadata "$DESTINATION"
    patchelf --set-rpath '$ORIGIN/runtime/linux-amd64' "$APP_ROOT/NFCX"
    ;;
  windows-amd64)
    [[ -f "$APP_PATH" ]] || { echo "application executable is missing: $APP_PATH" >&2; exit 1; }
    APP_ROOT="$(dirname "$APP_PATH")/NFCX-windows-amd64"
    DESTINATION="$APP_ROOT"
    rm -rf "$APP_ROOT"
    mkdir -p "$DESTINATION"
    install -m 0755 "$APP_PATH" "$DESTINATION/NFCX.exe"
    for library in "$(dirname "$APP_PATH")"/*.dll; do
      [[ -f "$library" ]] && install -m 0755 "$library" "$DESTINATION/$(basename "$library")"
    done
    for library in "$SOURCE"/*.dll; do
      [[ -f "$library" ]] && install -m 0755 "$library" "$DESTINATION/$(basename "$library")"
    done
    for engine in mfoc mfcuk mfoc-hardnested nfc-mfsetuid; do install -m 0755 "$SOURCE/$engine.exe" "$DESTINATION/$engine.exe"; done
    copy_metadata "$DESTINATION"
    copy_windows_dependencies "$DESTINATION"
    ;;
  *) echo "runtime packaging is not implemented for $PLATFORM" >&2; exit 1 ;;
esac

echo "packaged unsigned $PLATFORM runtime at $DESTINATION"
