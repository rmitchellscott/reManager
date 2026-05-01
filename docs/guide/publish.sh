#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
MANIFEST="$SCRIPT_DIR/guide-manifest.json"
RELEASES="$SCRIPT_DIR/releases"
EPUB="$SCRIPT_DIR/reManager User Guide.epub"

if [ $# -eq 0 ]; then
  echo "Usage: $0 <version> [version...]"
  echo ""
  echo "Publishes the current guide epub for the given version(s)."
  echo ""
  echo "Examples:"
  echo "  $0 1.4.0              # publish for a single version"
  echo "  $0 1.4.0 1.4.1 1.5.0  # publish for multiple versions"
  exit 1
fi

if [ ! -f "$EPUB" ]; then
  echo "Error: $EPUB not found. Run build.sh first."
  exit 1
fi

mkdir -p "$RELEASES"

SHA=$(sha256sum "$EPUB" | awk '{print $1}')

for VERSION in "$@"; do
  cp "$EPUB" "$RELEASES/${VERSION}.epub"
  jq --arg v "$VERSION" --arg h "$SHA" '.[$v] = {"sha256": $h}' \
    "$MANIFEST" > "$MANIFEST.tmp" && mv "$MANIFEST.tmp" "$MANIFEST"
  echo "Published v${VERSION} (sha256: ${SHA})"
done
