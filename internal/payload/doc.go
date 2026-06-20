// Package payload defines wire types exchanged between the remote MCP server
// (builds unsigned transactions and broadcasts signed ones) and the local MCP client/signer.
//
// TxPayload is returned by build_* tools; SignedTx is input to broadcast_signed_tx.
// sign_bytes.go builds SIGN_MODE_DIRECT sign bytes from TxBody + AuthInfo + chain_id + account_number —
// matching the on-chain path; actual signing happens on the client.
//
// This package intentionally has no I/O so the local signer binary can be imported without chain or HTTP deps.
package payload
