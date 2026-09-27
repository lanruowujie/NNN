#!/usr/bin/env bash

# Shared host and binary-format helpers for the pinned native toolchains.

nfcx_platform() {
  if [[ -n "${NFCX_PLATFORM:-}" ]]; then
    printf '%s\n' "$NFCX_PLATFORM"
    return
  fi
  case "$(uname -s):$(uname -m)" in
    Darwin:arm64) printf '%s\n' darwin-arm64 ;;
    Linux:x86_64) printf '%s\n' linux-amd64 ;;
    MINGW64_NT-*:*|MSYS_NT-*:*|CYGWIN_NT-*:* ) printf '%s\n' windows-amd64 ;;
    *) echo "unsupported NFCX release host $(uname -s)/$(uname -m)" >&2; return 1 ;;
  esac
}

nfcx_exe_suffix() {
  if [[ "$1" == windows-* ]]; then
    printf '%s' .exe
  fi
}

nfcx_download_file() {
  local url="$1" destination="$2"
  rm -f "$destination"
  if ! curl -fL --retry 6 --retry-delay 2 --retry-all-errors \
    --connect-timeout 30 --max-time 600 -o "$destination" "$url"; then
    rm -f "$destination"
    echo "download failed after retries: $url" >&2
    return 1
  fi
  if [[ ! -s "$destination" ]]; then
    rm -f "$destination"
    echo "download returned an empty file: $url" >&2
    return 1
  fi
}

nfcx_arg_max() {
  local value
  value="$(getconf ARG_MAX 2>/dev/null || true)"
  value="${value%$'\r'}"
  case "$value" in
    ''|*[!0-9]*|0) printf '%s\n' 262144 ;;
    *) printf '%s\n' "$value" ;;
  esac
}

nfcx_cpu_count() {
  local value
  value="$(getconf _NPROCESSORS_ONLN 2>/dev/null || true)"
  value="${value%$'\r'}"
  case "$value" in
    ''|*[!0-9]*|0)
      value="${NUMBER_OF_PROCESSORS:-}"
      ;;
  esac
  case "$value" in
    ''|*[!0-9]*|0)
      value="$(sysctl -n hw.ncpu 2>/dev/null || true)"
      value="${value%$'\r'}"
      ;;
  esac
  case "$value" in
    ''|*[!0-9]*|0) printf '%s\n' 2 ;;
    *) printf '%s\n' "$value" ;;
  esac
}

nfcx_strip_release_binary() {
  local platform="$1" binary="$2"
  case "$platform" in
    darwin-*) strip -S "$binary" ;;
    linux-*|windows-*) strip --strip-unneeded "$binary" ;;
  esac
}

nfcx_verify_no_build_path() {
  local binary="$1" repository_root="$2"
  if strings "$binary" | grep -Fq "$repository_root"; then
    echo "release binary contains repository path: $binary" >&2
    return 1
  fi
}

nfcx_add_runtime_rpath() {
  local platform="$1" binary="$2"
  case "$platform" in
    darwin-*) install_name_tool -add_rpath '@executable_path' "$binary" ;;
    linux-*) patchelf --set-rpath '$ORIGIN' "$binary" ;;
    windows-*) ;;
  esac
}

nfcx_verify_libnfc_dependency() {
  local platform="$1" binary="$2"
  case "$platform" in
    darwin-*)
      otool -L "$binary" | grep -Eq '@rpath/libnfc\.[0-9]+\.dylib'
      otool -l "$binary" | grep -A3 LC_RPATH | grep -Fq '@executable_path'
      ;;
    linux-*)
      readelf -d "$binary" | grep -Eq 'NEEDED.*libnfc\.so\.[0-9]+'
      patchelf --print-rpath "$binary" | grep -Fxq '$ORIGIN'
      ;;
    windows-*)
      objdump -p "$binary" | grep -Eiq 'DLL Name: .*libnfc.*\.dll'
      ;;
  esac
}
