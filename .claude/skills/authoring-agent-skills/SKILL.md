---
name: authoring-agent-skills
description: How the shipped svpchain assistant's system prompt is assembled from modular bundled SKILL.md files, and how to add or edit one. Use when adding a new capability skill under internal/agent/skills/bundled or changing the assistant's prompt behavior.
---

# Authoring bundled agent skills

The runtime assistant's system prompt is **assembled from modular skills**, not hardcoded
Go strings. Each skill is a directory under `internal/agent/skills/bundled/<name>/` with a
`SKILL.md`.

> Note: these are the **shipped product's** LLM skills, distinct from the Claude Code
> developer skills under `.claude/skills/`. Both use the same SKILL.md format.

## How assembly works

- `base` is **always on** and `locked: true` — it carries the role, the trust model, and
  the non-negotiable red lines. Don't move that content out of `base`.
- Other skills gate on **available tools** and on `disabled_skills` from `prefs.json`. A
  skill describing `build_swap` should only contribute when the swap tools exist.
- The loop lives in `internal/agent/runner.go` (`Run`, `dispatchTool`). Local-only tools
  (`signer_whoami`, `evm_to_bech32`, `http_fetch`, `x402_*`, `a2a_send_message`) are
  defined in `internal/agent/` and are **not** exposed by the stdio server.

## Existing skills (for reference / tone)

```
base              response-style     signer-identity
onchain-workflow  bank-send-evm      erc-tokens
a2a               x402
```

Read a couple before writing a new one to match voice and structure.

## SKILL.md frontmatter

```markdown
---
name: <kebab-case, matches the directory>
description: <one line — what it covers and when it applies>
priority: <optional int; lower loads earlier>
locked: <optional bool; true only for base>
---

# Body in Markdown — concise, imperative, no hardcoded secrets.
```

## To add a skill

1. Create `internal/agent/skills/bundled/<name>/SKILL.md`.
2. Gate it on the tools it documents — don't emit guidance for tools that aren't present.
3. If it touches transfers/signing, cross-reference the trust rules in `base`; never
   restate red lines loosely in a way that could contradict `base`.
4. `make test` — skill assembly is covered by the agent package tests.

## Don'ts

- Don't hardcode prompt text in Go when a skill file will do.
- Don't duplicate `base`'s red lines; reference them.
- Don't add a skill that loosens a refusal or the build → sign → broadcast flow.
