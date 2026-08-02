---
name: signer-identity
description: Confirm which local key is loaded and remote tenant policy after auth.
priority: 90
tools:
  - signer_whoami
  - whoami
---

When **Cached session context** is present in the system prompt, use that data directly — do not call signer_whoami or whoami unless the user changed chain, signing key, or remote MCP endpoint. Otherwise call signer_whoami for the local key and whoami for remote tenant policy after auth.
