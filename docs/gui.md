# Graphical app (svpchain-gui)

**English** | [简体中文](gui.zh-CN.md) · [← README](../README.md)

The GUI covers key management, MCP export, security policy, and the built-in assistant.

## Tabs

| Tab | Purpose |
|-----|---------|
| **Assistant** | Natural-language chat that drives build → sign → broadcast. Pick a chain id, enter a command, watch step-by-step progress. Conversations are **multi-turn and persisted** — switch between saved conversations or start a new one from the header (see [Assistant memory & context](assistant-context.md)). |
| **Keys / Import** | Import, list, and delete signing keys; view derived `svp1…` and `0x` addresses per chain. |
| **Security** | Manage a **transfer whitelist** (chain id + Cosmos or EVM address, optional alias). The GUI assistant requires at least one entry before it will transfer; the standalone signer treats an empty list as unrestricted (see [Transfer whitelist](security-whitelist.md)). |
| **MCP** | Generate stdio MCP client JSON for Cursor and other clients; auto-detect the bundled `svpchain-mcp` binary. |
| **Settings** | Collapsible sections — **Basic** (language, default chain id, tool-step display, run logging), **LLM** (API key, base URL, model, context window, remote MCP URL), **Assistant Skills** (enable/disable prompt modules). |
| **About** | Version and trust-model summary. |

## Assistant & LLM settings

The assistant speaks **OpenAI-compatible** APIs (default base `https://api.deepseek.com`, model `deepseek-v4-flash`) or native **Anthropic** (`/v1/messages` when the provider/base URL resolves to Anthropic). Responses stream into the chat UI. Configure API key, base URL, model, and remote MCP endpoint under **Settings → LLM**, then save. The remote MCP endpoint defaults to `https://indexer.svpchain.com/mcp`.

The app supports **English and Chinese** (Settings → Basic; persisted). Override first-launch detection with `SVPCHAIN_AGENT_LANG=zh|en`.

## Assistant skills

The assistant system prompt is assembled from modular **skills** (`internal/agent/skills/bundled/*/SKILL.md`), not a single hard-coded string. Each skill covers one workflow (on-chain build/sign/broadcast, x402 payments, bank send to `0x`, ERC-20/721, A2A delegation, etc.).

- **Bundled skills** are embedded in the binary. A skill may keep bulky detail (output templates, error catalogs) in `references/*.md` next to its `SKILL.md`; the assistant loads those on demand with the local `read_skill_reference` tool instead of carrying them in every system prompt.
- **User skills** — optional overrides in `<config-dir>/com.svpchain.agent-gui/skills/<name>/SKILL.md` (alongside `prefs.json`; e.g. `~/Library/Application Support/...` on macOS, `%AppData%` on Windows).
- **Settings → Assistant Skills** — toggle skills on/off (saved as `disabled_skills` in `prefs.json`). The `base` skill is locked on. Disabled skills are omitted from the system prompt; available MCP tools still control which tool-bound skills are injected at runtime.
