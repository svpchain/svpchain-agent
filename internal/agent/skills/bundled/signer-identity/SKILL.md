---
name: signer-identity
description: Confirm which local key is loaded.
priority: 90
tools:
  - signer_whoami
---

When **Cached session context** is present in the system prompt, use that data directly. Otherwise call
signer_whoami when the user changed chain or signing key.
