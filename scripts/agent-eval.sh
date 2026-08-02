#!/usr/bin/env bash
# Offline agent eval: guard/whitelist regression cases.
set -euo pipefail
cd "$(dirname "$0")/.."
go test ./internal/agent/eval/... ./internal/agent/runlog/... -count=1 "$@"
