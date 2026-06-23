#!/usr/bin/env bash
# package-macos-app.sh — Build the double-clickable macOS app "svpchain agent.app"
#
# Bundles svpchain-gui and svpchain-mcp into Contents/MacOS/
# so the GUI can auto-detect the MCP signer binary path.
#
# Usage:
#   ./scripts/package-macos-app.sh
#   VERSION=1.0.0 ./scripts/package-macos-app.sh
#   SIGN_IDENTITY="Developer ID Application: …" ./scripts/package-macos-app.sh
#
# Output:
#   build/svpchain agent.app
#   build/svpchain-agent-<version>-macos.dmg (drag .app to Applications)

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

APP_NAME="svpchain agent"
BUNDLE_ID="${BUNDLE_ID:-com.svpchain.agent-gui}"
BUILDDIR="${BUILDDIR:-$ROOT/build}"
APP_PATH="${APP_PATH:-$BUILDDIR/$APP_NAME.app}"
RELEASE_STEM="svpchain-agent"

if [[ "$(uname -s)" != "Darwin" ]]; then
	echo "package-macos-app.sh: macOS only" >&2
	exit 1
fi

resolve_version() {
	if [[ -n "${VERSION:-}" ]]; then
		echo "${VERSION#v}"
		return
	fi
	local tag
	if tag="$(git describe --tags --exact-match 2>/dev/null)"; then
		echo "${tag#v}"
		return
	fi
	echo "0.1.0-dev"
}

VERSION="$(resolve_version)"
DMG_PATH="$BUILDDIR/${RELEASE_STEM}-${VERSION}-macos.dmg"

echo "==> Packaging $APP_NAME $VERSION"
rm -rf "$BUILDDIR"

mkdir -p "$BUILDDIR"

echo "==> Locating wails CLI"
WAILS="${WAILS:-}"
if [[ -z "$WAILS" ]]; then
	if command -v wails >/dev/null 2>&1; then
		WAILS="$(command -v wails)"
	elif [[ -x "$(go env GOPATH)/bin/wails" ]]; then
		WAILS="$(go env GOPATH)/bin/wails"
	else
		echo "wails CLI not found; install with: go install github.com/wailsapp/wails/v2/cmd/wails@latest" >&2
		exit 1
	fi
fi

echo "==> Building MCP signer binary (CGO enabled)"
export CGO_ENABLED=1
go build -mod=readonly -trimpath -o "$BUILDDIR/svpchain-mcp" ./cmd/svpchain-mcp

echo "==> Building GUI with wails (frontend + bindings + binary)"
GUI_LDFLAGS="-X github.com/svpchain/svpchain-agent/internal/desktop.Version=${VERSION}"
(
	cd "$ROOT/cmd/svpchain-gui"
	echo "Build started, please wait..."
	"$WAILS" build -clean -trimpath -ldflags "$GUI_LDFLAGS"
)
cp "$ROOT/cmd/svpchain-gui/build/bin/svpchain-gui.app/Contents/MacOS/svpchain-gui" "$BUILDDIR/svpchain-gui"

echo "==> Assembling .app bundle"
rm -rf "$APP_PATH"
mkdir -p "$APP_PATH/Contents/MacOS" "$APP_PATH/Contents/Resources"

cp "$BUILDDIR/svpchain-gui" "$APP_PATH/Contents/MacOS/"
cp "$BUILDDIR/svpchain-mcp" "$APP_PATH/Contents/MacOS/"
chmod +x "$APP_PATH/Contents/MacOS/svpchain-gui" \
	"$APP_PATH/Contents/MacOS/svpchain-mcp"

if [[ -f "$ROOT/packaging/macos/AppIcon.icns" ]]; then
	cp "$ROOT/packaging/macos/AppIcon.icns" "$APP_PATH/Contents/Resources/AppIcon.icns"
fi

for lproj in en.lproj zh-Hans.lproj; do
	if [[ -d "$ROOT/packaging/macos/$lproj" ]]; then
		cp -R "$ROOT/packaging/macos/$lproj" "$APP_PATH/Contents/Resources/"
	fi
done

sed \
	-e "s/@VERSION@/$VERSION/g" \
	-e "s/@BUNDLE_ID@/$BUNDLE_ID/g" \
	"$ROOT/packaging/macos/Info.plist.in" > "$APP_PATH/Contents/Info.plist"

if [[ -f "$APP_PATH/Contents/Resources/AppIcon.icns" ]]; then
	/usr/libexec/PlistBuddy -c 'Add :CFBundleIconFile string AppIcon.icns' "$APP_PATH/Contents/Info.plist" 2>/dev/null \
		|| /usr/libexec/PlistBuddy -c 'Set :CFBundleIconFile AppIcon.icns' "$APP_PATH/Contents/Info.plist"
fi

if command -v codesign >/dev/null 2>&1; then
	echo "==> Code signing"
	sign_args=(--force --deep)
	if [[ -n "${SIGN_IDENTITY:-}" ]]; then
		sign_args+=(--options runtime --sign "$SIGN_IDENTITY")
		if [[ -f "$ROOT/packaging/macos/entitlements.plist" ]]; then
			sign_args+=(--entitlements "$ROOT/packaging/macos/entitlements.plist")
		fi
	else
		# CI / local ad-hoc: skip hardened runtime or double-click launch may fail.
		sign_args+=(--sign -)
	fi
	codesign "${sign_args[@]}" "$APP_PATH/Contents/MacOS/svpchain-mcp"
	codesign "${sign_args[@]}" "$APP_PATH/Contents/MacOS/svpchain-gui"
	codesign "${sign_args[@]}" "$APP_PATH"
	codesign --verify --deep --strict "$APP_PATH"
fi

echo "==> Creating DMG"
rm -f "$DMG_PATH"
RELEASE_DIR="$BUILDDIR/macos-release-$VERSION"
rm -rf "$RELEASE_DIR"
mkdir -p "$RELEASE_DIR"
cp -R "$APP_PATH" "$RELEASE_DIR/"
cp "$ROOT/packaging/macos/运行前先阅读.txt" "$RELEASE_DIR/"
cp "$ROOT/packaging/macos/READ-BEFORE-RUN.txt" "$RELEASE_DIR/"
ln -s /Applications "$RELEASE_DIR/Applications"
hdiutil create \
	-volname "$APP_NAME" \
	-srcfolder "$RELEASE_DIR" \
	-ov \
	-format UDZO \
	"$DMG_PATH"
rm -rf "$RELEASE_DIR"

echo ""
echo "Done."
echo "  App:  $APP_PATH"
echo "  DMG:  $DMG_PATH"
echo ""
echo "Open with:  open \"$APP_PATH\""
printf 'APP_DMG=%q\n' "$DMG_PATH"
printf 'APP_MACOS_BIN=%q\n' "$APP_PATH/Contents/MacOS"
