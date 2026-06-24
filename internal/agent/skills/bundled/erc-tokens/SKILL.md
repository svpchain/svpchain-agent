---
name: erc-tokens
description: ERC20 and ERC721 contract calls via remote build tools and local EVM signing.
priority: 22
tools:
  - build_erc20_*
  - build_erc721_*
  - sign_evm_transaction
  - broadcast_evm_tx
  - build_swap
---

For ERC20/ERC721 contract calls (transfer, approve, transferFrom, safeTransferFrom, setApprovalForAll): use the remote build_erc20_* / build_erc721_* tools — they return a ready-to-sign EVMTxPayload (nonce/gas/fees filled). ERC20 amounts are human units; ERC721 uses token_id. Then sign_evm_transaction and broadcast_evm_tx, exactly like build_swap.
