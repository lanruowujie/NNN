#!/usr/bin/env bash

set -euo pipefail

export LC_ALL=C
export LANG=C

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)"
LOCK_FILE="$REPO_ROOT/third_party/mfoc-hardnested/hardnested.lock"
# shellcheck source=platform.sh
source "$SCRIPT_DIR/platform.sh"
# shellcheck source=/dev/null
source "$LOCK_FILE"

for lock_name in HARDNESTED_VERSION HARDNESTED_COMMIT HARDNESTED_SOURCE_URL HARDNESTED_SOURCE_SHA256 HARDNESTED_LICENSE HARDNESTED_LIBNFC_VERSION; do
  if [[ -z "${!lock_name:-}" ]]; then
    echo "hardnested toolchain: missing $lock_name in $LOCK_FILE" >&2
    exit 1
  fi
done

PLATFORM="$(nfcx_platform)"
EXE_SUFFIX="$(nfcx_exe_suffix "$PLATFORM")"
SDK_DIR="${NFCX_LIBNFC_SDK_DIR:-$REPO_ROOT/build/toolchain/$PLATFORM/sdk}"
RUNTIME_DIR="${NFCX_RUNTIME_DIR:-$REPO_ROOT/runtime/$PLATFORM}"
CACHE_DIR="${NFCX_TOOLCHAIN_CACHE_DIR:-$REPO_ROOT/.cache/toolchain}"
BUILD_INFO_DIR="${NFCX_HARDNESTED_BUILD_INFO_DIR:-$REPO_ROOT/build/toolchain/$PLATFORM/hardnested-build-info}"
ARCHIVE="$CACHE_DIR/mfoc-hardnested-$HARDNESTED_COMMIT.tar.gz"
TEMP_BASE="${TMPDIR:-/tmp}"
TEMP_BASE="${TEMP_BASE%/}"
CLEANUP_DIR=""

cleanup() {
  case "$CLEANUP_DIR" in
    "$TEMP_BASE"/nfcx-hardnested-*) rm -rf "$CLEANUP_DIR" ;;
  esac
}
trap cleanup EXIT

sha256_file() {
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    sha256sum "$1" | awk '{print $1}'
  fi
}

obtain_archive() {
  local source="${NFCX_HARDNESTED_ARCHIVE:-$ARCHIVE}" actual temporary
  if [[ ! -f "$source" ]]; then
    if [[ -n "${NFCX_HARDNESTED_ARCHIVE:-}" ]]; then
      echo "hardnested toolchain: NFCX_HARDNESTED_ARCHIVE does not exist: $source" >&2
      return 1
    fi
    mkdir -p "$CACHE_DIR"
    temporary="$ARCHIVE.download.$$"
    nfcx_download_file "$HARDNESTED_SOURCE_URL" "$temporary" || return 1
    mv "$temporary" "$ARCHIVE"
  fi
  actual="$(sha256_file "$source")"
  if [[ "$actual" != "$HARDNESTED_SOURCE_SHA256" ]]; then
    echo "hardnested toolchain: source checksum mismatch; expected $HARDNESTED_SOURCE_SHA256, got $actual" >&2
    return 1
  fi
  printf '%s\n' "$source"
}

check() {
  local missing=0 tool
  local tools=(awk autoreconf cc curl install make mkdir mktemp mv pkg-config strings strip tar)
  case "$PLATFORM" in
    darwin-*) tools+=(install_name_tool otool) ;;
    linux-*) tools+=(patchelf readelf) ;;
    windows-*) tools+=(objdump) ;;
  esac
  for tool in "${tools[@]}"; do
    if command -v "$tool" >/dev/null 2>&1; then
      printf '  found   %-20s %s\n' "$tool" "$(command -v "$tool")"
    else
      printf '  missing %-20s\n' "$tool"
      missing=1
    fi
  done
  [[ -f "$SDK_DIR/lib/pkgconfig/libnfc.pc" ]] || { echo "  missing libnfc SDK: $SDK_DIR"; missing=1; }
  pkg-config --exists liblzma || { echo "  missing liblzma development files"; missing=1; }
  [[ -x "$RUNTIME_DIR/mfoc-hardnested$EXE_SUFFIX" ]] || echo "  mfoc-hardnested runtime not built"
  return "$missing"
}

build() {
  local archive work source binary
  check >/dev/null
  archive="$(obtain_archive)"
  work="$(mktemp -d "$TEMP_BASE/nfcx-hardnested-build.XXXXXX")"
  CLEANUP_DIR="$work"
  tar -xzf "$archive" -C "$work"
  source="$work/mfoc-hardnested-$HARDNESTED_COMMIT"
  [[ -f "$source/configure.ac" ]] || { echo "hardnested toolchain: unexpected source archive layout" >&2; return 1; }
  mkdir -p "$BUILD_INFO_DIR" "$RUNTIME_DIR/LICENSES"
  cp "$LOCK_FILE" "$BUILD_INFO_DIR/source.lock"
  (
    cd "$source"
    autoreconf -is
    env CFLAGS="${CFLAGS:-} -O2 -g0" PKG_CONFIG_PATH="$SDK_DIR/lib/pkgconfig" ./configure --prefix="$work/install" --disable-dependency-tracking
    make -j"${NFCX_BUILD_JOBS:-4}"
  )
  binary="$source/src/mfoc-hardnested$EXE_SUFFIX"
  nfcx_add_runtime_rpath "$PLATFORM" "$binary"
  nfcx_strip_release_binary "$PLATFORM" "$binary"
  install -m 0755 "$binary" "$RUNTIME_DIR/mfoc-hardnested$EXE_SUFFIX"
  if [[ "$PLATFORM" == windows-* ]]; then
    local lzma_prefix lzma_dll
    lzma_prefix="$(pkg-config --variable=prefix liblzma)"
    lzma_dll="$(find "$lzma_prefix/bin" -maxdepth 1 -type f -iname 'liblzma-*.dll' -print | head -n 1)"
    [[ -n "$lzma_dll" ]] || { echo "hardnested toolchain: liblzma runtime DLL is missing" >&2; return 1; }
    install -m 0755 "$lzma_dll" "$RUNTIME_DIR/$(basename "$lzma_dll")"
    if [[ -f "$lzma_prefix/share/licenses/xz/COPYING" ]]; then
      install -m 0644 "$lzma_prefix/share/licenses/xz/COPYING" "$RUNTIME_DIR/LICENSES/xz-COPYING.txt"
    fi
  fi
  install -m 0644 "$source/COPYING" "$RUNTIME_DIR/LICENSES/mfoc-hardnested-COPYING.txt"
  install -m 0644 "$LOCK_FILE" "$RUNTIME_DIR/mfoc-hardnested-source.lock"
  sha256_file "$RUNTIME_DIR/mfoc-hardnested$EXE_SUFFIX" >"$RUNTIME_DIR/mfoc-hardnested.sha256"
  verify
}

verify() {
  local help version binary="$RUNTIME_DIR/mfoc-hardnested$EXE_SUFFIX"
  [[ -x "$binary" ]] || { echo "hardnested verify: executable is missing" >&2; return 1; }
  [[ -f "$RUNTIME_DIR/LICENSES/mfoc-hardnested-COPYING.txt" ]] || { echo "hardnested verify: license is missing" >&2; return 1; }
  [[ -f "$RUNTIME_DIR/mfoc-hardnested-source.lock" ]] || { echo "hardnested verify: source lock is missing" >&2; return 1; }
  nfcx_verify_libnfc_dependency "$PLATFORM" "$binary" || { echo "hardnested verify: private libnfc dependency is invalid" >&2; return 1; }
  nfcx_verify_no_build_path "$binary" "$REPO_ROOT" || { echo "hardnested verify: build path was embedded" >&2; return 1; }
  if [[ "$PLATFORM" == windows-* ]]; then
    objdump -p "$binary" | grep -Eiq 'DLL Name: .*liblzma.*\.dll' || { echo "hardnested verify: liblzma dependency is missing" >&2; return 1; }
    compgen -G "$RUNTIME_DIR/liblzma-*.dll" >/dev/null || { echo "hardnested verify: private liblzma DLL is missing" >&2; return 1; }
    [[ -f "$RUNTIME_DIR/LICENSES/xz-COPYING.txt" ]] || { echo "hardnested verify: xz license is missing" >&2; return 1; }
  fi
  help="$(env PATH="$RUNTIME_DIR:$PATH" "$binary" -h 2>&1)"
  version="$(printf '%s\n' "$help" | sed -n 's/^This is mfoc-hardnested version \([0-9.]*\)\.$/\1/p')"
  [[ "$version" == "$HARDNESTED_VERSION" ]] || { echo "hardnested verify: version is $version, expected $HARDNESTED_VERSION" >&2; return 1; }
  grep -Fq -- '-F' <<<"$help" || { echo "hardnested verify: force option is missing" >&2; return 1; }
  grep -Fq -- '-C' <<<"$help" || { echo "hardnested verify: skip-default option is missing" >&2; return 1; }
  echo "hardnested verify: $version at $binary"
}

case "${1:-}" in
  check) check ;;
  build) build ;;
  verify) verify ;;
  *) echo "Usage: scripts/toolchain/hardnested.sh check|build|verify" >&2; exit 2 ;;
esac
