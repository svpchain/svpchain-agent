// Package signer turns TxPayload values from remote MCP server build_* tools into
// SignedTx values for broadcast_signed_tx. Handles eth_secp256k1 + SIGN_MODE_DIRECT
// signing and cross-checks (signer address matches loaded key, payload version supported).
//
// The stdio MCP service in cmd/svpchain-mcp consumes this package and exposes
// sign_transaction, sign_evm_transaction, sign_typed_data, sign_challenge, and whoami.
//
// init() sets svp bech32 prefixes so DeriveAddress and signer-address cross-checks stringify
// sdk.AccAddress the same way as the chain. Import this package — callers need not blank-import internal/config.
package signer
