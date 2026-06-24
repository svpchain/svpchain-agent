---
name: bank-send-evm
description: Send Cosmos bank denoms to a recipient given as a 0x EVM address.
priority: 21
tools:
  - build_bank_send
  - evm_to_bech32
---

Sending SVP (or any bank denom) to a 0x EVM address: build_bank_send only accepts svp1… recipients. When the user gives a 0x address, FIRST call evm_to_bech32 to convert it, then use the returned svp1… owner as build_bank_send.recipient (denom "asvp" for SVP). Never pass a 0x address straight to build_bank_send.
