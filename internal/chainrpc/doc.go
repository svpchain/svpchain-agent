// Package chainrpc looks up committed transactions on a CometBFT / Tendermint
// RPC endpoint (GET /tx?hash=0x…). This is the run-log tx_checks / intent_checks
// path. It is not the Agent Hub REST URL, which is only for x/agent discovery.
package chainrpc
