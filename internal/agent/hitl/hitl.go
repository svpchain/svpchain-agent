// Package hitl is the assistant's human-in-the-loop gate: every grant and every
// local sign_* call must get an explicit yes from the user before it proceeds.
//
// A nil hook, a decline, a cancelled context, or a timeout all deny. There is
// no default-approve path. Whitelist rejections stay in package guard — those
// are policy, not something a dialog can override.
package hitl

import (
	"context"
	"fmt"
	"strings"
)

// Kind values are stable identifiers the GUI uses to style the dialog.
const (
	KindCreateDelegation = "create_delegation"
	KindResumeDelegation = "resume_delegation"
	KindRevokeDelegation = "revoke_delegation"
	KindDelegateTask     = "delegate_task"
	KindSignTransaction  = "sign_transaction"
	KindSignEVM          = "sign_evm_transaction"
	KindSignTypedData    = "sign_typed_data"
)

// Request is what the user is shown. The GUI already understands this shape
// (kind / title / lines) from the original delegation confirm dialog.
type Request struct {
	Kind  string   `json:"kind"`
	Title string   `json:"title"`
	Lines []string `json:"lines"`
}

// Func asks the user. Implementations must treat a cancelled ctx as a denial
// and must never return true unless the user explicitly approved.
type Func func(ctx context.Context, req Request) bool

// Denied is returned when Ask does not get an explicit approval. The agent
// loop stops the run (same as a whitelist Rejection) so the model cannot
// retry the grant or the signature.
type Denied struct {
	Kind  string
	Title string
}

func (d *Denied) Error() string {
	return fmt.Sprintf("the user declined %q", d.displayTitle())
}

func (d *Denied) displayTitle() string {
	if d == nil {
		return ""
	}
	if t := strings.TrimSpace(d.Title); t != "" {
		return t
	}
	return strings.TrimSpace(d.Kind)
}

func (d *Denied) signing() bool {
	if d == nil {
		return false
	}
	switch d.Kind {
	case KindSignTransaction, KindSignEVM, KindSignTypedData:
		return true
	default:
		return false
	}
}

// StopMessage is the user-visible end-of-run text. The runlog classifier keys
// off the "Signing declined" / "Declined —" prefixes.
func (d *Denied) StopMessage() string {
	title := d.displayTitle()
	if d.signing() {
		return fmt.Sprintf("Signing declined — the user did not approve %q. No transaction was signed or broadcast.", title)
	}
	return fmt.Sprintf("Declined — the user did not approve %q. No further action was taken.", title)
}

// Ask runs the confirmation hook. Nil fn or a false answer both deny.
func Ask(ctx context.Context, fn Func, req Request) error {
	if ctx.Err() != nil {
		return &Denied{Kind: req.Kind, Title: req.Title}
	}
	if fn == nil || !fn(ctx, req) {
		return &Denied{Kind: req.Kind, Title: req.Title}
	}
	return nil
}

// NeedsConfirm reports whether a local tool must pause for user approval.
// sign_challenge is excluded: it is the MCP auth handshake, prefix-gated in
// the signer, not a fund movement.
func NeedsConfirm(name string) bool {
	switch strings.TrimSpace(name) {
	case KindSignTransaction, KindSignEVM, KindSignTypedData:
		return true
	default:
		return false
	}
}
