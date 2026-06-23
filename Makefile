BUILDDIR ?= build
BINARY   := svpchain-mcp
GUIBIN   := svpchain-gui

.PHONY: build build-gui build-all package-macos-app package-windows-app build-macos-icon install test tidy clean

# Prebuilt binaries for end-user distribution. Each platform builds natively on its runner.
build:
	mkdir -p $(BUILDDIR)
	CGO_ENABLED=1 go build -mod=readonly -trimpath -o $(BUILDDIR)/$(BINARY) ./cmd/svpchain-mcp

# Graphical setup tool (Wails: Vue frontend + Go). macOS uses the system WebKit;
# Linux needs GTK3 + WebKit2GTK dev packages. Requires the wails CLI and Node:
#   go install github.com/wailsapp/wails/v2/cmd/wails@latest
build-gui:
	cd cmd/svpchain-gui && wails build -clean -trimpath

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

clean:
	rm -rf $(BUILDDIR)
