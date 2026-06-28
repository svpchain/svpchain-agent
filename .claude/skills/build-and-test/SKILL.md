---
name: build-and-test
description: How to build, test, and run the svpchain-agent (Go + Wails). Use when building the stdio signer or GUI, running Go tests, formatting, or working on the Vue frontend.
---

# Building and testing svpchain-agent

## CGO is mandatory

Every build/test needs `CGO_ENABLED=1` — `eth_secp256k1` links libsecp256k1. A pure-Go
build will fail to sign.

## Make targets

```sh
make build          # → build/svpchain-mcp (stdio signer). CGO_ENABLED=1 set for you.
make build-gui      # → Wails GUI; needs `wails` CLI + Node
make build-all      # both
make test           # go test ./...
make tidy           # go mod tidy
make install        # go install ./cmd/svpchain-mcp
make package-macos-app    # .app + DMG (macOS only)
make package-windows-app  # zip (Windows, needs pwsh 7+)
```

## Run a single test

```sh
go test ./internal/signer/ -run TestName -v
```

Most security-critical logic has tests in `internal/signer/*_test.go`,
`internal/whitelist/enforce_test.go`, and `internal/agent/whitelist_gate_test.go`.
When you change those areas, run the matching package directly first, then `make test`.

## Format and vet before finishing

```sh
gofmt -w <edited files>
go vet ./...
```

`goimports` is not assumed present; `gofmt` is. The project's PostToolUse hook
(`.claude/hooks/go-postedit.sh`) runs `gofmt -w` on saved `.go` files automatically, but
`go vet` is on you.

## Frontend (Wails GUI)

From `cmd/svpchain-gui/frontend/`:

```sh
npm install
npm run dev     # Vite dev server
npm run build   # production bundle (embedded by Wails)
```

The Wails CLI for `make build-gui`:
`go install github.com/wailsapp/wails/v2/cmd/wails@latest`. `wailsjs/` holds generated
bindings — don't hand-edit them.
