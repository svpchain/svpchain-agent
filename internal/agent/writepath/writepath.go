// Package writepath is the deterministic on-chain write subgraph:
// remote build_* → local sign_* → remote broadcast_*.
//
// The assistant conversation stays a tool loop. This package only refuses
// illegal transitions: signing a payload that was not built this run,
// broadcasting without a matching signature, or mutating the signed bytes
// (EVM raw_tx_hex / Cosmos tx_raw_bytes_b64). Diagnostic fields on signed_tx
// (v/r/s, tx_hash, signature_b64) may be dropped.
// It does not override the whitelist (package guard) or HITL (package hitl).
package writepath

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// Family is one independent write lane. Cosmos and EVM may be in flight together.
type Family string

const (
	FamilyCosmos Family = "cosmos"
	FamilyEVM    Family = "evm"
)

// Kind is where a tool sits on the write graph.
type Kind int

const (
	KindNone Kind = iota
	KindBuild
	KindSign
	KindBroadcast
)

const (
	signTransaction = "sign_transaction"
	signEVM         = "sign_evm_transaction"
	broadcastCosmos = "broadcast_signed_tx"
	broadcastEVM    = "broadcast_evm_tx"
)

// Violation stops the run the same way a whitelist rejection does: the model
// must not retry with a constructed payload or a patched signed_tx.
type Violation struct {
	Reason string
}

func (v *Violation) Error() string { return v.Reason }

// StopMessage is the user-visible end-of-run text. The runlog classifier keys
// off the "Write path rejected" prefix.
func (v *Violation) StopMessage() string {
	reason := ""
	if v != nil {
		reason = strings.TrimSpace(v.Reason)
	}
	if reason == "" {
		reason = "the build → sign → broadcast sequence was broken"
	}
	return fmt.Sprintf("Write path rejected — %s. No transaction was signed or broadcast.", reason)
}

// Tracker holds per-run write-graph state. One instance per agent.Run.
type Tracker struct {
	mu    sync.Mutex
	lanes map[Family]*lane
}

type lane struct {
	payload  string // canonical JSON of the unsigned payload from build_*
	signedTx string // EVM raw_tx_hex or Cosmos tx_raw_bytes_b64 identity from sign_*
	signed   bool
}

// New returns an empty tracker. A nil receiver is a no-op (tests).
func New() *Tracker {
	return &Tracker{lanes: make(map[Family]*lane)}
}

// Before rejects illegal sign/broadcast calls. Build tools always pass.
func (t *Tracker) Before(name string, args map[string]any) error {
	if t == nil {
		return nil
	}
	kind, family := Classify(name)
	switch kind {
	case KindSign:
		return t.beforeSign(family, name, args)
	case KindBroadcast:
		return t.beforeBroadcast(family, name, args)
	default:
		return nil
	}
}

// After records a successful build or sign so the next step can be checked.
func (t *Tracker) After(name string, args map[string]any, result string) error {
	if t == nil {
		return nil
	}
	kind, family := Classify(name)
	switch kind {
	case KindBuild:
		t.afterBuild(result)
		return nil
	case KindSign:
		return t.afterSign(family, result)
	case KindBroadcast:
		t.afterBroadcast(family)
		return nil
	default:
		return nil
	}
}

func (t *Tracker) beforeSign(family Family, name string, args map[string]any) error {
	payload, err := canonicalArg(args, "payload")
	if err != nil {
		return &Violation{Reason: name + " requires a payload from a matching build_* tool in this run"}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	ln := t.lanes[family]
	if ln == nil || ln.payload == "" {
		return &Violation{Reason: name + " requires a payload from a matching build_* tool in this run"}
	}
	if payload != ln.payload {
		return &Violation{Reason: "payload does not match the last build_* result; pass it verbatim"}
	}
	return nil
}

func (t *Tracker) beforeBroadcast(family Family, name string, args map[string]any) error {
	signed, err := wireFromArgs(args)
	if err != nil {
		return &Violation{Reason: name + " requires the signed_tx from sign_* in this run; do not construct or edit it"}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	ln := t.lanes[family]
	if ln == nil || !ln.signed || ln.signedTx == "" {
		return &Violation{Reason: name + " requires the signed_tx from sign_* in this run; do not construct or edit it"}
	}
	if signed != ln.signedTx {
		return &Violation{Reason: "signed_tx was altered; pass it verbatim from sign_*"}
	}
	return nil
}

func (t *Tracker) afterBuild(result string) {
	family, payload, ok := extractBuildPayload(result)
	if !ok {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.lanes == nil {
		t.lanes = make(map[Family]*lane)
	}
	t.lanes[family] = &lane{payload: payload}
}

func (t *Tracker) afterSign(family Family, result string) error {
	signed, err := extractSignedTx(result)
	if err != nil {
		return &Violation{Reason: "sign_* did not return a signed_tx object"}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	ln := t.lanes[family]
	if ln == nil {
		ln = &lane{}
		if t.lanes == nil {
			t.lanes = make(map[Family]*lane)
		}
		t.lanes[family] = ln
	}
	ln.signedTx = signed
	ln.signed = true
	return nil
}

func (t *Tracker) afterBroadcast(family Family) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.lanes, family)
}

// Classify maps a tool name onto the write graph. Tools outside the graph
// (queries, HITL-only sign_typed_data, x402, grants) are KindNone.
func Classify(name string) (Kind, Family) {
	switch strings.TrimSpace(name) {
	case signTransaction:
		return KindSign, FamilyCosmos
	case signEVM:
		return KindSign, FamilyEVM
	case broadcastCosmos:
		return KindBroadcast, FamilyCosmos
	case broadcastEVM:
		return KindBroadcast, FamilyEVM
	}
	if isOnchainBuild(name) {
		return KindBuild, ""
	}
	return KindNone, ""
}

func isOnchainBuild(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || name == "x402_build_payment" {
		return false
	}
	if strings.HasPrefix(name, "build_") {
		return true
	}
	// lendora_build_supply_tx and similar: they return an EVMTxPayload.
	return strings.Contains(name, "_build_") && strings.HasSuffix(name, "_tx")
}

func extractBuildPayload(result string) (Family, string, bool) {
	var raw any
	if err := json.Unmarshal([]byte(result), &raw); err != nil {
		return "", "", false
	}
	obj, _ := raw.(map[string]any)
	if obj == nil {
		return "", "", false
	}
	// approval_required (and similar) is not a signable payload.
	if _, ok := obj["approval_required"]; ok {
		if nested, has := obj["payload"]; !has || nested == nil {
			return "", "", false
		}
	}
	candidate := obj
	if nested, ok := obj["payload"].(map[string]any); ok {
		candidate = nested
	}
	family, ok := familyOf(candidate)
	if !ok {
		return "", "", false
	}
	canon, err := canonical(candidate)
	if err != nil {
		return "", "", false
	}
	return family, canon, true
}

func familyOf(obj map[string]any) (Family, bool) {
	if _, ok := obj["tx_body_bytes_b64"]; ok {
		return FamilyCosmos, true
	}
	if _, ok := obj["evm_chain_id"]; ok {
		return FamilyEVM, true
	}
	return "", false
}

func extractSignedTx(result string) (string, error) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(result), &raw); err != nil {
		return "", err
	}
	v, ok := raw["signed_tx"]
	if !ok || v == nil {
		return "", fmt.Errorf("missing signed_tx")
	}
	return signedWire(v)
}

func canonicalArg(args map[string]any, key string) (string, error) {
	if args == nil {
		return "", fmt.Errorf("missing %s", key)
	}
	v, ok := args[key]
	if !ok || v == nil {
		return "", fmt.Errorf("missing %s", key)
	}
	return canonical(v)
}

// wireFromArgs extracts the broadcast identity from a broadcast_* argument
// object: nested signed_tx, or flattened raw_tx_hex / tx_raw_bytes_b64.
func wireFromArgs(args map[string]any) (string, error) {
	if args == nil {
		return "", fmt.Errorf("missing signed_tx")
	}
	if v, ok := args["signed_tx"]; ok && v != nil {
		return signedWire(v)
	}
	return signedWire(args)
}

// signedWire is the byte identity that must match between sign_* and
// broadcast_*: EVM raw_tx_hex or Cosmos tx_raw_bytes_b64. Diagnostic fields
// (v/r/s, tx_hash, signature_b64) may be omitted without failing the check.
func signedWire(v any) (string, error) {
	switch t := v.(type) {
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return "", fmt.Errorf("empty signed_tx")
		}
		if strings.Contains(s, "=") || !looksLikeHex(s) {
			return "cosmos:" + s, nil
		}
		return "evm:" + normalizeHex(s), nil
	case map[string]any:
		if hex, ok := stringField(t, "raw_tx_hex"); ok {
			return "evm:" + normalizeHex(hex), nil
		}
		if b64, ok := stringField(t, "tx_raw_bytes_b64"); ok {
			return "cosmos:" + strings.TrimSpace(b64), nil
		}
		return "", fmt.Errorf("missing signed bytes")
	default:
		return "", fmt.Errorf("signed_tx must be an object")
	}
}

func stringField(m map[string]any, key string) (string, bool) {
	v, ok := m[key]
	if !ok || v == nil {
		return "", false
	}
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	s = strings.TrimSpace(s)
	return s, s != ""
}

func looksLikeHex(s string) bool {
	s = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s)), "0x")
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func normalizeHex(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "0x")
	return "0x" + s
}

func canonical(v any) (string, error) {
	bz, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	var norm any
	if err := json.Unmarshal(bz, &norm); err != nil {
		return "", err
	}
	out, err := json.Marshal(norm)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
