#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)"
DESTINATION="${1:?license destination is required}"
INDEX="$DESTINATION/THIRD_PARTY-GO.txt"
MODULES_FILE="$(mktemp "${TMPDIR:-/tmp}/nfcx-go-modules.XXXXXX")"
trap 'rm -f "$MODULES_FILE"' EXIT

mkdir -p "$DESTINATION"
(
  cd "$REPO_ROOT"
  go list -deps -tags 'desktop,production' -f '{{with .Module}}{{if not .Main}}{{.Path}}|{{.Version}}|{{.Dir}}{{end}}{{end}}' . ./app ./internal/desktop | sort -u > "$MODULES_FILE"
)
: > "$INDEX"
while IFS='|' read -r module version directory; do
  [[ -n "$module" ]] || continue
  safe_name="$(printf '%s@%s' "$module" "$version" | tr '/:' '__')"
  module_destination="$DESTINATION/$safe_name"
  mkdir -p "$module_destination"
  found=0
  for name in LICENSE LICENSE.txt LICENSE.md COPYING COPYING.txt NOTICE NOTICE.txt; do
    if [[ -f "$directory/$name" ]]; then
      install -m 0644 "$directory/$name" "$module_destination/$name"
      found=1
    fi
  done
  if [[ "$found" -ne 1 ]]; then
    echo "release licenses: no license file found for $module $version at $directory" >&2
    exit 1
  fi
  printf '%s %s https://%s\n' "$module" "$version" "$module" >> "$INDEX"
done < "$MODULES_FILE"
