package hitl

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/svpchain/svpchain-agent/internal/evmcall"
	"github.com/svpchain/svpchain-agent/internal/payload"
)

// SignRequest builds the confirmation dialog for a local sign_* tool.
// The payload bytes that will be signed remain authoritative; these lines are
// display-only. Unparseable arguments still produce a dialog — never an
// auto-approve.
func SignRequest(name string, args map[string]any) Request {
	switch strings.TrimSpace(name) {
	case KindSignTransaction:
		return cosmosSignRequest(args)
	case KindSignEVM:
		return evmSignRequest(args)
	case KindSignTypedData:
		return typedDataSignRequest(args)
	default:
		return Request{Kind: name, Title: "Approve " + name, Lines: []string{"Unknown signing action."}}
	}
}

func cosmosSignRequest(args map[string]any) Request {
	req := Request{Kind: KindSignTransaction, Title: "Sign Cosmos transaction"}
	p, err := decodeArg[payload.TxPayload](args, "payload")
	if err != nil {
		req.Lines = []string{"Could not parse the payload. Decline unless you asked for this signature."}
		return req
	}
	s := p.Summary
	req.Lines = compactLines(
		labeled("Chain", p.ChainID),
		labeled("Signer", p.SignerAddress),
		labeled("Tool", s.ToolName),
		labeled("Message", s.MsgTypeURL),
		labeled("Amount", s.AmountHuman),
		labeled("Recipient", s.RecipientOwner),
		labeled("Market", s.Ticker),
		labeled("Side", s.Side),
		labeled("Size", s.SizeHuman),
		labeled("Price", s.PriceHuman),
		coinsLine("Fee", p.Fee.Amount),
	)
	if len(req.Lines) == 0 {
		req.Lines = []string{"Cosmos transaction with no summary fields. Decline unless you asked for this signature."}
	}
	return req
}

func evmSignRequest(args map[string]any) Request {
	req := Request{Kind: KindSignEVM, Title: "Sign EVM transaction"}
	p, err := decodeArg[payload.EvmTxPayload](args, "payload")
	if err != nil {
		req.Lines = []string{"Could not parse the payload. Decline unless you asked for this signature."}
		return req
	}
	req.Lines = compactLines(
		labeled("EVM chain", p.EVMChainID),
		labeled("Signer", p.SignerAddress),
		labeled("Tool", p.Summary.ToolName),
		labeled("Description", p.Summary.Description),
		labeled("To", p.To),
		labeled("Value (wei)", p.Value),
		labeled("Gas limit", p.Gas),
		evmCallLine(p.Data),
	)
	if len(req.Lines) == 0 {
		req.Lines = []string{"EVM transaction with no summary fields. Decline unless you asked for this signature."}
	}
	return req
}

func typedDataSignRequest(args map[string]any) Request {
	req := Request{Kind: KindSignTypedData, Title: "Sign typed data"}
	td, err := decodeArg[payload.EIP712TypedData](args, "typed_data")
	if err != nil {
		req.Lines = []string{"Could not parse typed data. Decline unless you asked for this signature."}
		return req
	}
	req.Lines = compactLines(
		labeled("Type", td.PrimaryType),
		labeled("Domain", td.Domain.Name),
		labeled("Contract", td.Domain.VerifyingContract),
		labeled("Chain", fmt.Sprint(td.Domain.ChainId)),
		messageField(td.Message, "from"),
		messageField(td.Message, "to"),
		messageField(td.Message, "value"),
		messageField(td.Message, "validBefore"),
	)
	if len(req.Lines) == 0 {
		req.Lines = []string{"EIP-712 typed data with no recognizable fields. Decline unless you asked for this signature."}
	}
	return req
}

func decodeArg[T any](args map[string]any, key string) (T, error) {
	var zero T
	if args == nil {
		return zero, fmt.Errorf("missing %s", key)
	}
	raw, ok := args[key]
	if !ok || raw == nil {
		return zero, fmt.Errorf("missing %s", key)
	}
	bz, err := json.Marshal(raw)
	if err != nil {
		return zero, err
	}
	var out T
	if err := json.Unmarshal(bz, &out); err != nil {
		return zero, err
	}
	return out, nil
}

func labeled(label, v string) string {
	v = strings.TrimSpace(v)
	if v == "" || v == "<nil>" {
		return ""
	}
	return label + ": " + v
}

func coinsLine(label string, coins []payload.Coin) string {
	if len(coins) == 0 {
		return ""
	}
	parts := make([]string, 0, len(coins))
	for _, c := range coins {
		a, d := strings.TrimSpace(c.Amount), strings.TrimSpace(c.Denom)
		if a == "" && d == "" {
			continue
		}
		parts = append(parts, strings.TrimSpace(a+" "+d))
	}
	if len(parts) == 0 {
		return ""
	}
	return labeled(label, strings.Join(parts, ", "))
}

func evmCallLine(data string) string {
	data = strings.TrimSpace(data)
	if data == "" {
		return ""
	}
	bz, err := hexutil.Decode(data)
	if err != nil {
		return labeled("Call data", "unreadable")
	}
	dest, err := evmcall.Decode(bz)
	if err != nil {
		return labeled("Call data", "recognized selector, undecodable — refuse unless you asked for this")
	}
	if dest == nil {
		if len(bz) >= 4 {
			return labeled("Selector", fmt.Sprintf("0x%x", bz[:4]))
		}
		return ""
	}
	return labeled(string(dest.Role)+" ("+dest.Method+")", dest.Address.Hex())
}

func messageField(msg map[string]any, key string) string {
	if msg == nil {
		return ""
	}
	v, ok := msg[key]
	if !ok || v == nil {
		return ""
	}
	s := strings.TrimSpace(fmt.Sprint(v))
	if s == "" || s == "<nil>" {
		return ""
	}
	return labeled(key, s)
}

func compactLines(in ...string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}
