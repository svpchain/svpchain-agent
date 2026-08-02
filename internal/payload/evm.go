package payload

// EVM transaction signing wire types. These mirror the Cosmos TxPayload /
// SignedTx pair (payload.go) but carry a raw Ethereum transaction the signer
// builds, signs, and RLP-encodes for broadcast via eth_sendRawTransaction.
//
// Unlike the Cosmos path — where the remote MCP server pre-builds the TxBody
// proto bytes and the signer only computes sign-bytes — the EVM path passes
// the transaction as structured fields. The signer assembles the go-ethereum
// transaction object itself so it can keccak-hash and sign it, then returns
// the canonical raw hex.
//
// All numeric fields are JSON-encoded as decimal strings to preserve full
// precision for JS-based MCP clients (matching the uint64-as-string
// convention in TxPayload): nonce/gas are uint64-range, but value and the
// gas-price fields are wei amounts that overflow JS doubles.

// EVMTxType discriminates the two Ethereum transaction formats the signer
// supports. An empty TxType is inferred: max_fee_per_gas set => eip1559,
// otherwise legacy.
const (
	EVMTxTypeEIP1559 = "eip1559"
	EVMTxTypeLegacy  = "legacy"
)

// EvmTxPayload is the on-wire envelope the remote MCP server's EVM build_*
// tools produce and the local signer turns into a SignedEvmTx. The server
// fills every field; the signer never invents transaction parameters.
type EvmTxPayload struct {
	Version int `json:"version"`

	// EVMChainID is the numeric EIP-155 chain id (decimal string). The signer
	// refuses to sign unless this matches the EVM chain id it was started
	// against — the EVM analog of TxPayload.ChainID's cross-chain replay guard.
	EVMChainID string `json:"evm_chain_id"`

	// SignerAddress is the 0x-checksummed sender. When non-empty the signer
	// cross-checks it against the key-derived address and refuses on mismatch
	// (mirror of TxPayload.SignerAddress); empty is tolerated for ad-hoc demos.
	SignerAddress string `json:"signer_address,omitempty"`

	// TxType selects the transaction format ("eip1559" | "legacy"). Empty is
	// inferred from which gas fields are populated.
	TxType string `json:"tx_type,omitempty"`

	Nonce string `json:"nonce"`

	// To is the 0x recipient. Empty means contract creation (nil To).
	To string `json:"to,omitempty"`

	// Value is the wei amount transferred (decimal string). Empty means zero.
	Value string `json:"value,omitempty"`

	// Gas is the gas limit (uint64 decimal string).
	Gas string `json:"gas"`

	// GasPrice is the legacy gas price in wei (decimal string). Used only for
	// EVMTxTypeLegacy.
	GasPrice string `json:"gas_price,omitempty"`

	// MaxFeePerGas / MaxPriorityFeePerGas are the EIP-1559 fee caps in wei
	// (decimal strings). Used only for EVMTxTypeEIP1559.
	MaxFeePerGas         string `json:"max_fee_per_gas,omitempty"`
	MaxPriorityFeePerGas string `json:"max_priority_fee_per_gas,omitempty"`

	// Data is the call data / contract init code as a 0x-hex string. Empty
	// means no data.
	Data string `json:"data,omitempty"`

	// Summary is informational, shown to the user for approval; the signed
	// bytes (the structured fields above) are authoritative.
	Summary EvmSummary `json:"summary"`
}

// EvmSummary is the human-readable description of an EVM build_* result,
// displayed for user approval. Informational only — never re-validated against
// the signed transaction.
type EvmSummary struct {
	ToolName    string `json:"tool_name"`
	Description string `json:"description,omitempty"`
}

// SignedEvmTx is what the signer returns for an EVM transaction. RawTxHex is
// the canonical RLP-encoded signed transaction ready for eth_sendRawTransaction;
// the v/r/s components are surfaced for callers that re-derive the hash or log
// the signature.
type SignedEvmTx struct {
	RawTxHex string `json:"raw_tx_hex"` // 0x-prefixed
	TxHash   string `json:"tx_hash"`    // 0x-prefixed
	V        string `json:"v"`
	R        string `json:"r"`
	S        string `json:"s"`
}
