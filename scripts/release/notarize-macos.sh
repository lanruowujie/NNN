#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)"
VERSION="${1:?NFCX version is required}"
PROFILE="${2:?notarytool keychain profile is required}"
APP="$REPO_ROOT/build/bin/NFCX.app"
DMG="$REPO_ROOT/dist/NFCX-$VERSION-darwin-arm64.dmg"

[[ -f "$DMG" ]] || { echo "DMG is missing: $DMG" >&2; exit 1; }
xcrun notarytool submit "$DMG" --keychain-profile "$PROFILE" --wait
xcrun stapler staple "$DMG"
xcrun stapler validate "$DMG"
spctl --assess --type open --context context:primary-signature --verbose=2 "$DMG"
codesign --verify --deep --strict --verbose=2 "$APP"
