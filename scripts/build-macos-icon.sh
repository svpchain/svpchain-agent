#!/usr/bin/env bash
# build-macos-icon.sh — Convert packaging/logo-svp1.png to AppIcon.icns
#
# Usage:
#   ./scripts/build-macos-icon.sh
#   SRC=path/to/logo.png OUT=packaging/macos/AppIcon.icns ./scripts/build-macos-icon.sh

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC="${SRC:-$ROOT/packaging/logo-svp1.png}"
OUT="${OUT:-$ROOT/packaging/macos/AppIcon.icns}"
ICONSET="${ICONSET:-$ROOT/build/AppIcon.iconset}"

if [[ "$(uname -s)" != "Darwin" ]]; then
	echo "build-macos-icon.sh: macOS only (requires sips and iconutil)" >&2
	exit 1
fi

if [[ ! -f "$SRC" ]]; then
	echo "build-macos-icon.sh: source image not found: $SRC" >&2
	exit 1
fi

make_icon() {
	local size="$1"
	local name="$2"
	sips -z "$size" "$size" "$SRC" --out "$ICONSET/$name" >/dev/null
}

echo "==> Generating iconset from $SRC"
rm -rf "$ICONSET"
mkdir -p "$ICONSET"

make_icon 16   icon_16x16.png
make_icon 32   icon_16x16@2x.png
make_icon 32   icon_32x32.png
make_icon 64   icon_32x32@2x.png
make_icon 128  icon_128x128.png
make_icon 256  icon_128x128@2x.png
make_icon 256  icon_256x256.png
make_icon 512  icon_256x256@2x.png
make_icon 512  icon_512x512.png
make_icon 1024 icon_512x512@2x.png

mkdir -p "$(dirname "$OUT")"
echo "==> Writing $OUT"
iconutil -c icns "$ICONSET" -o "$OUT"

echo "Done: $OUT"
