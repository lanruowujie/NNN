#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)"
APP_PATH="${1:-$REPO_ROOT/build/bin/NFCX.app}"
VERSION="${2:?NFCX version is required}"
COMMIT="${3:?Git commit is required}"
IDENTITY="${4:--}"
RUNTIME="$APP_PATH/Contents/Resources/runtime/darwin-arm64"

[[ -d "$RUNTIME" ]] || { echo "packaged macOS runtime is missing" >&2; exit 1; }
SIGN_ARGS=(--force --sign "$IDENTITY")
if [[ "$IDENTITY" != "-" ]]; then
  SIGN_ARGS+=(--options runtime --timestamp)
fi
for file in "$RUNTIME"/libnfc.[0-9]*.dylib "$RUNTIME"/mfoc "$RUNTIME"/mfcuk "$RUNTIME"/mfoc-hardnested "$RUNTIME"/nfc-mfsetuid; do
  [[ -f "$file" ]] && codesign "${SIGN_ARGS[@]}" "$file"
done
NFCX_PLATFORM=darwin-arm64 "$SCRIPT_DIR/generate-manifest.sh" "$VERSION" "$COMMIT"
codesign "${SIGN_ARGS[@]}" "$APP_PATH"
codesign --verify --deep --strict --verbose=2 "$APP_PATH"
otool -l "$APP_PATH/Contents/MacOS/NFCX" | grep -A3 LC_RPATH | grep -Fq '@executable_path/../Resources/runtime/darwin-arm64'
