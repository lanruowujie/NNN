#!/usr/bin/env bash

set -euo pipefail

export LC_ALL=C
export LANG=C

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)"
LOCK_FILE="$REPO_ROOT/third_party/mfcuk/mfcuk.lock"
# shellcheck source=platform.sh
source "$SCRIPT_DIR/platform.sh"
# shellcheck source=/dev/null
source "$LOCK_FILE"

for lock_name in MFCUK_VERSION MFCUK_TAG MFCUK_COMMIT MFCUK_SOURCE_URL MFCUK_SOURCE_SHA256 MFCUK_LICENSE MFCUK_LIBNFC_VERSION MFCUK_PATCH_SERIES; do
  if [[ -z "${!lock_name:-}" ]]; then
    echo "mfcuk toolchain: missing $lock_name in $LOCK_FILE" >&2
    exit 1
  fi
done

PLATFORM="$(nfcx_platform)"
EXE_SUFFIX="$(nfcx_exe_suffix "$PLATFORM")"
SDK_DIR="${NFCX_LIBNFC_SDK_DIR:-$REPO_ROOT/build/toolchain/$PLATFORM/sdk}"
RUNTIME_DIR="${NFCX_RUNTIME_DIR:-$REPO_ROOT/runtime/$PLATFORM}"
CACHE_DIR="${NFCX_TOOLCHAIN_CACHE_DIR:-$REPO_ROOT/.cache/toolchain}"
BUILD_INFO_DIR="${NFCX_MFCUK_BUILD_INFO_DIR:-$REPO_ROOT/build/toolchain/$PLATFORM/mfcuk-build-info}"
ARCHIVE="$CACHE_DIR/mfcuk-$MFCUK_VERSION.tar.gz"
PATCH_FILE="$REPO_ROOT/third_party/mfcuk/patches/$MFCUK_PATCH_SERIES"
TEMP_BASE="${TMPDIR:-/tmp}"
TEMP_BASE="${TEMP_BASE%/}"
CLEANUP_DIR=""

cleanup() {
  case "$CLEANUP_DIR" in
    "$TEMP_BASE"/nfcx-mfcuk-*) rm -rf "$CLEANUP_DIR" ;;
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
  local source="${NFCX_MFCUK_ARCHIVE:-$ARCHIVE}" actual temporary
  if [[ ! -f "$source" ]]; then
    if [[ -n "${NFCX_MFCUK_ARCHIVE:-}" ]]; then
      echo "mfcuk toolchain: NFCX_MFCUK_ARCHIVE does not exist: $source" >&2
      return 1
    fi
    mkdir -p "$CACHE_DIR"
    temporary="$ARCHIVE.download.$$"
    nfcx_download_file "$MFCUK_SOURCE_URL" "$temporary" || return 1
    mv "$temporary" "$ARCHIVE"
  fi
  actual="$(sha256_file "$source")"
  if [[ "$actual" != "$MFCUK_SOURCE_SHA256" ]]; then
    echo "mfcuk toolchain: source checksum mismatch; expected $MFCUK_SOURCE_SHA256, got $actual" >&2
    return 1
  fi
  printf '%s\n' "$source"
}

check() {
  local missing=0 tool
  local tools=(awk autoreconf cc curl install make mkdir mktemp mv patch pkg-config strings strip tar)
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
  [[ -f "$PATCH_FILE" ]] || { echo "  missing NFCX result patch: $PATCH_FILE"; missing=1; }
  [[ -x "$RUNTIME_DIR/mfcuk$EXE_SUFFIX" ]] || echo "  mfcuk runtime not built"
  return "$missing"
}

build() {
  local archive work source binary
  check >/dev/null
  archive="$(obtain_archive)"
  work="$(mktemp -d "$TEMP_BASE/nfcx-mfcuk-build.XXXXXX")"
  CLEANUP_DIR="$work"
  tar -xzf "$archive" -C "$work"
  source="$work/mfcuk-$MFCUK_TAG"
  if [[ ! -d "$source" ]]; then
    source="$work/mfcuk-mfcuk-$MFCUK_VERSION"
  fi
  [[ -f "$source/configure.ac" ]] || { echo "mfcuk toolchain: unexpected source archive layout" >&2; return 1; }
  mkdir -p "$BUILD_INFO_DIR" "$RUNTIME_DIR/LICENSES" "$RUNTIME_DIR/patches"
  cp "$LOCK_FILE" "$BUILD_INFO_DIR/source.lock"
  cp "$PATCH_FILE" "$BUILD_INFO_DIR/$MFCUK_PATCH_SERIES"
  (
    cd "$source"
    patch -p1 <"$PATCH_FILE"
    autoreconf -is
    env CFLAGS="${CFLAGS:-} -O2 -g0" PKG_CONFIG_PATH="$SDK_DIR/lib/pkgconfig" ./configure --prefix="$work/install" --disable-dependency-tracking
    make -j"${NFCX_BUILD_JOBS:-4}"
  )
  binary="$source/src/mfcuk$EXE_SUFFIX"
  nfcx_add_runtime_rpath "$PLATFORM" "$binary"
  nfcx_strip_release_binary "$PLATFORM" "$binary"
  install -m 0755 "$binary" "$RUNTIME_DIR/mfcuk$EXE_SUFFIX"
  install -m 0644 "$source/COPYING" "$RUNTIME_DIR/LICENSES/mfcuk-COPYING.txt"
  install -m 0644 "$LOCK_FILE" "$RUNTIME_DIR/mfcuk-source.lock"
  install -m 0644 "$PATCH_FILE" "$RUNTIME_DIR/patches/$MFCUK_PATCH_SERIES"
  sha256_file "$RUNTIME_DIR/mfcuk$EXE_SUFFIX" >"$RUNTIME_DIR/mfcuk.sha256"
  verify
}

verify() {
  local help version binary="$RUNTIME_DIR/mfcuk$EXE_SUFFIX"
  [[ -x "$binary" ]] || { echo "mfcuk verify: executable is missing" >&2; return 1; }
  [[ -f "$RUNTIME_DIR/LICENSES/mfcuk-COPYING.txt" ]] || { echo "mfcuk verify: license is missing" >&2; return 1; }
  [[ -f "$RUNTIME_DIR/mfcuk-source.lock" ]] || { echo "mfcuk verify: source lock is missing" >&2; return 1; }
  [[ -f "$RUNTIME_DIR/patches/$MFCUK_PATCH_SERIES" ]] || { echo "mfcuk verify: source patch is missing" >&2; return 1; }
  nfcx_verify_libnfc_dependency "$PLATFORM" "$binary" || { echo "mfcuk verify: private libnfc dependency is invalid" >&2; return 1; }
  nfcx_verify_no_build_path "$binary" "$REPO_ROOT" || { echo "mfcuk verify: build path was embedded" >&2; return 1; }
  help="$(env PATH="$RUNTIME_DIR:$PATH" "$binary" -h 2>&1 || true)"
  version="$(printf '%s\n' "$help" | sed -n 's/^NFCX_VERSION \([0-9.]*\)$/\1/p')"
  [[ "$version" == "$MFCUK_VERSION" ]] || { echo "mfcuk verify: version is $version, expected $MFCUK_VERSION" >&2; return 1; }
  grep -Fq -- '-R sector' <<<"$help" || { echo "mfcuk verify: recovery option is missing" >&2; return 1; }
  strings "$binary" | grep -Fq 'NFCX_RESULT key=' || { echo "mfcuk verify: machine result marker is missing" >&2; return 1; }
  echo "mfcuk verify: $version at $binary"
}

case "${1:-}" in
  check) check ;;
  build) build ;;
  verify) verify ;;
  *) echo "Usage: scripts/toolchain/mfcuk.sh check|build|verify" >&2; exit 2 ;;
esac
