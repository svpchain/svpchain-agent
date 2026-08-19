package x402

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/svpchain/svpchain-agent/internal/payload"
	"github.com/svpchain/svpchain-agent/internal/signer"
)

const x402AuthWindowSeconds = 1800

type x402PaymentRequired struct {
	X402Version int           `json:"x402Version"`
	Accepts     []x402Accept  `json:"accepts"`
	Error       string        `json:"error,omitempty"`
	Resource    *x402Resource `json:"resource,omitempty"`
}

type x402Resource struct {
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

type x402Accept struct {
	Scheme            string                 `json:"scheme"`
	Network           string                 `json:"network"`
	Asset             string                 `json:"asset"`
	Amount            string                 `json:"amount"`
	PayTo             string                 `json:"payTo"`
	MaxTimeoutSeconds int                    `json:"maxTimeoutSeconds"`
	Extra             map[string]interface{} `json:"extra"`
}

type x402PrepareResult struct {
	TypedData   payload.EIP712TypedData `json:"typed_data"`
	Accepted    x402Accept              `json:"accepted"`
	ValidBefore string                  `json:"valid_before"`
	Nonce       string                  `json:"nonce"`
}

type x402PaymentPayload struct {
	X402Version int               `json:"x402Version"`
	Scheme      string            `json:"scheme"`
	Network     string            `json:"network"`
	Accepted    x402Accept        `json:"accepted"`
	Payload     x402SignedPayload `json:"payload"`
}

type x402SignedPayload struct {
	Signature     string                 `json:"signature"`
	Authorization map[string]interface{} `json:"authorization"`
}

// ParseX402PaymentRequired decodes a PAYMENT-REQUIRED header (base64 JSON).
func ParseX402PaymentRequired(raw string) (*x402PaymentRequired, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("payment_required is required")
	}
	bz, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("decode payment_required base64: %w", err)
	}
	var req x402PaymentRequired
	if err := json.Unmarshal(bz, &req); err != nil {
		return nil, fmt.Errorf("parse payment_required json: %w", err)
	}
	if len(req.Accepts) == 0 {
		return nil, fmt.Errorf("payment_required has no accepts entries")
	}
	return &req, nil
}

// PrepareX402TypedData builds EIP-712 TransferWithAuthorization from x402 requirements.
func PrepareX402TypedData(paymentRequired string, from string) (*x402PrepareResult, error) {
	req, err := ParseX402PaymentRequired(paymentRequired)
	if err != nil {
		return nil, err
	}
	from = strings.TrimSpace(from)
	if from == "" {
		return nil, fmt.Errorf("from (payer EVM address) is required")
	}
	if !strings.HasPrefix(strings.ToLower(from), "0x") {
		return nil, fmt.Errorf("from must be a 0x EVM address")
	}

	accept := req.Accepts[0]
	if strings.TrimSpace(accept.Scheme) != "exact" {
		return nil, fmt.Errorf("unsupported x402 scheme %q (only exact/EIP-3009 is supported)", accept.Scheme)
	}
	chainID, err := parseEIP155Network(accept.Network)
	if err != nil {
		return nil, err
	}
	name, version, err := tokenDomainFromExtra(accept.Extra)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(accept.Asset) == "" {
		return nil, fmt.Errorf("accept.asset is required")
	}
	if strings.TrimSpace(accept.PayTo) == "" {
		return nil, fmt.Errorf("accept.payTo is required")
	}
	if strings.TrimSpace(accept.Amount) == "" {
		return nil, fmt.Errorf("accept.amount is required")
	}

	nonce, err := randomBytes32Hex()
	if err != nil {
		return nil, err
	}
	validBefore := strconv.FormatInt(time.Now().Unix()+x402AuthWindowSeconds, 10)

	td := payload.EIP712TypedData{
		Types: map[string][]payload.EIP712Type{
			"EIP712Domain": {
				{Name: "name", Type: "string"},
				{Name: "version", Type: "string"},
				{Name: "chainId", Type: "uint256"},
				{Name: "verifyingContract", Type: "address"},
			},
			signer.PrimaryTypeTransferWithAuthorization: {
				{Name: "from", Type: "address"},
				{Name: "to", Type: "address"},
				{Name: "value", Type: "uint256"},
				{Name: "validAfter", Type: "uint256"},
				{Name: "validBefore", Type: "uint256"},
				{Name: "nonce", Type: "bytes32"},
			},
		},
		PrimaryType: signer.PrimaryTypeTransferWithAuthorization,
		Domain: payload.EIP712Domain{
			Name:              name,
			Version:           version,
			ChainId:           chainID,
			VerifyingContract: accept.Asset,
		},
		Message: map[string]interface{}{
			"from":        from,
			"to":          accept.PayTo,
			"value":       accept.Amount,
			"validAfter":  "0",
			"validBefore": validBefore,
			"nonce":       nonce,
		},
	}

	return &x402PrepareResult{
		TypedData:   td,
		Accepted:    accept,
		ValidBefore: validBefore,
		Nonce:       nonce,
	}, nil
}

// BuildX402PaymentPayload assembles the x402 v2 payment object and base64 header value.
func BuildX402PaymentPayload(x402Version int, accepted x402Accept, signature string, authorization map[string]interface{}) (map[string]any, string, error) {
	if x402Version == 0 {
		x402Version = 2
	}
	payment := x402PaymentPayload{
		X402Version: x402Version,
		Scheme:      accepted.Scheme,
		Network:     accepted.Network,
		Accepted:    accepted,
		Payload: x402SignedPayload{
			Signature:     signature,
			Authorization: authorization,
		},
	}
	bz, err := json.Marshal(payment)
	if err != nil {
		return nil, "", err
	}
	return map[string]any{
		"payment": json.RawMessage(bz),
	}, base64.StdEncoding.EncodeToString(bz), nil
}

func PrepareFromArgs(args map[string]any) (string, error) {
	paymentRequired, _ := args["payment_required"].(string)
	from, _ := args["from"].(string)
	result, err := PrepareX402TypedData(paymentRequired, from)
	if err != nil {
		return "", err
	}
	out := map[string]any{
		"typed_data":   result.TypedData,
		"accepted":     result.Accepted,
		"valid_before": result.ValidBefore,
		"nonce":        result.Nonce,
		"x402_version": 2,
	}
	bz, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(bz), nil
}

func BuildPaymentFromArgs(args map[string]any) (string, error) {
	signature, _ := args["signature"].(string)
	if strings.TrimSpace(signature) == "" {
		return "", fmt.Errorf("signature is required")
	}
	var accepted x402Accept
	if raw, ok := args["accepted"]; ok {
		bz, err := json.Marshal(raw)
		if err != nil {
			return "", err
		}
		if err := json.Unmarshal(bz, &accepted); err != nil {
			return "", fmt.Errorf("parse accepted: %w", err)
		}
	}
	var authorization map[string]interface{}
	if raw, ok := args["authorization"]; ok {
		bz, err := json.Marshal(raw)
		if err != nil {
			return "", err
		}
		if err := json.Unmarshal(bz, &authorization); err != nil {
			return "", fmt.Errorf("parse authorization: %w", err)
		}
	}
	if authorization == nil {
		return "", fmt.Errorf("authorization is required")
	}
	x402Version := 2
	if v, ok := args["x402_version"].(float64); ok && v > 0 {
		x402Version = int(v)
	}
	paymentObj, paymentB64, err := BuildX402PaymentPayload(x402Version, accepted, signature, authorization)
	if err != nil {
		return "", err
	}
	out := map[string]any{
		"payment":           paymentObj["payment"],
		"payment_b64":       paymentB64,
		"submit_header":     "X-PAYMENT",
		"submit_header_alt": "PAYMENT-SIGNATURE",
		"hint":              "Resend the same GET with header X-PAYMENT (web) or PAYMENT-SIGNATURE (API) set to payment_b64",
	}
	bz, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(bz), nil
}

func parseEIP155Network(network string) (uint64, error) {
	network = strings.TrimSpace(network)
	const prefix = "eip155:"
	if !strings.HasPrefix(network, prefix) {
		return 0, fmt.Errorf("unsupported network %q (want eip155:<chainId>)", network)
	}
	id, err := strconv.ParseUint(strings.TrimPrefix(network, prefix), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse network %q: %w", network, err)
	}
	return id, nil
}

func tokenDomainFromExtra(extra map[string]interface{}) (name, version string, err error) {
	if extra == nil {
		return "", "", fmt.Errorf("accept.extra is required (token name + version for EIP-712 domain)")
	}
	name, _ = extra["name"].(string)
	version, _ = extra["version"].(string)
	name = strings.TrimSpace(name)
	version = strings.TrimSpace(version)
	if name == "" || version == "" {
		return "", "", fmt.Errorf("accept.extra must include name and version")
	}
	return name, version, nil
}

func randomBytes32Hex() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	return "0x" + hex.EncodeToString(b[:]), nil
}

func IsTool(name string) bool {
	switch name {
	case "x402_prepare_typed_data", "x402_build_payment":
		return true
	default:
		return false
	}
}
