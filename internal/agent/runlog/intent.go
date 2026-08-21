package runlog

import (
	"encoding/json"
	"strings"
)

const (
	IntentMatched    = "matched"    // required fields found in confirmed tx events
	IntentMismatch   = "mismatch"   // confirmed tx, but expected fields did not appear
	IntentIncluded   = "included"   // tx included; event-level compare not available (EVM)
	IntentUnobserved = "unobserved" // no confirmed tx yet
	IntentSkipped    = "skipped"    // no chain RPC
)

// IntentCheck is one write intent inferred from a build_* (or similar) tool,
// scored against CometBFT tx events from confirmed transactions.
type IntentCheck struct {
	Kind     string            `json:"kind"`
	Tool     string            `json:"tool"`
	Expect   map[string]string `json:"expect,omitempty"`
	Status   string            `json:"status"`
	Detail   string            `json:"detail,omitempty"`
	Observed map[string]string `json:"observed,omitempty"`
}

var expectKeys = []string{
	"recipient", "to", "spender", "operator",
	"amount", "denom", "ticker", "symbol", "market",
	"side", "size", "price", "token_id", "order_id", "agent_id",
}

// ExtractIntents pulls write intents from successful tool steps.
func ExtractIntents(steps []Step) []IntentCheck {
	var out []IntentCheck
	for _, st := range steps {
		if st.Kind != "tool" || strings.TrimSpace(st.Tool) == "" {
			continue
		}
		if st.OK != nil && !*st.OK {
			continue
		}
		if !isWriteIntentTool(st.Tool) {
			continue
		}
		expect := collectExpect(st.Args, st.Result)
		out = append(out, IntentCheck{
			Kind:   intentKind(st.Tool),
			Tool:   st.Tool,
			Expect: expect,
		})
	}
	return out
}

// MatchIntents scores extracted intents against confirmed tx events.
func MatchIntents(intents []IntentCheck, checks []TxCheck, events []ChainEvent, haveLookup bool) []IntentCheck {
	if len(intents) == 0 {
		return nil
	}
	confirmed := false
	for _, c := range checks {
		if c.Status == TxConfirmed {
			confirmed = true
			break
		}
	}
	blob := eventBlob(events)
	out := make([]IntentCheck, len(intents))
	for i, in := range intents {
		out[i] = scoreIntent(in, confirmed, haveLookup, blob, events)
	}
	return out
}

func isWriteIntentTool(name string) bool {
	n := strings.ToLower(name)
	if strings.HasPrefix(n, "build_") || strings.HasPrefix(n, "lendora_build_") {
		return true
	}
	switch n {
	case "create_root_delegation", "update_delegation", "pause_delegation",
		"resume_delegation", "revoke_delegation", "delegate_task":
		return true
	default:
		return false
	}
}

func intentKind(tool string) string {
	n := strings.ToLower(tool)
	switch {
	case strings.Contains(n, "bank_send"):
		return "bank_send"
	case strings.Contains(n, "place") && strings.Contains(n, "order"):
		return "place_order"
	case strings.Contains(n, "cancel") && strings.Contains(n, "order"):
		return "cancel_order"
	case strings.Contains(n, "erc20_transfer"):
		return "erc20_transfer"
	case strings.Contains(n, "erc20_approve") || strings.Contains(n, "approval"):
		return "erc20_approve"
	case strings.Contains(n, "erc721"):
		return "nft"
	case strings.Contains(n, "swap"):
		return "swap"
	case strings.Contains(n, "bridge"):
		return "bridge"
	case strings.Contains(n, "lendora"):
		return "lending"
	case strings.Contains(n, "delegat"):
		return "delegation"
	default:
		return "write"
	}
}

func isEVMKind(kind string) bool {
	switch kind {
	case "erc20_transfer", "erc20_approve", "nft", "swap", "lending", "bridge":
		return true
	default:
		return false
	}
}

func collectExpect(argsJSON, resultJSON string) map[string]string {
	out := map[string]string{}
	absorbJSON(argsJSON, out)
	var result map[string]any
	if json.Unmarshal([]byte(strings.TrimSpace(resultJSON)), &result) == nil {
		if summary, ok := result["summary"].(map[string]any); ok {
			absorbMap(summary, out)
		}
		absorbMap(result, out)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func absorbJSON(raw string, out map[string]string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return
	}
	var m map[string]any
	if json.Unmarshal([]byte(raw), &m) != nil {
		return
	}
	absorbMap(m, out)
}

func absorbMap(m map[string]any, out map[string]string) {
	for _, key := range expectKeys {
		if _, ok := out[key]; ok {
			continue
		}
		if s := stringArg(m, key); s != "" {
			out[key] = clipExpect(s)
		}
	}
	if _, ok := out["size"]; !ok {
		if s := stringArg(m, "size_human"); s != "" {
			out["size"] = clipExpect(s)
		}
	}
}

func stringArg(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		if t == float64(int64(t)) {
			return strings.TrimSpace(strings.TrimRight(strings.TrimRight(
				jsonNumber(t), "0"), "."))
		}
		return jsonNumber(t)
	default:
		return ""
	}
}

func jsonNumber(f float64) string {
	b, _ := json.Marshal(f)
	return string(b)
}

func clipExpect(s string) string {
	if len(s) > 200 {
		return s[:200]
	}
	return s
}

func eventBlob(events []ChainEvent) string {
	var b strings.Builder
	for _, e := range events {
		b.WriteString(strings.ToLower(e.Type))
		b.WriteByte('\n')
		for k, v := range e.Attrs {
			b.WriteString(strings.ToLower(k))
			b.WriteByte('=')
			b.WriteString(strings.ToLower(v))
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func scoreIntent(in IntentCheck, confirmed, haveLookup bool, blob string, events []ChainEvent) IntentCheck {
	if !haveLookup {
		in.Status = IntentSkipped
		in.Detail = "chain RPC is not configured"
		return in
	}
	if !confirmed {
		in.Status = IntentUnobserved
		in.Detail = "no confirmed transaction yet"
		return in
	}

	required := requiredFields(in.Kind, in.Expect)
	observed := map[string]string{}
	missing := make([]string, 0, len(required))
	for _, key := range required {
		want := in.Expect[key]
		if want == "" {
			continue
		}
		if hit, ok := findInEvents(key, want, events); ok {
			observed[key] = hit
			continue
		}
		missing = append(missing, key)
	}

	if len(required) > 0 && len(missing) == 0 {
		in.Status = IntentMatched
		in.Observed = observed
		return in
	}
	if len(required) > 0 {
		if isEVMKind(in.Kind) && len(missing) == len(required) {
			in.Status = IntentIncluded
			in.Detail = "tx included; EVM logs are not in Cosmos events"
			return in
		}
		in.Status = IntentMismatch
		in.Observed = observed
		in.Detail = "missing in events: " + strings.Join(missing, ", ")
		return in
	}
	if actionHint(in.Kind, blob) {
		in.Status = IntentMatched
		in.Detail = "matched by message type"
		return in
	}
	if isEVMKind(in.Kind) {
		in.Status = IntentIncluded
		in.Detail = "tx included; EVM logs are not in Cosmos events"
		return in
	}
	in.Status = IntentIncluded
	in.Detail = "tx included; no comparable event fields"
	return in
}

func requiredFields(kind string, expect map[string]string) []string {
	var keys []string
	switch kind {
	case "bank_send":
		keys = []string{"recipient"}
	case "place_order":
		keys = []string{"ticker", "market", "symbol", "side"}
	case "cancel_order":
		keys = []string{"order_id"}
	case "erc20_transfer", "nft", "bridge":
		keys = []string{"to", "recipient"}
	case "erc20_approve":
		keys = []string{"spender", "operator"}
	case "delegation":
		keys = []string{"agent_id"}
	}
	var out []string
	seen := map[string]bool{}
	for _, k := range keys {
		if expect[k] == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, k)
	}
	return out
}

func findInEvents(key, want string, events []ChainEvent) (string, bool) {
	want = strings.TrimSpace(want)
	if want == "" {
		return "", false
	}
	aliases := fieldAliases(key)
	for _, e := range events {
		for k, v := range e.Attrs {
			if !aliasHit(k, aliases) {
				continue
			}
			if matchAttr(key, want, v) {
				return v, true
			}
		}
	}
	// Unexpected key names: still accept an exact value anywhere in the event.
	for _, e := range events {
		for _, v := range e.Attrs {
			if strings.EqualFold(strings.TrimSpace(v), want) {
				return v, true
			}
		}
	}
	return "", false
}

func fieldAliases(key string) []string {
	switch key {
	case "recipient", "to":
		return []string{"recipient", "receiver", "to"}
	case "spender", "operator":
		return []string{"spender", "operator"}
	case "ticker", "market", "symbol":
		return []string{"ticker", "market", "symbol"}
	default:
		return []string{key}
	}
}

func aliasHit(got string, aliases []string) bool {
	g := strings.ToLower(strings.TrimSpace(got))
	for _, a := range aliases {
		if g == a {
			return true
		}
	}
	return false
}

func matchAttr(key, want, got string) bool {
	got = strings.TrimSpace(got)
	if strings.EqualFold(got, want) {
		return true
	}
	if key == "amount" || key == "size" {
		d := digitsOnly(want)
		return d != "" && strings.Contains(digitsOnly(got), d)
	}
	return false
}

func actionHint(kind, blob string) bool {
	switch kind {
	case "bank_send":
		return strings.Contains(blob, "msgsend") || strings.Contains(blob, "type=transfer") ||
			strings.Contains(blob, "coin_received") || strings.Contains(blob, "transfer\n")
	case "place_order":
		return strings.Contains(blob, "msgplaceorder") || strings.Contains(blob, "place_order")
	case "cancel_order":
		return strings.Contains(blob, "msgcancelorder") || strings.Contains(blob, "cancel_order")
	case "delegation":
		return strings.Contains(blob, "agentwallet") || strings.Contains(blob, "delegation")
	default:
		return strings.Contains(blob, "ethereum_tx") || strings.Contains(blob, "msgethereumtx")
	}
}

func digitsOnly(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= '0' && c <= '9' {
			b.WriteByte(c)
		}
	}
	return b.String()
}
