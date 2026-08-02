---
description: Audit the current diff against the svpchain signing trust boundary.
allowed-tools: Bash(git diff:*), Bash(git status:*), Task
---

Review the current changes for trust-boundary safety.

1. Gather the diff: `git status` then `git diff` and `git diff --staged`. If both are empty,
   use `git diff main...HEAD`.
2. If nothing touches `internal/signer`, `internal/payload`, `internal/whitelist`,
   `internal/mcp`, `internal/agent/whitelist_gate.go`, or `internal/agent/chainid.go`, say
   the diff is clear of the trust boundary and stop.
3. Otherwise delegate to the `trust-boundary-auditor` subagent for a full read-only audit,
   then relay its verdict, findings, and any missing-test notes.

$ARGUMENTS
