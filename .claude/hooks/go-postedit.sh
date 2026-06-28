#!/usr/bin/env bash
# PostToolUse hook for svpchain-agent (.claude/).
# - gofmt -w on saved .go files
# - reminds about trust-boundary invariants when such a file is edited
# Always exits 0 so it can never block an edit.
set -u

input="$(cat 2>/dev/null || true)"

# Extract tool_input.file_path. Prefer jq; fall back to a sed grab.
if command -v jq >/dev/null 2>&1; then
  file="$(printf '%s' "$input" | jq -r '.tool_input.file_path // empty' 2>/dev/null)"
else
  file="$(printf '%s' "$input" \
    | sed -n 's/.*"file_path"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' \
    | head -n1)"
fi

[ -n "${file:-}" ] || exit 0

# Format Go files in place (best-effort).
case "$file" in
  *.go)
    if command -v gofmt >/dev/null 2>&1 && [ -f "$file" ]; then
      gofmt -w "$file" >/dev/null 2>&1 || true
    fi
    ;;
esac

# Trust-boundary reminder.
case "$file" in
  */internal/signer/*|*/internal/payload/*|*/internal/whitelist/*|*/internal/mcp/*|*/internal/agent/whitelist_gate.go|*/internal/agent/chainid.go)
    msg="Trust-boundary file edited ($file). Preserve the chain-id and signer_address cross-checks, the svpchain-mcp-auth-v1: challenge guard, and the two-layer whitelist semantics (gate: empty=refuse all; signer: empty=unrestricted). Keep internal/payload I/O-free. Update the matching _test.go and run go vet. Consider /trust-check."
    if command -v jq >/dev/null 2>&1; then
      jq -nc --arg m "$msg" '{hookSpecificOutput:{hookEventName:"PostToolUse",additionalContext:$m}}'
    else
      esc="${msg//\\/\\\\}"; esc="${esc//\"/\\\"}"
      printf '{"hookSpecificOutput":{"hookEventName":"PostToolUse","additionalContext":"%s"}}\n' "$esc"
    fi
    ;;
esac

exit 0
