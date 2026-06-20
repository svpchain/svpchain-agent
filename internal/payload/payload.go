package payload

import (
	"time"
)

// CurrentVersion is the only supported wire version for TxPayload / SignedTx in v0.1.
// Increment when wire shapes change incompatibly.
const CurrentVersion = 1

// TxPayload is the wire envelope returned by build_* MCP tools and later passed with SignedTx to broadcast_signed_tx.
//
// The remote MCP server fills every field except the signature; the local signer signs precomputed SignBytes
// (SIGN_MODE_DIRECT) and returns the full TxPayload with BroadcastSignedTxArgs so the server can re-verify
// byte equality before broadcast.
//
// Cosmos uint64 fields are JSON-encoded as strings (Cosmos SDK convention) to avoid precision loss in JS-based MCP clients.
type TxPayload struct {
	Version int `json:"version"`

	// ClientID is the broadcast idempotency key (uuid), distinct from per-order Order.ClientId (uint32) in Summary.
	ClientID string `json:"client_id"`

	ChainID         string `json:"chain_id"`
	SignerAddress   string `json:"signer_address"`
	AccountNumber   string `json:"account_number"`
	Sequence        string `json:"sequence"`
	IsShortTermCLOB bool   `json:"is_short_term_clob"`

	// Encoded transaction parts as standard base64 strings; _b64 suffix marks the wire shape.
	// string instead of []byte: MCP SDK JSON Schema reflection maps []byte to
	// {type:"array", items:{type:"integer"}}, which does not match encoding/json's base64 string output
	// and fails SDK output validation. Encode/decode at boundaries with base64.StdEncoding.
	//
	// TxBodyBytesB64 is always present. AuthInfoBytesB64 and SignBytesB64 are precomputed by the server only when
	// tenant config includes the signer pubkey (v0.2+); when missing in v0.1 the local signer builds AuthInfo and
	// sign bytes from chain id, account number, sequence, and fee.
	TxBodyBytesB64   string `json:"tx_body_bytes_b64"`
	AuthInfoBytesB64 string `json:"auth_info_bytes_b64,omitempty"`
	SignBytesB64     string `json:"sign_bytes_b64,omitempty"`

	Fee     Fee     `json:"fee"`
	Summary Summary `json:"summary"`

	// ExpiresAt is the server-side payload TTL; broadcast_signed_tx rejects expired payloads to avoid
	// unexpected on-chain behavior from long pending sign-broadcast cycles.
	ExpiresAt time.Time `json:"expires_at"`
}

// Fee mirrors wire cosmos.tx.v1beta1.Fee. Short-term CLOB txs on svpchain are fee-free with empty Amount;
// other txs carry fees set by the remote MCP builder, written into AuthInfo.Fee by the signer. GasLimit is a fixed constant.
type Fee struct {
	GasLimit string `json:"gas_limit"`
	Amount   []Coin `json:"amount"`
}

// Coin is sdk.Coin in JSON form (amount as string preserves uint128/uint64 precision).
type Coin struct {
	Denom  string `json:"denom"`
	Amount string `json:"amount"`
}

// SubaccountRef is subaccounts.SubaccountId in JSON form
// (proto/dydxprotocol/subaccounts/subaccount.proto:11).
type SubaccountRef struct {
	Owner  string `json:"owner"`
	Number uint32 `json:"number"`
}

// Summary is a human-readable description of a build_* result. The remote server fills it from tool args for client display.
// On-chain content is authoritative in TxBodyBytesB64 — Summary is informational only and not re-validated at broadcast beyond basic sanity.
type Summary struct {
	ToolName   string        `json:"tool_name"`
	MsgTypeURL string        `json:"msg_type_url"`
	Subaccount SubaccountRef `json:"subaccount"`

	// Order-related fields (omitted for non-trading tools).
	Ticker        string `json:"ticker,omitempty"`
	Side          string `json:"side,omitempty"`
	SizeHuman     string `json:"size_human,omitempty"`
	PriceHuman    string `json:"price_human,omitempty"`
	NotionalUSD   string `json:"notional_usd,omitempty"`
	GoodTil       string `json:"good_til,omitempty"`
	ReduceOnly    bool   `json:"reduce_only,omitempty"`
	OrderClientID uint32 `json:"order_client_id,omitempty"`

	// Transfer fields (v0.2; reserved for stable wire shape).
	AssetID        uint32 `json:"asset_id,omitempty"`
	AmountHuman    string `json:"amount_human,omitempty"`
	RecipientOwner string `json:"recipient_owner,omitempty"`
	RecipientNum   uint32 `json:"recipient_subaccount,omitempty"`
}

// SignedTx is what MCP clients pass to broadcast_signed_tx. The remote server base64-decodes TxRawBytesB64,
// parses the TxRaw proto, and verifies the signer address matches the tenant owner before broadcast.
//
// All three fields are standard base64 strings; see TxPayload.TxBodyBytesB64 for why string is used instead of []byte.
type SignedTx struct {
	TxRawBytesB64 string `json:"tx_raw_bytes_b64"`
	SignatureB64  string `json:"signature_b64"`
	PubKeyB64     string `json:"pub_key_b64"`
}

// BroadcastResult is returned by broadcast_signed_tx after a successful BroadcastSync.
// Code 0 means accepted into the mempool; non-zero is CheckTx rejection with RawLog explaining why.
type BroadcastResult struct {
	TxHash string `json:"tx_hash"` // hex
	Code   uint32 `json:"code"`
	RawLog string `json:"raw_log,omitempty"`
}
