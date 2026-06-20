BUILDDIR ?= build
BINARY   := svpchain-mcp
GUIBIN   := svpchain-gui

.PHONY: build build-gui build-all package-macos-app build-macos-icon install test tidy clean

# Prebuilt binaries for end-user distribution. Windows is unsupported (CGO/libsecp256k1 toolchain);
# macOS and Linux build natively.
build:
ifeq ($(OS),Windows_NT)
	exit 1
else
	mkdir -p $(BUILDDIR)
	go build -mod=readonly -o $(BUILDDIR)/$(BINARY) ./cmd/svpchain-mcp
endif

# Graphical setup tool (Wails: Vue frontend + Go). macOS uses the system WebKit;
# Linux needs GTK3 + WebKit2GTK dev packages. Requires the wails CLI and Node:
#   go install github.com/wailsapp/wails/v2/cmd/wails@latest
build-gui:
ifeq ($(OS),Windows_NT)
	exit 1
else
	cd cmd/svpchain-gui && wails build -clean -trimpath
endif

build-all: build build-gui

# macOS only: build a double-clickable .app bundle and zip archive.
# Cleans the build directory first for a reproducible from-scratch package.
package-macos-app: clean
	./scripts/package-macos-app.sh

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
