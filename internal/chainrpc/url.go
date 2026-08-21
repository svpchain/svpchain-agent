package chainrpc

import "strings"

// TestnetURL is the CometBFT RPC for svpchain testnet.
const TestnetURL = "https://rpc-testnet.svpchain.org"

// URLForChain returns the CometBFT RPC base for a Cosmos chain id.
// Unknown chains return "" so tx lookup is skipped rather than hitting the
// wrong network.
func URLForChain(chainID string) string {
	id := strings.ToLower(strings.TrimSpace(chainID))
	id = strings.ReplaceAll(id, "_", "-")
	switch id {
	case "svp-2517-1":
		return TestnetURL
	default:
		return ""
	}
}

// QueryHash formats a hash the way CometBFT /tx?hash= expects: 0x + lowercase.
func QueryHash(hash string) string {
	h := strings.TrimSpace(hash)
	h = strings.TrimPrefix(strings.ToLower(h), "0x")
	if h == "" {
		return ""
	}
	return "0x" + h
}
