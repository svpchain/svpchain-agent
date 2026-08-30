BUILDDIR ?= build
BINARY   := svpchain-mcp
GUIBIN   := svpchain-gui

.PHONY: build build-gui build-all package-macos-app package-windows-app build-macos-icon install test tidy clean clean-build

# Prebuilt binaries for end-user distribution. Each platform builds natively on its runner.
build:
	mkdir -p $(BUILDDIR)
	CGO_ENABLED=1 go build -mod=readonly -trimpath -o $(BUILDDIR)/$(BINARY) ./cmd/svpchain-mcp

# Graphical setup tool (Wails: Vue frontend + Go). macOS uses the system WebKit;
# Linux needs GTK3 + WebKit2GTK dev packages. Requires the wails CLI and Node:
#   go install github.com/wailsapp/wails/v2/cmd/wails@latest
build-gui:
	cd cmd/svpchain-gui && wails build -clean -trimpath
	$(MAKE) patch-wails-macos-privacy
	$(MAKE) restore-dist-placeholder

# Wails' generated Info.plist omits microphone / speech-recognition usage
# strings; without them macOS denies SFSpeechRecognizer with no prompt.
.PHONY: patch-wails-macos-privacy
patch-wails-macos-privacy:
	@./scripts/macos-add-privacy-keys.sh \
		cmd/svpchain-gui/build/bin/svpchain-gui.app/Contents/Info.plist
	@if command -v codesign >/dev/null 2>&1 && \
		[ -d cmd/svpchain-gui/build/bin/svpchain-gui.app ]; then \
		codesign --force --deep --sign - cmd/svpchain-gui/build/bin/svpchain-gui.app; \
		codesign --verify --deep --strict cmd/svpchain-gui/build/bin/svpchain-gui.app; \
	fi

# Vite empties frontend/dist on every build, which deletes the tracked
# placeholder that lets `go build` work without the frontend toolchain — so a
# GUI build would otherwise leave the working tree dirty. Restoring it is
# cheaper than dropping emptyOutDir, which would let stale hashed assets pile
# up in the embedded bundle.
.PHONY: restore-dist-placeholder
restore-dist-placeholder:
	@touch cmd/svpchain-gui/frontend/dist/.gitkeep

build-all: build build-gui

# macOS only: build a double-clickable .app bundle and DMG installer.
# Cleans the build directory first for a reproducible from-scratch package.
package-macos-app: clean
	./scripts/package-macos-app.sh

# Windows only: build release folder and zip archive.
package-windows-app:
	pwsh ./scripts/package-windows.ps1

# macOS only: regenerate AppIcon.icns from packaging/logo-svp1.png.
build-macos-icon:
	./scripts/build-macos-icon.sh

install:
	go install -mod=readonly ./cmd/svpchain-mcp

test:
	go test ./...

tidy:
	go mod tidy

# Remove artifacts under build/ (svpchain-mcp, .app, DMG, release zips, etc.).
# Alias: clean-build
clean clean-build:
	rm -rf "$(BUILDDIR)"
	@mkdir -p "$(BUILDDIR)"
	@echo "cleared $(BUILDDIR)/"
