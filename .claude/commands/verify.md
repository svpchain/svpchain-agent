---
description: Build, test, vet, and format-check the svpchain-agent from a clean checkout.
allowed-tools: Bash(make build), Bash(make test), Bash(go vet ./...), Bash(gofmt -l .), Bash(git status:*)
---

Verify the repo builds and passes checks. Run these and report a concise pass/fail summary
(do not fix anything unless I ask):

1. `make build` — CGO stdio signer build.
2. `make test` — full `go test ./...`.
3. `gofmt -l .` — any output means unformatted files; list them.
4. `go vet ./...` — report any diagnostics.

Summarize as a short table: step → pass/fail (+ first failing lines if any). If everything
passes, say so plainly. Do not run packaging or GUI builds unless asked.
