package mcp

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/cosmos/evm/crypto/ethsecp256k1"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/svpchain/svpchain-mcp/internal/payload"
	"github.com/svpchain/svpchain-mcp/internal/signer"
)

// ChallengePrefix restricts sign_challenge input to the svpchain self-service auth flow.
// Messages without this prefix are rejected — the signer must not act as a generic message-signing oracle
// (attackers could trick users into signing arbitrary text and forge valid content on some chains).
const ChallengePrefix = "svpchain-mcp-auth-v1:"

// Handlers holds process state: loaded private key plus chain id boundaries for refused signatures.
// ChainID is the Cosmos chain id (string); EVMChainID is the numeric EIP-155 chain id used when signing
// raw Ethereum transactions with the same key. Both come from --chain-id at startup (EVMChainID derived from it,
// or set explicitly via --evm-chain-id).
type Handlers struct {
	Priv       *ethsecp256k1.PrivKey
	ChainID    string
	EVMChainID uint64
}

// --- sign_transaction ---

type SignTransactionInput struct {
	Payload payload.TxPayload `json:"payload" jsonschema:"the TxPayload produced by the remote MCP server's build_* tools"`
}

type SignTransactionOutput struct {
	SignedTx payload.SignedTx `json:"signed_tx"`
}

// SignTransaction signs a TxPayload; rejects when payload.chain_id does not match startup flags.
func (h *Handlers) SignTransaction(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in SignTransactionInput,
) (*mcp.CallToolResult, SignTransactionOutput, error) {
	if h.ChainID != "" && in.Payload.ChainID != h.ChainID {
		return nil, SignTransactionOutput{}, fmt.Errorf(
			"payload.chain_id %q does not match signer --chain-id %q",
			in.Payload.ChainID, h.ChainID,
		)
	}
	signed, err := signer.Sign(h.Priv, &in.Payload)
	if err != nil {
		return nil, SignTransactionOutput{}, err
	}
	return nil, SignTransactionOutput{SignedTx: *signed}, nil
}

// --- sign_evm_transaction ---

type SignEvmTransactionInput struct {
	Payload payload.EvmTxPayload `json:"payload" jsonschema:"the EvmTxPayload produced by the remote MCP server's EVM build_* tools"`
}

type SignEvmTransactionOutput struct {
	SignedTx payload.SignedEvmTx `json:"signed_tx"`
}

// SignEvmTransaction signs a raw Ethereum transaction (EIP-1559 or legacy)
// built from the structured fields in the payload, returning RLP-encoded hex
// for eth_sendRawTransaction. It enforces the same cross-chain replay guard as
// the Cosmos path, on the numeric EVM chain id: a signer bound to one chain
// refuses payloads targeting another.
func (h *Handlers) SignEvmTransaction(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in SignEvmTransactionInput,
) (*mcp.CallToolResult, SignEvmTransactionOutput, error) {
	// A zero EVM chain id means the signer couldn't determine one from
	// --chain-id and none was supplied via --evm-chain-id: EVM signing is
	// disabled rather than left to sign for an attacker-chosen chain.
	if h.EVMChainID == 0 {
		return nil, SignEvmTransactionOutput{}, fmt.Errorf(
			"EVM signing is not configured: start the signer with --evm-chain-id (or an evmos-style --chain-id)")
	}
	want := strconv.FormatUint(h.EVMChainID, 10)
	if in.Payload.EVMChainID != want {
		return nil, SignEvmTransactionOutput{}, fmt.Errorf(
			"payload.evm_chain_id %q does not match signer evm chain id %q",
			in.Payload.EVMChainID, want,
		)
	}
	signed, err := signer.SignEvm(h.Priv, &in.Payload)
	if err != nil {
		return nil, SignEvmTransactionOutput{}, err
	}
	return nil, SignEvmTransactionOutput{SignedTx: *signed}, nil
}

// --- sign_typed_data ---

type SignTypedDataInput struct {
	TypedData payload.EIP712TypedData `json:"typed_data" jsonschema:"EIP-712 typed data (eth_signTypedData_v4 shape) for x402 EIP-3009 TransferWithAuthorization"`
}

type SignTypedDataOutput struct {
	Signed payload.SignedTypedData `json:"signed"`
}

// SignTypedData signs EIP-712 typed data for x402 "exact" payments (EIP-3009 TransferWithAuthorization).
// Refuses other primaryType values and enforces domain.chainId + message.from cross-checks.
func (h *Handlers) SignTypedData(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in SignTypedDataInput,
) (*mcp.CallToolResult, SignTypedDataOutput, error) {
	signed, err := signer.SignTypedData(h.Priv, &in.TypedData, h.EVMChainID)
	if err != nil {
		return nil, SignTypedDataOutput{}, err
	}
	return nil, SignTypedDataOutput{Signed: *signed}, nil
}

// --- sign_challenge ---

type SignChallengeInput struct {
	Challenge string `json:"challenge" jsonschema:"the challenge text returned by the remote MCP server's auth_challenge tool"`
}

type SignChallengeOutput struct {
	Signature string `json:"signature"`
	Owner     string `json:"owner"`
}

// SignChallenge signs a self-service auth challenge; must start with ChallengePrefix and match chain id.
func (h *Handlers) SignChallenge(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in SignChallengeInput,
) (*mcp.CallToolResult, SignChallengeOutput, error) {
	if !strings.HasPrefix(in.Challenge, ChallengePrefix) {
		return nil, SignChallengeOutput{}, fmt.Errorf(
			"challenge must start with %q — refusing arbitrary-message signing",
			ChallengePrefix,
		)
	}
	parts := strings.SplitN(in.Challenge, ":", 4)
	if len(parts) != 4 {
		return nil, SignChallengeOutput{}, fmt.Errorf(
			"challenge malformed: expected %q:<chain_id>:<nonce>:<expires_at>",
			strings.TrimSuffix(ChallengePrefix, ":"),
		)
	}
	if h.ChainID != "" && parts[1] != h.ChainID {
		return nil, SignChallengeOutput{}, fmt.Errorf(
			"challenge chain_id %q does not match signer --chain-id %q",
			parts[1], h.ChainID,
		)
	}

	sig, err := h.Priv.Sign([]byte(in.Challenge))
	if err != nil {
		return nil, SignChallengeOutput{}, fmt.Errorf("sign challenge: %w", err)
	}
	return nil, SignChallengeOutput{
		Signature: base64.StdEncoding.EncodeToString(sig),
		Owner:     signer.DeriveAddress(h.Priv),
	}, nil
}

// --- whoami ---

type WhoamiInput struct{}

type WhoamiOutput struct {
	Owner   string `json:"owner"`    // bech32 svp1… (Cosmos)
	ChainID string `json:"chain_id"` // Cosmos chain id

	// EVM-side identity for the same key. EVMOwner is the 0x-checksummed
	// address; EVMChainID is the numeric EIP-155 chain id this signer signs
	// raw Ethereum txs for ("0" if EVM signing isn't configured).
	EVMOwner   string `json:"evm_owner"`
	EVMChainID string `json:"evm_chain_id"`
}

// Whoami returns the bech32 address derived from the loaded key and the configured chain id.
func (h *Handlers) Whoami(
	_ context.Context,
	_ *mcp.CallToolRequest,
	_ WhoamiInput,
) (*mcp.CallToolResult, WhoamiOutput, error) {
	return nil, WhoamiOutput{
		Owner:      signer.DeriveAddress(h.Priv),
		ChainID:    h.ChainID,
		EVMOwner:   signer.DeriveEvmAddress(h.Priv),
		EVMChainID: strconv.FormatUint(h.EVMChainID, 10),
	}, nil
}

// --- tool registration ---

// Register registers v0.1 signing tools on srv.
func Register(srv *mcp.Server, h *Handlers) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "sign_transaction",
		Description: "Sign a TxPayload produced by the svpchain remote MCP server's build_* tools. ONLY signs payloads whose chain_id matches this signer's configured --chain-id; payloads for other chains are refused pre-signature. Returns a SignedTx ready for broadcast_signed_tx.",
	}, h.SignTransaction)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "sign_evm_transaction",
		Description: "Sign a raw Ethereum transaction (EIP-1559 or legacy) from an EvmTxPayload produced by the svpchain remote MCP server's EVM build_* tools, using the same key as sign_transaction. ONLY signs payloads whose evm_chain_id matches this signer's configured chain; payloads for other chains are refused pre-signature. Returns RLP-encoded hex (raw_tx_hex) ready for eth_sendRawTransaction.",
	}, h.SignEvmTransaction)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "sign_typed_data",
		Description: "Sign EIP-712 typed data for x402 HTTP payments. Supports EIP-3009 TransferWithAuthorization (USDC) and Permit2 PermitWitnessTransferFrom (universal ERC-20 fallback). ONLY signs allowed primaryType values whose domain.chainId matches this signer's configured EVM chain; TransferWithAuthorization requires message.from = loaded key, Permit2 requires canonical Permit2 domain + signature recovery = loaded key. Other typed-data shapes are refused. Returns a 0x signature for x402 payment payloads.",
	}, h.SignTypedData)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "sign_challenge",
		Description: "Sign the svpchain self-service-auth challenge text returned by the remote MCP server's auth_challenge tool. Refuses any text that doesn't begin with svpchain-mcp-auth-v1: + the matching chain_id — this signer is NEVER a generic-message signing oracle. Returns a signature ready to pass to auth_verify.",
	}, h.SignChallenge)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "whoami",
		Description: "Return the svpchain bech32 owner address (svp1…) derived from the loaded key plus the configured chain id. Lets the calling agent confirm this signer is bound to the expected chain + key before any signing attempt. The signing key itself is never exposed.",
	}, h.Whoami)
}
