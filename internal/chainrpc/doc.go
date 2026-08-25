// Package chainrpc looks up committed transactions on a CometBFT / Tendermint
// RPC endpoint (GET /tx?hash=0x…). This is the run-log tx_checks / intent_checks
// path. It is unrelated to the Agent Market URL, which only serves agent search.
package chainrpc
