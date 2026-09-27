#!/usr/bin/env bash

set -euo pipefail

export LC_ALL=C
export LANG=C

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)"
LOCK_FILE="$REPO_ROOT/third_party/libnfc/libnfc.lock"
PATCH_DIR="$REPO_ROOT/third_party/libnfc/patches"
# shellcheck source=platform.sh
source "$SCRIPT_DIR/platform.sh"

# shellcheck source=/dev/null
source "$LOCK_FILE"

required_lock_values=(
  LIBNFC_VERSION
  LIBNFC_TAG
  LIBNFC_COMMIT
  LIBNFC_SOURCE_URL
  LIBNFC_SOURCE_SHA256
  LIBNFC_LICENSE
  LIBNFC_UTILS_LICENSE
  LIBNFC_DRIVERS
)
for lock_name in "${required_lock_values[@]}"; do
  if [[ -z "${!lock_name:-}" ]]; then
    echo "libnfc toolchain: missing $lock_name in $LOCK_FILE" >&2
    exit 1
  fi
done

PLATFORM="$(nfcx_platform)"
EXE_SUFFIX="$(nfcx_exe_suffix "$PLATFORM")"
OUTPUT_ROOT="${NFCX_TOOLCHAIN_OUTPUT_DIR:-$REPO_ROOT/build/toolchain/$PLATFORM}"
SDK_DIR="$OUTPUT_ROOT/sdk"
BUILD_INFO_DIR="$OUTPUT_ROOT/build-info"
RUNTIME_DIR="${NFCX_RUNTIME_DIR:-$REPO_ROOT/runtime/$PLATFORM}"
CACHE_DIR="${NFCX_TOOLCHAIN_CACHE_DIR:-$REPO_ROOT/.cache/toolchain}"
ARCHIVE_NAME="libnfc-$LIBNFC_VERSION.tar.bz2"
CACHED_ARCHIVE="$CACHE_DIR/$ARCHIVE_NAME"
TEMP_BASE="${TMPDIR:-/tmp}"
TEMP_BASE="${TEMP_BASE%/}"
CLEANUP_DIR=""

cleanup() {
  if [[ -z "$CLEANUP_DIR" ]]; then
    return
  fi
  case "$CLEANUP_DIR" in
    "$TEMP_BASE"/nfcx-libnfc-*) rm -rf "$CLEANUP_DIR" ;;
  esac
}
trap cleanup EXIT

usage() {
  cat <<'EOF'
Usage: scripts/toolchain/libnfc.sh <command> [arguments]

Commands:
  check                Report host tools and whether build outputs exist.
  build                Download, verify and build the pinned libnfc release.
  verify               Verify drivers, artifacts and dynamic-library paths.
  smoke                Compile and run the minimal nfc_init/nfc_exit program.
  repeatability-check  Build and verify twice in independent temporary trees.
  hardware-smoke CONN  Run nfc-list with one explicit pn532_uart connstring.

Optional environment:
  NFCX_LIBNFC_ARCHIVE          Use an existing source archive instead of downloading.
  NFCX_TOOLCHAIN_CACHE_DIR     Override the verified download cache.
  NFCX_TOOLCHAIN_OUTPUT_DIR    Override the development SDK/output directory.
  NFCX_RUNTIME_DIR             Override the runtime directory.
  NFCX_BUILD_JOBS              Override parallel make job count.
EOF
}

have_command() {
  command -v "$1" >/dev/null 2>&1
}

sha256_file() {
  local file="$1"
  if have_command shasum; then
    shasum -a 256 "$file" | awk '{print $1}'
  elif have_command sha256sum; then
    sha256sum "$file" | awk '{print $1}'
  else
    echo "libnfc toolchain: shasum or sha256sum is required" >&2
    return 1
  fi
}

check_tools() {
  local missing=0
  local tool
  local tools=(awk cc cmp cp curl file grep head install ln make mkdir mktemp mv patch sed strings strip tar tee)
  case "$PLATFORM" in
    darwin-*) tools+=(install_name_tool otool) ;;
    linux-*) tools+=(patchelf readelf) ;;
    windows-*) tools+=(cmake ninja objdump) ;;
  esac

  echo "libnfc toolchain: host=$PLATFORM"
  echo "libnfc toolchain: version=$LIBNFC_VERSION commit=$LIBNFC_COMMIT"
  echo "libnfc toolchain: drivers=$LIBNFC_DRIVERS"
  for tool in "${tools[@]}"; do
    if have_command "$tool"; then
      printf '  found   %-20s %s\n' "$tool" "$(command -v "$tool")"
    else
      printf '  missing %-20s\n' "$tool"
      missing=1
    fi
  done
  if ! have_command shasum && ! have_command sha256sum; then
    printf '  missing %-20s\n' "shasum/sha256sum"
    missing=1
  fi

  if [[ -f "$SDK_DIR/include/nfc/nfc.h" ]]; then
    echo "libnfc toolchain: development SDK present at $SDK_DIR"
  else
    echo "libnfc toolchain: development SDK not built"
  fi
  if compgen -G "$RUNTIME_DIR/libnfc*" >/dev/null; then
    echo "libnfc toolchain: runtime present at $RUNTIME_DIR"
  else
    echo "libnfc toolchain: runtime not built"
  fi
  return "$missing"
}

obtain_archive() {
  local archive actual_hash temp_archive
  if [[ -n "${NFCX_LIBNFC_ARCHIVE:-}" ]]; then
    archive="$NFCX_LIBNFC_ARCHIVE"
    if [[ ! -f "$archive" ]]; then
      echo "libnfc toolchain: NFCX_LIBNFC_ARCHIVE does not exist: $archive" >&2
      return 1
    fi
  else
    mkdir -p "$CACHE_DIR"
    archive="$CACHED_ARCHIVE"
    if [[ ! -f "$archive" ]]; then
      temp_archive="$archive.download.$$"
      echo "libnfc toolchain: downloading $LIBNFC_SOURCE_URL" >&2
      nfcx_download_file "$LIBNFC_SOURCE_URL" "$temp_archive" || return 1
      actual_hash="$(sha256_file "$temp_archive")"
      if [[ "$actual_hash" != "$LIBNFC_SOURCE_SHA256" ]]; then
        rm -f "$temp_archive"
        echo "libnfc toolchain: source checksum mismatch" >&2
        echo "  expected: $LIBNFC_SOURCE_SHA256" >&2
        echo "  actual:   $actual_hash" >&2
        return 1
      fi
      mv "$temp_archive" "$archive"
    fi
  fi

  actual_hash="$(sha256_file "$archive")"
  if [[ "$actual_hash" != "$LIBNFC_SOURCE_SHA256" ]]; then
    echo "libnfc toolchain: source checksum mismatch for $archive" >&2
    echo "  expected: $LIBNFC_SOURCE_SHA256" >&2
    echo "  actual:   $actual_hash" >&2
    return 1
  fi
  printf '%s\n' "$archive"
}

prepare_output_root() {
  if [[ -e "$OUTPUT_ROOT" ]]; then
    case "$OUTPUT_ROOT" in
      "$REPO_ROOT/build/toolchain/"*)
        rm -rf "$OUTPUT_ROOT"
        ;;
      *)
        echo "libnfc toolchain: refusing to replace custom output directory: $OUTPUT_ROOT" >&2
        echo "Remove it explicitly or choose a new NFCX_TOOLCHAIN_OUTPUT_DIR." >&2
        return 1
        ;;
    esac
  fi
  mkdir -p "$SDK_DIR" "$BUILD_INFO_DIR"
}

apply_patches() {
  local source_dir="$1"
  local patch_name
  : >"$BUILD_INFO_DIR/patches-applied.txt"
  while IFS= read -r patch_name || [[ -n "$patch_name" ]]; do
    patch_name="${patch_name%$'\r'}"
    case "$patch_name" in
      ""|'#'*) continue ;;
    esac
    if [[ ! -f "$PATCH_DIR/$patch_name" ]]; then
      echo "libnfc toolchain: listed patch is missing: $patch_name" >&2
      return 1
    fi
    patch -d "$source_dir" -p1 <"$PATCH_DIR/$patch_name"
    printf '%s\n' "$patch_name" >>"$BUILD_INFO_DIR/patches-applied.txt"
  done <"$PATCH_DIR/series"
}

make_jobs() {
  if [[ -n "${NFCX_BUILD_JOBS:-}" ]]; then
    case "$NFCX_BUILD_JOBS" in
      *[!0-9]*|0)
        echo "libnfc toolchain: NFCX_BUILD_JOBS must be a positive integer" >&2
        return 1
        ;;
      *) printf '%s\n' "$NFCX_BUILD_JOBS" ;;
    esac
  else
    nfcx_cpu_count
  fi
}

build_windows_libnfc() {
  local source_dir="$1" work_dir="$2" build_dir jobs import_library built_dll built_mfsetuid header
  local cmake_args
  build_dir="$work_dir/cmake-build"
  jobs="$(make_jobs)"
  cmake_args=(
    -S "$source_dir"
    -B "$build_dir"
    -G Ninja
    "-DCMAKE_INSTALL_PREFIX=$SDK_DIR"
    -DCMAKE_INSTALL_LIBDIR=lib
    -DCMAKE_BUILD_TYPE=Release
    -DCMAKE_POLICY_VERSION_MINIMUM=3.5
    "-DCMAKE_C_FLAGS_RELEASE=-O2 -DNDEBUG -g0 -ffile-prefix-map=$work_dir=."
    -DLIBNFC_CONFFILES_MODE=OFF
    -DLIBNFC_ENVVARS=ON
    -DLIBNFC_DRIVER_PCSC=OFF
    -DLIBNFC_DRIVER_ACR122_PCSC=OFF
    -DLIBNFC_DRIVER_ACR122_USB=OFF
    -DLIBNFC_DRIVER_ACR122S=OFF
    -DLIBNFC_DRIVER_ARYGON=OFF
    -DLIBNFC_DRIVER_PN53X_USB=OFF
    -DLIBNFC_DRIVER_PN532_UART=ON
    -DLIBNFC_DRIVER_PN532_SPI=OFF
    -DLIBNFC_DRIVER_PN532_I2C=OFF
    -DBUILD_EXAMPLES=ON
    -DBUILD_UTILS=ON
  )

  printf '%q ' cmake "${cmake_args[@]}" >"$BUILD_INFO_DIR/configure-args.txt"
  printf '\n' >>"$BUILD_INFO_DIR/configure-args.txt"
  echo "libnfc toolchain: configuring Windows build with upstream CMake support"
  cmake "${cmake_args[@]}" 2>&1 | tee "$BUILD_INFO_DIR/configure.log"
  echo "libnfc toolchain: building nfc-mfsetuid and its dependencies with $jobs jobs"
  cmake --build "$build_dir" --target nfc-mfsetuid --parallel "$jobs" 2>&1 | tee "$BUILD_INFO_DIR/build.log"
  cp "$build_dir/CMakeCache.txt" "$BUILD_INFO_DIR/libnfc.CMakeCache.txt"

  # The old CMake install rules require every legacy example target and omit
  # the MinGW import archive and .pc file. Stage only NFCX's required SDK and
  # nfc-mfsetuid instead of linking unrelated diagnostic examples against
  # internal symbols that the DLL intentionally does not export.
  built_dll="$(find "$build_dir" -type f -name 'libnfc.dll' -print | head -n 1)"
  import_library="$(find "$build_dir" -type f -name 'libnfc.dll.a' -print | head -n 1)"
  built_mfsetuid="$(find "$build_dir" -type f -name 'nfc-mfsetuid.exe' -print | head -n 1)"
  [[ -n "$built_dll" ]] || { echo "libnfc toolchain: Windows DLL was not produced" >&2; return 1; }
  [[ -n "$import_library" ]] || { echo "libnfc toolchain: MinGW import library was not produced" >&2; return 1; }
  [[ -n "$built_mfsetuid" ]] || { echo "libnfc toolchain: nfc-mfsetuid.exe was not produced" >&2; return 1; }
  mkdir -p "$SDK_DIR/bin" "$SDK_DIR/include/nfc" "$SDK_DIR/lib/pkgconfig"
  install -m 0755 "$built_dll" "$SDK_DIR/bin/libnfc.dll"
  install -m 0755 "$built_mfsetuid" "$SDK_DIR/bin/nfc-mfsetuid.exe"
  install -m 0644 "$import_library" "$SDK_DIR/lib/libnfc.dll.a"
  for header in "$source_dir"/include/nfc/*.h; do
    install -m 0644 "$header" "$SDK_DIR/include/nfc/$(basename "$header")"
  done
  # mfoc, mfcuk, and mfoc-hardnested reuse libnfc's example helpers, which
  # include <err.h>. MinGW-w64 does not provide that BSD header, so expose the
  # compatibility implementation shipped by the same pinned libnfc source.
  # Its macros use stderr/fprintf without including stdio.h, so make the SDK
  # copy self-contained for translation units that include err.h first.
  {
    printf '%s\n' '#include <stdio.h>'
    cat "$source_dir/contrib/win32/err.h"
  } >"$SDK_DIR/include/err.h"
  chmod 0644 "$SDK_DIR/include/err.h"
  {
    printf 'prefix=%s\n' "$SDK_DIR"
    printf '%s\n' 'exec_prefix=${prefix}' 'libdir=${exec_prefix}/lib' 'includedir=${prefix}/include'
    printf '\nName: libnfc\nDescription: Near Field Communication (NFC) library\n'
    printf 'Version: %s\nRequires:\nLibs: -L${libdir} -lnfc\nCflags: -I${includedir}\n' "$LIBNFC_VERSION"
  } >"$SDK_DIR/lib/pkgconfig/libnfc.pc"
  printf '%s\n' 'staged libnfc.dll, libnfc.dll.a, public and Windows compatibility headers, libnfc.pc, and nfc-mfsetuid.exe' >"$BUILD_INFO_DIR/install.log"
}

verify_driver_selection() {
  local disabled_driver
  if [[ "$PLATFORM" == windows-* ]]; then
    grep -Fxq 'LIBNFC_DRIVER_PN532_UART:BOOL=ON' "$BUILD_INFO_DIR/libnfc.CMakeCache.txt" || {
      echo "libnfc verify: pn532_uart was not enabled in the Windows build" >&2
      return 1
    }
    for disabled_driver in PCSC ACR122_PCSC ACR122_USB ACR122S ARYGON PN53X_USB PN532_SPI PN532_I2C; do
      grep -Fxq "LIBNFC_DRIVER_${disabled_driver}:BOOL=OFF" "$BUILD_INFO_DIR/libnfc.CMakeCache.txt" || {
        echo "libnfc verify: out-of-scope Windows driver setting: $disabled_driver" >&2
        return 1
      }
    done
    return
  fi
  grep -Eq '^DRIVERS_CFLAGS = +-DDRIVER_PN532_UART_ENABLED *$' "$BUILD_INFO_DIR/libnfc.Makefile" || {
    echo "libnfc verify: pn532_uart was not the exact configured driver set" >&2
    return 1
  }
  if grep -Eq 'DRIVER_(PCSC|ACR122|PN53X_USB|ARYGON|PN532_(SPI|I2C)|PN71XX)_ENABLED' "$BUILD_INFO_DIR/libnfc.Makefile"; then
    echo "libnfc verify: an out-of-scope driver was enabled" >&2
    return 1
  fi
}

build_libnfc() {
  local archive work_dir source_dir build_dir shim_dir false_path jobs arg_max max_cmd_len runtime_library runtime_name soname
  check_tools >/dev/null
  archive="$(obtain_archive)"
  prepare_output_root
  work_dir="$(mktemp -d "$TEMP_BASE/nfcx-libnfc-build.XXXXXX")"
  CLEANUP_DIR="$work_dir"

  echo "libnfc toolchain: extracting verified source into $work_dir"
  tar -xjf "$archive" -C "$work_dir"
  source_dir="$work_dir/libnfc-$LIBNFC_VERSION"
  # Build in the disposable source tree so __FILE__ strings remain relative;
  # an out-of-tree build embeds the private absolute source path in libnfc.
  build_dir="$source_dir"
  shim_dir="$work_dir/configure-shims"
  if [[ ! -x "$source_dir/configure" ]]; then
    echo "libnfc toolchain: release archive does not contain the expected configure script" >&2
    return 1
  fi
  mkdir -p "$build_dir" "$shim_dir"
  apply_patches "$source_dir"

  if [[ "$PLATFORM" == windows-* ]]; then
    cp "$LOCK_FILE" "$BUILD_INFO_DIR/source.lock"
    build_windows_libnfc "$source_dir" "$work_dir"
  else

    # The release configure script unconditionally tries `git describe` when it
    # finds Git. Hide Git so it cannot record an enclosing NFCX revision or emit
    # a misleading fatal message for an archive without .git metadata.
    false_path="$(type -P false)"
    ln -s "$false_path" "$shim_dir/git"

    configure_args=(
      "--prefix=$SDK_DIR"
      "--sysconfdir=$SDK_DIR/etc"
      "--with-drivers=$LIBNFC_DRIVERS"
      "--enable-shared"
      "--disable-static"
      "--disable-conffiles"
      "--enable-envvars"
      "--disable-dependency-tracking"
    )
    printf '%q ' "${configure_args[@]}" >"$BUILD_INFO_DIR/configure-args.txt"
    printf '\n' >>"$BUILD_INFO_DIR/configure-args.txt"
    cp "$LOCK_FILE" "$BUILD_INFO_DIR/source.lock"

    echo "libnfc toolchain: configuring drivers=$LIBNFC_DRIVERS"
    arg_max="$(nfcx_arg_max)"
    max_cmd_len="$((arg_max / 4 * 3))"
    printf 'lt_cv_sys_max_cmd_len=%s\n' "$max_cmd_len" >"$BUILD_INFO_DIR/configure-env.txt"
    (
      cd "$build_dir"
      PATH="$shim_dir:$PATH" CFLAGS="${CFLAGS:-} -O2 -g0" lt_cv_sys_max_cmd_len="$max_cmd_len" \
        "$source_dir/configure" "${configure_args[@]}" 2>&1 | tee "$BUILD_INFO_DIR/configure.log"
    )

    jobs="$(make_jobs)"
    echo "libnfc toolchain: building with $jobs jobs"
    (
      cd "$build_dir"
      make -j"$jobs" 2>&1 | tee "$BUILD_INFO_DIR/build.log"
      make install 2>&1 | tee "$BUILD_INFO_DIR/install.log"
    )
    cp "$build_dir/libnfc/Makefile" "$BUILD_INFO_DIR/libnfc.Makefile"
  fi

  mkdir -p "$RUNTIME_DIR/LICENSES"
  case "$PLATFORM" in
    darwin-*)
      runtime_library="$(find "$SDK_DIR/lib" -maxdepth 1 -type f -name 'libnfc.*.dylib' -print | head -n 1)"
      [[ -n "$runtime_library" ]] || { echo "libnfc toolchain: installed versioned dylib was not found" >&2; return 1; }
      runtime_name="$(basename "$runtime_library")"
      install_name_tool -id "@rpath/$runtime_name" "$runtime_library"
      nfcx_strip_release_binary "$PLATFORM" "$runtime_library"
      rm -f "$RUNTIME_DIR/libnfc.dylib" "$RUNTIME_DIR"/libnfc.*.dylib
      install -m 0755 "$runtime_library" "$RUNTIME_DIR/$runtime_name"
      ln -s "$runtime_name" "$RUNTIME_DIR/libnfc.dylib"
      install -m 0755 "$SDK_DIR/bin/nfc-mfsetuid" "$RUNTIME_DIR/nfc-mfsetuid"
      install_name_tool -change "$SDK_DIR/lib/$runtime_name" "@rpath/$runtime_name" "$RUNTIME_DIR/nfc-mfsetuid"
      install_name_tool -add_rpath "@loader_path" "$RUNTIME_DIR/nfc-mfsetuid"
      nfcx_strip_release_binary "$PLATFORM" "$RUNTIME_DIR/nfc-mfsetuid"
      ;;
    linux-*)
      runtime_library="$(find "$SDK_DIR/lib" -maxdepth 1 -type f -name 'libnfc.so.*.*.*' -print | head -n 1)"
      [[ -n "$runtime_library" ]] || { echo "libnfc toolchain: installed versioned shared object was not found" >&2; return 1; }
      runtime_name="$(basename "$runtime_library")"
      soname="$(readelf -d "$runtime_library" | sed -n 's/.*SONAME.*\[\(.*\)\].*/\1/p')"
      [[ -n "$soname" ]] || { echo "libnfc toolchain: shared object has no SONAME" >&2; return 1; }
      rm -f "$RUNTIME_DIR"/libnfc.so*
      nfcx_strip_release_binary "$PLATFORM" "$runtime_library"
      install -m 0755 "$runtime_library" "$RUNTIME_DIR/$runtime_name"
      ln -s "$runtime_name" "$RUNTIME_DIR/$soname"
      ln -s "$soname" "$RUNTIME_DIR/libnfc.so"
      install -m 0755 "$SDK_DIR/bin/nfc-mfsetuid" "$RUNTIME_DIR/nfc-mfsetuid"
      patchelf --set-rpath '$ORIGIN' "$RUNTIME_DIR/nfc-mfsetuid"
      nfcx_strip_release_binary "$PLATFORM" "$RUNTIME_DIR/nfc-mfsetuid"
      ;;
    windows-*)
      runtime_library="$(find "$SDK_DIR/bin" -maxdepth 1 -type f -iname '*nfc*.dll' -print | head -n 1)"
      [[ -n "$runtime_library" ]] || { echo "libnfc toolchain: installed libnfc DLL was not found" >&2; return 1; }
      runtime_name="$(basename "$runtime_library")"
      rm -f "$RUNTIME_DIR"/*nfc*.dll
      nfcx_strip_release_binary "$PLATFORM" "$runtime_library"
      install -m 0755 "$runtime_library" "$RUNTIME_DIR/$runtime_name"
      install -m 0755 "$SDK_DIR/bin/nfc-mfsetuid.exe" "$RUNTIME_DIR/nfc-mfsetuid.exe"
      nfcx_strip_release_binary "$PLATFORM" "$RUNTIME_DIR/nfc-mfsetuid.exe"
      ;;
  esac
  install -m 0644 "$source_dir/COPYING" "$RUNTIME_DIR/LICENSES/libnfc-COPYING.txt"
  install -m 0644 "$REPO_ROOT/third_party/libnfc/nfc-mfsetuid-BSD-2-Clause.txt" "$RUNTIME_DIR/LICENSES/nfc-mfsetuid-BSD-2-Clause.txt"
  install -m 0644 "$LOCK_FILE" "$RUNTIME_DIR/source.lock"

  echo "libnfc toolchain: build complete"
  echo "  development SDK: $SDK_DIR"
  echo "  application runtime: $RUNTIME_DIR"
}

runtime_dylib() {
  find "$RUNTIME_DIR" -maxdepth 1 -type f -name 'libnfc.*.dylib' -print | head -n 1
}

verify_non_darwin_libnfc() {
  local runtime_library mfsetuid="$RUNTIME_DIR/nfc-mfsetuid$EXE_SUFFIX" dependency
  local required_files=(
    "$SDK_DIR/include/nfc/nfc.h"
    "$SDK_DIR/lib/pkgconfig/libnfc.pc"
    "$BUILD_INFO_DIR/configure.log"
    "$RUNTIME_DIR/LICENSES/libnfc-COPYING.txt"
    "$RUNTIME_DIR/LICENSES/nfc-mfsetuid-BSD-2-Clause.txt"
    "$mfsetuid"
    "$RUNTIME_DIR/source.lock"
  )
  if [[ "$PLATFORM" == windows-* ]]; then
    required_files+=(
      "$SDK_DIR/include/err.h"
      "$BUILD_INFO_DIR/libnfc.CMakeCache.txt"
    )
  else
    required_files+=("$BUILD_INFO_DIR/libnfc.Makefile")
  fi
  for required_file in "${required_files[@]}"; do
    [[ -e "$required_file" ]] || { echo "libnfc verify: missing $required_file" >&2; return 1; }
  done
  verify_driver_selection
  case "$PLATFORM" in
    linux-*)
      runtime_library="$(find "$RUNTIME_DIR" -maxdepth 1 -type f -name 'libnfc.so.*.*.*' -print | head -n 1)"
      [[ -n "$runtime_library" ]] || { echo "libnfc verify: runtime shared object is missing" >&2; return 1; }
      file "$runtime_library" | grep -Eq 'ELF 64-bit.*x86-64' || { echo "libnfc verify: shared object is not Linux amd64" >&2; return 1; }
      dependency="$(readelf -d "$mfsetuid" | sed -n 's/.*NEEDED.*\[\(libnfc\.so\.[0-9]*\)\].*/\1/p')"
      [[ -n "$dependency" && -e "$RUNTIME_DIR/$dependency" ]] || { echo "libnfc verify: nfc-mfsetuid private libnfc dependency is missing" >&2; return 1; }
      patchelf --print-rpath "$mfsetuid" | grep -Fxq '$ORIGIN' || { echo "libnfc verify: nfc-mfsetuid runtime rpath is missing" >&2; return 1; }
      ;;
    windows-*)
      runtime_library="$(find "$RUNTIME_DIR" -maxdepth 1 -type f -iname '*nfc*.dll' -print | head -n 1)"
      [[ -n "$runtime_library" ]] || { echo "libnfc verify: runtime DLL is missing" >&2; return 1; }
      file "$runtime_library" | grep -Eq 'PE32\+.*x86-64' || { echo "libnfc verify: DLL is not Windows amd64" >&2; return 1; }
      objdump -p "$mfsetuid" | grep -Eiq 'DLL Name: .*libnfc.*\.dll' || { echo "libnfc verify: nfc-mfsetuid libnfc import is missing" >&2; return 1; }
      ;;
  esac
  if strings "$runtime_library" | grep -Eq '/Users/|/home/|/private/tmp/|/private/var/folders/|[A-Za-z]:[/\\](Users|actions)[/\\]'; then
    echo "libnfc verify: runtime library contains a private build path" >&2
    return 1
  fi
  nfcx_verify_no_build_path "$mfsetuid" "$REPO_ROOT" || { echo "libnfc verify: nfc-mfsetuid contains a build path" >&2; return 1; }
  cmp -s "$LOCK_FILE" "$RUNTIME_DIR/source.lock" || { echo "libnfc verify: runtime source lock differs from repository lock" >&2; return 1; }
  echo "libnfc verify: PASS ($PLATFORM, pn532_uart only)"
}

verify_libnfc() {
  if [[ "$PLATFORM" != darwin-* ]]; then
    verify_non_darwin_libnfc
    return
  fi
  local dylib dylib_name dependency dylib_id mfsetuid_dependency
  local required_files=(
    "$SDK_DIR/include/nfc/nfc.h"
    "$SDK_DIR/lib/pkgconfig/libnfc.pc"
    "$SDK_DIR/bin/nfc-list"
    "$SDK_DIR/bin/nfc-scan-device"
    "$SDK_DIR/bin/nfc-mfsetuid"
    "$BUILD_INFO_DIR/configure.log"
    "$BUILD_INFO_DIR/libnfc.Makefile"
    "$RUNTIME_DIR/LICENSES/libnfc-COPYING.txt"
    "$RUNTIME_DIR/LICENSES/nfc-mfsetuid-BSD-2-Clause.txt"
    "$RUNTIME_DIR/nfc-mfsetuid"
    "$RUNTIME_DIR/source.lock"
  )
  for required_file in "${required_files[@]}"; do
    if [[ ! -e "$required_file" ]]; then
      echo "libnfc verify: missing $required_file" >&2
      return 1
    fi
  done

  if ! grep -Eq '^DRIVERS_CFLAGS = +-DDRIVER_PN532_UART_ENABLED *$' "$BUILD_INFO_DIR/libnfc.Makefile"; then
    echo "libnfc verify: pn532_uart was not the exact configured driver set" >&2
    return 1
  fi
  if grep -Eq 'DRIVER_(PCSC|ACR122|PN53X_USB|ARYGON|PN532_(SPI|I2C)|PN71XX)_ENABLED' "$BUILD_INFO_DIR/libnfc.Makefile"; then
    echo "libnfc verify: an out-of-scope driver was enabled" >&2
    return 1
  fi

  dylib="$(runtime_dylib)"
  if [[ -z "$dylib" ]]; then
    echo "libnfc verify: runtime dylib not found in $RUNTIME_DIR" >&2
    return 1
  fi
  dylib_name="$(basename "$dylib")"
  if ! file "$dylib" | grep -q 'arm64'; then
    echo "libnfc verify: runtime dylib is not arm64" >&2
    return 1
  fi
  dylib_id="$(otool -D "$dylib" | sed -n '2p')"
  if [[ "$dylib_id" != "@rpath/$dylib_name" ]]; then
    echo "libnfc verify: unexpected dylib install name: $dylib_id" >&2
    return 1
  fi
  mfsetuid_dependency="$(otool -L "$RUNTIME_DIR/nfc-mfsetuid" | sed -n '2s/^[[:space:]]*\([^[:space:]]*\).*/\1/p')"
  if [[ "$mfsetuid_dependency" != "@rpath/$dylib_name" ]]; then
    echo "libnfc verify: nfc-mfsetuid has a non-portable libnfc dependency: $mfsetuid_dependency" >&2
    return 1
  fi
  if ! otool -l "$RUNTIME_DIR/nfc-mfsetuid" | grep -A3 LC_RPATH | grep -Fq '@loader_path'; then
    echo "libnfc verify: nfc-mfsetuid is missing its runtime @loader_path" >&2
    return 1
  fi
  while IFS= read -r dependency; do
    case "$dependency" in
      /usr/lib/*|/System/Library/*|@rpath/*|@loader_path/*|@executable_path/*) ;;
      *)
        echo "libnfc verify: non-portable dependency: $dependency" >&2
        return 1
        ;;
    esac
  done < <(otool -L "$dylib" | awk 'NR > 1 {print $1}')
  if strings "$dylib" | grep -Eq '/Users/|/private/tmp/|/private/var/folders/|/var/folders/'; then
    echo "libnfc verify: runtime dylib contains a private build path" >&2
    return 1
  fi
  if ! cmp -s "$LOCK_FILE" "$RUNTIME_DIR/source.lock"; then
    echo "libnfc verify: runtime source lock differs from the repository lock" >&2
    return 1
  fi

  echo "libnfc verify: PASS"
  echo "  driver: pn532_uart only"
  echo "  architecture: arm64"
  echo "  install name: $dylib_id"
  echo "  private dependencies: none"
}

smoke_libnfc() {
  local smoke_dir smoke_binary dylib
  if [[ "$PLATFORM" != darwin-* ]]; then
    smoke_dir="$OUTPUT_ROOT/smoke"
    smoke_binary="$smoke_dir/libnfc-smoke$EXE_SUFFIX"
    mkdir -p "$smoke_dir"
    env PKG_CONFIG_PATH="$SDK_DIR/lib/pkgconfig" cc \
      "$SCRIPT_DIR/libnfc-smoke.c" \
      $(PKG_CONFIG_PATH="$SDK_DIR/lib/pkgconfig" pkg-config --cflags --libs libnfc) \
      -o "$smoke_binary"
    env -u LIBNFC_DEVICE -u LIBNFC_DEFAULT_DEVICE \
      PATH="$RUNTIME_DIR:$PATH" LD_LIBRARY_PATH="$RUNTIME_DIR" \
      LIBNFC_AUTO_SCAN=false LIBNFC_INTRUSIVE_SCAN=false NFCX_EXPECT_DEVICE_COUNT=0 \
      "$smoke_binary"
    env PATH="$RUNTIME_DIR:$PATH" LD_LIBRARY_PATH="$RUNTIME_DIR" \
      LIBNFC_DEVICE="pn532_uart:/dev/nfcx-smoke-not-opened" \
      LIBNFC_AUTO_SCAN=false LIBNFC_INTRUSIVE_SCAN=false NFCX_EXPECT_DEVICE_COUNT=1 \
      "$smoke_binary"
    return
  fi
  dylib="$(runtime_dylib)"
  if [[ -z "$dylib" || ! -f "$SDK_DIR/include/nfc/nfc.h" ]]; then
    echo "libnfc smoke: build outputs are missing; run make libnfc-build first" >&2
    return 1
  fi
  smoke_dir="$OUTPUT_ROOT/smoke"
  smoke_binary="$smoke_dir/libnfc-smoke"
  mkdir -p "$smoke_dir"
  cc \
    -I"$SDK_DIR/include" \
    "$SCRIPT_DIR/libnfc-smoke.c" \
    -L"$RUNTIME_DIR" \
    -lnfc \
    -Wl,-rpath,"$RUNTIME_DIR" \
    -o "$smoke_binary"
  env -u LIBNFC_DEVICE -u LIBNFC_DEFAULT_DEVICE \
    LIBNFC_AUTO_SCAN=false \
    LIBNFC_INTRUSIVE_SCAN=false \
    NFCX_EXPECT_DEVICE_COUNT=0 \
    "$smoke_binary"
  LIBNFC_DEVICE="pn532_uart:/dev/nfcx-smoke-not-opened" \
    LIBNFC_AUTO_SCAN=false \
    LIBNFC_INTRUSIVE_SCAN=false \
    NFCX_EXPECT_DEVICE_COUNT=1 \
    "$smoke_binary"
}

hardware_smoke() {
  local connstring="${1:-${LIBNFC_DEVICE:-}}"
  if [[ -z "$connstring" ]]; then
    echo "libnfc hardware smoke: a connstring is required" >&2
    echo "Example: $0 hardware-smoke pn532_uart:/dev/cu.usbserial-..." >&2
    return 2
  fi
  case "$connstring" in
    pn532_uart:*) ;;
    *)
      echo "libnfc hardware smoke: only pn532_uart connstrings are accepted in this stage" >&2
      return 2
      ;;
  esac
  if [[ ! -x "$SDK_DIR/bin/nfc-list" ]]; then
    echo "libnfc hardware smoke: nfc-list is missing; run make libnfc-build first" >&2
    return 1
  fi
  echo "libnfc hardware smoke: using explicit device $connstring"
  LIBNFC_DEVICE="$connstring" \
    LIBNFC_AUTO_SCAN=false \
    DYLD_LIBRARY_PATH="$SDK_DIR/lib${DYLD_LIBRARY_PATH:+:$DYLD_LIBRARY_PATH}" \
    "$SDK_DIR/bin/nfc-list" -v
}

repeatability_check() {
  local archive test_root run_index
  check_tools >/dev/null
  archive="$(obtain_archive)"
  test_root="$(mktemp -d "$TEMP_BASE/nfcx-libnfc-repeatability.XXXXXX")"
  CLEANUP_DIR="$test_root"
  for run_index in 1 2; do
    echo "libnfc repeatability: clean build $run_index/2"
    NFCX_LIBNFC_ARCHIVE="$archive" \
      NFCX_TOOLCHAIN_OUTPUT_DIR="$test_root/run-$run_index/toolchain/$PLATFORM" \
      NFCX_RUNTIME_DIR="$test_root/run-$run_index/runtime/$PLATFORM" \
      "$0" build
    NFCX_TOOLCHAIN_OUTPUT_DIR="$test_root/run-$run_index/toolchain/$PLATFORM" \
      NFCX_RUNTIME_DIR="$test_root/run-$run_index/runtime/$PLATFORM" \
      "$0" verify
    NFCX_TOOLCHAIN_OUTPUT_DIR="$test_root/run-$run_index/toolchain/$PLATFORM" \
      NFCX_RUNTIME_DIR="$test_root/run-$run_index/runtime/$PLATFORM" \
      "$0" smoke
  done
  echo "libnfc repeatability: PASS (two independent clean builds)"
}

command_name="${1:-}"
case "$command_name" in
  check) check_tools ;;
  build) build_libnfc ;;
  verify) verify_libnfc ;;
  smoke) smoke_libnfc ;;
  hardware-smoke) shift; hardware_smoke "${1:-}" ;;
  repeatability-check) repeatability_check ;;
  -h|--help|help) usage ;;
  *) usage >&2; exit 2 ;;
esac
