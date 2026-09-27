#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)"
# shellcheck source=../toolchain/platform.sh
source "$REPO_ROOT/scripts/toolchain/platform.sh"

PLATFORM="$(nfcx_platform)"
VERSION="${1:?NFCX version is required}"
COMMIT="${2:?Git commit is required}"
case "$PLATFORM" in
  darwin-arm64) ROOT="$REPO_ROOT/build/bin/NFCX.app/Contents/Resources/runtime/$PLATFORM" ;;
  linux-amd64) ROOT="$REPO_ROOT/build/bin/NFCX-linux-amd64/runtime/$PLATFORM" ;;
  windows-amd64) ROOT="$REPO_ROOT/build/bin/NFCX-windows-amd64" ;;
  *) echo "unsupported manifest platform $PLATFORM" >&2; exit 1 ;;
esac

[[ -d "$ROOT" ]] || { echo "packaged runtime is missing: $ROOT" >&2; exit 1; }
rm -rf "$ROOT/LICENSES/go"
"$SCRIPT_DIR/collect-go-licenses.sh" "$ROOT/LICENSES/go"
if [[ "$PLATFORM" == linux-* ]]; then
  install -m 0644 "$REPO_ROOT/third_party/appimage/type2-runtime-LICENSE.txt" "$ROOT/LICENSES/AppImage-type2-runtime-LICENSE.txt"
fi
(
  cd "$REPO_ROOT"
  go run ./cmd/nfcx-release manifest \
    -repository "$REPO_ROOT" -root "$ROOT" -output "$ROOT/manifest.json" \
    -platform "$PLATFORM" -version "$VERSION" -commit "$COMMIT"
  go run ./cmd/nfcx-release verify -root "$ROOT" -manifest "$ROOT/manifest.json"
)
