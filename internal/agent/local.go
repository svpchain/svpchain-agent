package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cosmos/evm/crypto/ethsecp256k1"

	signermcp "github.com/svpchain/svpchain-agent/internal/mcp"
	"github.com/svpchain/svpchain-agent/internal/payload"
)

// LocalSigner executes in-process sign_* tools.
type LocalSigner struct {
	h *signermcp.Handlers
}

// NewLocalSigner builds handlers for the loaded key.
func NewLocalSigner(priv *ethsecp256k1.PrivKey, chainID string, evmChainID uint64) *LocalSigner {
	return &LocalSigner{h: &signermcp.Handlers{
		Priv:       priv,
		ChainID:    chainID,
		EVMChainID: evmChainID,
	}}
}

// Owner returns the bech32 address for auth.
func (l *LocalSigner) Owner() string {
	_, out, err := l.h.Whoami(context.Background(), nil, signermcp.WhoamiInput{})
	if err != nil {
		return ""
	}
	return out.Owner
}

// SignChallenge signs an auth challenge and returns base64 signature.
func (l *LocalSigner) SignChallenge(challenge string) (string, error) {
	_, out, err := l.h.SignChallenge(context.Background(), nil, signermcp.SignChallengeInput{
		Challenge: challenge,
	})
	if err != nil {
		return "", err
	}
	return out.Signature, nil
}

// CallTool dispatches a local signing tool by name.
func (l *LocalSigner) CallTool(ctx context.Context, name string, args map[string]any) (string, error) {
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
	default:
		return "", fmt.Errorf("unknown local tool %q", name)
	}
}

func (l *LocalSigner) signTransaction(ctx context.Context, args map[string]any) (string, error) {
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

func (l *LocalSigner) signEvmTransaction(ctx context.Context, args map[string]any) (string, error) {
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

func (l *LocalSigner) signTypedData(ctx context.Context, args map[string]any) (string, error) {
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

// LocalToolDefs returns extra tools exposed only on the local signer.
func LocalToolDefs() []llmTool {
	return []llmTool{
		{
			Type: "function",
			Function: llmFunction{
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
			Function: llmFunction{
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
			Function: llmFunction{
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
			Function: llmFunction{
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
			Function: llmFunction{
				Name:        "signer_whoami",
				Description: "Return local signing key addresses (svp1 + 0x) and chain ids.",
				Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
			},
		},
		{
			Type: "function",
			Function: llmFunction{
				Name:        "http_fetch",
				Description: "HTTP GET/POST for x402 paid content. On 402, parse payment requirements, sign with sign_typed_data, retry with X-PAYMENT header (base64 JSON payment payload).",
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
	}
}

func isLocalTool(name string) bool {
	switch name {
	case "sign_transaction", "sign_evm_transaction", "sign_typed_data", "sign_challenge", "signer_whoami", "http_fetch":
		return true
	default:
		return false
	}
}
