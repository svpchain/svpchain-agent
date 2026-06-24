package signer

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/cosmos/gogoproto/proto"

	"github.com/svpchain/svpchain-agent/internal/payload"
	"github.com/svpchain/svpchain-agent/internal/whitelist"
)

// bankMsgSendTypeURL is the Any type URL for a x/bank send; the only message
// type whose Go binding is available here, so the only one we decode in full.
const bankMsgSendTypeURL = "/cosmos.bank.v1beta1.MsgSend"

// allowedMsgTypeURLs is the set of on-chain message types the signer is willing
// to sign. The remote MCP server pre-builds the TxBody and the signer otherwise
// treats it as opaque bytes (see signer.Sign), so this allowlist is the signer's
// own, caller-independent guard against a malicious/compromised builder slipping
// in an unexpected message (e.g. an authz grant or a governance proposal) behind
// a benign Summary.
//
// This is the exact, complete set the remote MCP builder produces (verified
// against protocol/lib/mcp/builder/{orders,funds}.go and lib/mcp/tools/cross.go:
// banktypes.MsgSend, clobtypes.{NewMsgPlaceOrder,MsgCancelOrder,MsgBatchCancel},
// sendingtypes.{NewMsgCreateTransfer,NewMsgDepositToSubaccount,NewMsgWithdraw-
// FromSubaccount}). build_swap / build_token_approval / build_erc* are EVM-only
// (EVMTxPayload) and never reach this path. Admin/governance dYdX messages are
// intentionally excluded — the wallet must never sign them. Keep this in sync
// with the builder if it gains new tools.
var allowedMsgTypeURLs = map[string]struct{}{
	bankMsgSendTypeURL: {},

	// x/clob — orders.
	"/dydxprotocol.clob.MsgPlaceOrder":  {},
	"/dydxprotocol.clob.MsgCancelOrder": {},
	"/dydxprotocol.clob.MsgBatchCancel": {},

	// x/sending — subaccount transfers.
	"/dydxprotocol.sending.MsgCreateTransfer":         {},
	"/dydxprotocol.sending.MsgDepositToSubaccount":    {},
	"/dydxprotocol.sending.MsgWithdrawFromSubaccount": {},
}

// validateTxBody decodes the SIGN_MODE_DIRECT TxBody bytes the remote server
// produced and enforces the signer's own policy before signing. It fails closed:
// any decode error, an empty message set, or a message type outside
// allowedMsgTypeURLs is refused. For the one message type with a Go binding
// (x/bank MsgSend) it additionally checks the decoded fields for internal
// consistency. signerAddr is the key-derived bech32 address ("" tolerated for
// demos, mirroring Sign).
//
// This guards only against a hostile builder returning a different transaction
// than expected; it cannot know the user's intended recipient/amount, which is
// out of scope here (would require caller-supplied intent or a confirmation step).
func validateTxBody(bodyBytes []byte, summary payload.Summary, signerAddr, chainID string) error {
	var body txtypes.TxBody
	if err := proto.Unmarshal(bodyBytes, &body); err != nil {
		return fmt.Errorf("decode tx body: %w", err)
	}
	if len(body.Messages) == 0 {
		return fmt.Errorf("transaction has no messages")
	}

	summaryTypeSeen := summary.MsgTypeURL == ""
	for i, msg := range body.Messages {
		if msg == nil {
			return fmt.Errorf("message %d is nil", i)
		}
		if _, ok := allowedMsgTypeURLs[msg.TypeUrl]; !ok {
			return fmt.Errorf("message %d type %q is not on the signer allowlist", i, msg.TypeUrl)
		}
		if msg.TypeUrl == summary.MsgTypeURL {
			summaryTypeSeen = true
		}
		if msg.TypeUrl == bankMsgSendTypeURL {
			if err := validateBankSend(msg.Value, summary, signerAddr, chainID); err != nil {
				return fmt.Errorf("message %d: %w", i, err)
			}
		}
	}

	// The server-supplied Summary is informational, but a Summary that names a
	// message type absent from the body signals tampering — refuse rather than
	// sign something the operator was shown as a different action.
	if !summaryTypeSeen {
		return fmt.Errorf("summary.msg_type_url %q does not match any message in the tx body", summary.MsgTypeURL)
	}
	return nil
}

// validateBankSend decodes a cosmos.bank.v1beta1.MsgSend and sanity-checks its
// fields: the sender must be this signer (when known), the amount must be a
// valid non-empty coin set, and — when the Summary names a recipient — it must
// match the on-chain ToAddress.
func validateBankSend(value []byte, summary payload.Summary, signerAddr, chainID string) error {
	var msg banktypes.MsgSend
	if err := proto.Unmarshal(value, &msg); err != nil {
		return fmt.Errorf("decode MsgSend: %w", err)
	}
	if _, err := sdk.AccAddressFromBech32(msg.FromAddress); err != nil {
		return fmt.Errorf("MsgSend.from_address %q is not a valid address: %w", msg.FromAddress, err)
	}
	if _, err := sdk.AccAddressFromBech32(msg.ToAddress); err != nil {
		return fmt.Errorf("MsgSend.to_address %q is not a valid address: %w", msg.ToAddress, err)
	}
	if signerAddr != "" && msg.FromAddress != signerAddr {
		return fmt.Errorf("MsgSend.from_address %q is not the signing key %q", msg.FromAddress, signerAddr)
	}
	if msg.Amount.Empty() {
		return fmt.Errorf("MsgSend.amount is empty")
	}
	if err := msg.Amount.Validate(); err != nil {
		return fmt.Errorf("MsgSend.amount is invalid: %w", err)
	}
	if summary.RecipientOwner != "" && summary.RecipientOwner != msg.ToAddress {
		return fmt.Errorf("summary.recipient_owner %q does not match MsgSend.to_address %q",
			summary.RecipientOwner, msg.ToAddress)
	}
	return whitelist.CheckCosmosRecipient(chainID, msg.ToAddress)
}
