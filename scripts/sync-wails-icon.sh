#!/usr/bin/env bash
# sync-wails-icon.sh — Copy packaging/logo-svp1.png into Wails build assets.
#
# Wails embeds build/appicon.png into Windows .exe (via build/windows/icon.ico) and
# other platform bundles. macOS release icons also come from packaging/macos/AppIcon.icns,
# but wails build should still see the same source logo for consistency.
#
# Usage:
#   ./scripts/sync-wails-icon.sh

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC="${SRC:-$ROOT/packaging/logo-svp1.png}"
WAILS_BUILD="${WAILS_BUILD:-$ROOT/cmd/svpchain-gui/build}"
DST="$WAILS_BUILD/appicon.png"

if [[ ! -f "$SRC" ]]; then
	echo "sync-wails-icon.sh: source image not found: $SRC" >&2
	exit 1
fi

mkdir -p "$WAILS_BUILD"
cp "$SRC" "$DST"

# Force Wails to regenerate platform icons from the updated PNG.
rm -rf "$WAILS_BUILD/windows" "$WAILS_BUILD/darwin"

echo "Synced Wails app icon: $DST"
