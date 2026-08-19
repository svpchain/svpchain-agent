---
name: signer-identity
description: Confirm which local key is loaded for the selected chain.
priority: 90
tools:
  - signer_whoami
---

Use cached signer identity from the session prompt when present. Otherwise call `signer_whoami` before any signing or delegation that depends on the active account. Never request private key material.
