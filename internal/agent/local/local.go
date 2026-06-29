package local

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cosmos/evm/crypto/ethsecp256k1"

	"github.com/svpchain/svpchain-agent/internal/agent/llm"
	signermcp "github.com/svpchain/svpchain-agent/internal/mcp"
	"github.com/svpchain/svpchain-agent/internal/payload"
)

// Signer executes in-process sign_* tools.
type Signer struct {
	h *signermcp.Handlers
}

// NewSigner builds handlers for the loaded key.
func NewSigner(priv *ethsecp256k1.PrivKey, chainID string, evmChainID uint64) *Signer {
	return &Signer{h: &signermcp.Handlers{
		Priv:       priv,
		ChainID:    chainID,
		EVMChainID: evmChainID,
	}}
}

// Owner returns the bech32 address for auth.
func (l *Signer) Owner() string {
	_, out, err := l.h.Whoami(context.Background(), nil, signermcp.WhoamiInput{})
	if err != nil {
		return ""
	}
	return out.Owner
}

// SignChallenge signs an auth challenge and returns base64 signature.
func (l *Signer) SignChallenge(challenge string) (string, error) {
	_, out, err := l.h.SignChallenge(context.Background(), nil, signermcp.SignChallengeInput{
		Challenge: challenge,
	})
	if err != nil {
		return "", err
	}
	return out.Signature, nil
}

// CallTool dispatches a local signing tool by name.
func (l *Signer) CallTool(ctx context.Context, name string, args map[string]any) (string, error) {
	switch name {
	case "sign_transaction":
		return l.signTransaction(ctx, args)
	case "sign_evm_transaction":
		return l.signEvmTransaction(ctx, args)
	case "sign_typed_data":
		return l.signTypedData(ctx, args)
	case "sign_challenge":
		ch, _ := args["challenge"].(string)
		sig, err := l.SignChallenge(ch)
		if err != nil {
			return "", err
		}
		owner := l.Owner()
		bz, _ := json.Marshal(map[string]string{"signature": sig, "owner": owner})
		return string(bz), nil
	case "signer_whoami":
		_, out, err := l.h.Whoami(ctx, nil, signermcp.WhoamiInput{})
		if err != nil {
			return "", err
		}
		bz, err := json.Marshal(out)
		if err != nil {
			return "", err
		}
		return string(bz), nil
	case "evm_to_bech32":
		addr, _ := args["evm_address"].(string)
		_, out, err := l.h.EvmToBech32(ctx, nil, signermcp.EvmToBech32Input{EVMAddress: addr})
		if err != nil {
			return "", err
		}
		bz, err := json.Marshal(out)
		if err != nil {
			return "", err
		}
		return string(bz), nil
	default:
		return "", fmt.Errorf("unknown local tool %q", name)
	}
}

func (l *Signer) signTransaction(ctx context.Context, args map[string]any) (string, error) {
	raw, err := json.Marshal(args["payload"])
	if err != nil {
		return "", err
	}
	var p payload.TxPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", err
	}
	_, out, err := l.h.SignTransaction(ctx, nil, signermcp.SignTransactionInput{Payload: p})
	if err != nil {
		return "", err
	}
	bz, err := json.Marshal(map[string]payload.SignedTx{"signed_tx": out.SignedTx})
	if err != nil {
		return "", err
	}
	return string(bz), nil
}

func (l *Signer) signEvmTransaction(ctx context.Context, args map[string]any) (string, error) {
	raw, err := json.Marshal(args["payload"])
	if err != nil {
		return "", err
	}
	var p payload.EvmTxPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", err
	}
	_, out, err := l.h.SignEvmTransaction(ctx, nil, signermcp.SignEvmTransactionInput{Payload: p})
	if err != nil {
		return "", err
	}
	bz, err := json.Marshal(map[string]payload.SignedEvmTx{"signed_tx": out.SignedTx})
	if err != nil {
		return "", err
	}
	return string(bz), nil
}

func (l *Signer) signTypedData(ctx context.Context, args map[string]any) (string, error) {
	raw, err := json.Marshal(args["typed_data"])
	if err != nil {
		return "", err
	}
	var td payload.EIP712TypedData
	if err := json.Unmarshal(raw, &td); err != nil {
		return "", err
	}
	_, out, err := l.h.SignTypedData(ctx, nil, signermcp.SignTypedDataInput{TypedData: td})
	if err != nil {
		return "", err
	}
	bz, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(bz), nil
}

// ToolDefs returns extra tools exposed only on the local signer.
func ToolDefs() []llm.Tool {
	return []llm.Tool{
		{
			Type: "function",
			Function: llm.Function{
				Name:        "sign_transaction",
				Description: "Sign a TxPayload from remote build_* tools. Returns signed_tx for broadcast_signed_tx.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"payload": map[string]any{"type": "object", "description": "TxPayload from build_*"},
					},
					"required": []string{"payload"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.Function{
				Name:        "sign_evm_transaction",
				Description: "Sign an EvmTxPayload from remote EVM build_* tools. Returns signed_tx for broadcast_evm_tx.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"payload": map[string]any{"type": "object", "description": "EvmTxPayload from build_*"},
					},
					"required": []string{"payload"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.Function{
				Name:        "sign_typed_data",
				Description: "Sign EIP-712 typed data for x402 HTTP payments (TransferWithAuthorization / Permit2).",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"typed_data": map[string]any{"type": "object"},
					},
					"required": []string{"typed_data"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.Function{
				Name:        "sign_challenge",
				Description: "Sign auth challenge text from auth_challenge for auth_verify.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"challenge": map[string]any{"type": "string"},
					},
					"required": []string{"challenge"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.Function{
				Name:        "signer_whoami",
				Description: "Return local signing key addresses (svp1 + 0x) and chain ids.",
				Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
			},
		},
		{
			Type: "function",
			Function: llm.Function{
				Name:        "evm_to_bech32",
				Description: "Convert a 0x Ethereum address to the svpchain svp1… bech32 address for the SAME account. REQUIRED before any Cosmos x/bank send (build_bank_send) whose recipient was given as a 0x address — build_bank_send only accepts svp1… recipients. Example: to send SVP to 0xabc…, first call this, then pass the returned owner as build_bank_send.recipient.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"evm_address": map[string]any{"type": "string", "description": "0x Ethereum address (checksummed or lowercase)"},
					},
					"required": []string{"evm_address"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.Function{
				Name:        "http_fetch",
				Description: "HTTP GET/POST for x402 paid content. On 402, read payment_required from the response, call x402_prepare_typed_data, sign_typed_data, x402_build_payment, then retry with X-PAYMENT or PAYMENT-SIGNATURE header.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"url":     map[string]any{"type": "string"},
						"method":  map[string]any{"type": "string", "description": "GET or POST, default GET"},
						"headers": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
						"body":    map[string]any{"type": "string"},
					},
					"required": []string{"url"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.Function{
				Name:        "x402_prepare_typed_data",
				Description: "Build EIP-712 TransferWithAuthorization typed_data from a base64 PAYMENT-REQUIRED header. Generates a cryptographically random 32-byte nonce and validBefore window — do NOT invent nonce by hand.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"payment_required": map[string]any{"type": "string", "description": "Base64 PAYMENT-REQUIRED header from http_fetch 402 response"},
						"from":             map[string]any{"type": "string", "description": "Payer 0x address (evm_owner from signer_whoami)"},
					},
					"required": []string{"payment_required", "from"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.Function{
				Name:        "a2a_send_message",
				Description: "Send a message to another A2A-compatible agent and return its reply. Use for delegating sub-tasks (compliance review, research, etc.) to remote agents. agent_url is the base URL of the remote agent (Agent Card is fetched from /.well-known/agent-card.json).",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"agent_url": map[string]any{"type": "string", "description": "Base URL of the remote A2A agent, e.g. http://localhost:9001"},
						"message":   map[string]any{"type": "string", "description": "User message to send to the remote agent"},
					},
					"required": []string{"agent_url", "message"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.Function{
				Name:        "x402_build_payment",
				Description: "Assemble x402 v2 payment payload and base64 header value after sign_typed_data. Pass accepted from x402_prepare_typed_data, signature from sign_typed_data, and authorization from typed_data.message.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"accepted":      map[string]any{"type": "object"},
						"signature":     map[string]any{"type": "string"},
						"authorization": map[string]any{"type": "object"},
						"x402_version":  map[string]any{"type": "integer"},
					},
					"required": []string{"accepted", "signature", "authorization"},
				},
			},
		},
	}
}

func IsLocalTool(name string) bool {
	switch name {
	case "sign_transaction", "sign_evm_transaction", "sign_typed_data", "sign_challenge", "signer_whoami", "evm_to_bech32":
		return true
	default:
		return false
	}
}
