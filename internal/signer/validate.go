package signer

import (
	"fmt"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/cosmos/gogoproto/proto"

	"github.com/svpchain/svpchain-agent/internal/chainmsgs"
	settlementmsgs "github.com/svpchain/svpchain-agent/internal/chainmsgs/settlement"
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

	// x/agentwallet — the user's own delegation lifecycle, built locally by
	// internal/delegation rather than by the remote MCP server. Each is
	// decoded in full below: the delegator must be the signing key.
	"/dydxprotocol.agentwallet.MsgCreateDelegation": {},
	"/dydxprotocol.agentwallet.MsgUpdateDelegation": {},
	"/dydxprotocol.agentwallet.MsgPauseDelegation":  {},
	"/dydxprotocol.agentwallet.MsgResumeDelegation": {},
	"/dydxprotocol.agentwallet.MsgRevokeDelegation": {},
	"/dydxprotocol.agentwallet.MsgRevokeToken":      {},

	// x/settlement — the user's own escrow lifecycle, built locally by
	// internal/delegation. Each is decoded in full below: the opener must be
	// the signing key. MsgRecordSpend and MsgClaim are deliberately absent —
	// recording is the paid agent's move inside MsgAgentExecDelegated, and
	// claiming withdraws an agent operator's earnings; this wallet has no
	// legitimate reason to sign either.
	"/dydxprotocol.settlement.MsgOpenSettlement":   {},
	"/dydxprotocol.settlement.MsgSettle":           {},
	"/dydxprotocol.settlement.MsgRefundSettlement": {},
}

// delegationMsgPrefix marks the agentwallet lifecycle messages.
const delegationMsgPrefix = "/dydxprotocol.agentwallet.Msg"

// settlementMsgPrefix marks the settlement escrow lifecycle messages.
const settlementMsgPrefix = "/dydxprotocol.settlement.Msg"

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
		if strings.HasPrefix(msg.TypeUrl, delegationMsgPrefix) {
			if err := validateDelegationMsg(msg.TypeUrl, msg.Value, signerAddr); err != nil {
				return fmt.Errorf("message %d: %w", i, err)
			}
		}
		if strings.HasPrefix(msg.TypeUrl, settlementMsgPrefix) {
			if err := validateSettlementMsg(msg.TypeUrl, msg.Value, signerAddr); err != nil {
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

// validateDelegationMsg decodes an x/agentwallet lifecycle message and checks
// that the delegator it names is the signing key. A delegation the user did
// not sign for their own account is refused outright — there is no legitimate
// reason for this wallet to sign one for somebody else.
func validateDelegationMsg(typeURL string, value []byte, signerAddr string) error {
	var delegator string
	switch typeURL {
	case "/dydxprotocol.agentwallet.MsgCreateDelegation":
		var msg chainmsgs.MsgCreateDelegation
		if err := proto.Unmarshal(value, &msg); err != nil {
			return fmt.Errorf("decode %s: %w", typeURL, err)
		}
		if msg.AgentId == "" {
			return fmt.Errorf("MsgCreateDelegation.agent_id is empty")
		}
		delegator = msg.Delegator
	case "/dydxprotocol.agentwallet.MsgUpdateDelegation":
		var msg chainmsgs.MsgUpdateDelegation
		if err := proto.Unmarshal(value, &msg); err != nil {
			return fmt.Errorf("decode %s: %w", typeURL, err)
		}
		delegator = msg.Delegator
	case "/dydxprotocol.agentwallet.MsgPauseDelegation":
		var msg chainmsgs.MsgPauseDelegation
		if err := proto.Unmarshal(value, &msg); err != nil {
			return fmt.Errorf("decode %s: %w", typeURL, err)
		}
		delegator = msg.Delegator
	case "/dydxprotocol.agentwallet.MsgResumeDelegation":
		var msg chainmsgs.MsgResumeDelegation
		if err := proto.Unmarshal(value, &msg); err != nil {
			return fmt.Errorf("decode %s: %w", typeURL, err)
		}
		delegator = msg.Delegator
	case "/dydxprotocol.agentwallet.MsgRevokeDelegation":
		var msg chainmsgs.MsgRevokeDelegation
		if err := proto.Unmarshal(value, &msg); err != nil {
			return fmt.Errorf("decode %s: %w", typeURL, err)
		}
		delegator = msg.Delegator
	case "/dydxprotocol.agentwallet.MsgRevokeToken":
		var msg chainmsgs.MsgRevokeToken
		if err := proto.Unmarshal(value, &msg); err != nil {
			return fmt.Errorf("decode %s: %w", typeURL, err)
		}
		delegator = msg.Delegator
	default:
		return fmt.Errorf("unrecognized agentwallet message %q", typeURL)
	}

	if _, err := sdk.AccAddressFromBech32(delegator); err != nil {
		return fmt.Errorf("%s.delegator %q is not a valid address: %w", typeURL, delegator, err)
	}
	if signerAddr != "" && delegator != signerAddr {
		return fmt.Errorf("%s.delegator %q is not the signing key %q", typeURL, delegator, signerAddr)
	}
	return nil
}

// validateSettlementMsg decodes a settlement escrow lifecycle message and
// checks that the opener it names is the signing key: opening escrows the
// signer's own funds, and closing returns them to the same account. An order
// this wallet did not open is refused outright.
func validateSettlementMsg(typeURL string, value []byte, signerAddr string) error {
	var opener string
	switch typeURL {
	case "/dydxprotocol.settlement.MsgOpenSettlement":
		var msg settlementmsgs.MsgOpenSettlement
		if err := proto.Unmarshal(value, &msg); err != nil {
			return fmt.Errorf("decode %s: %w", typeURL, err)
		}
		if !msg.Cap.IsValid() || msg.Cap.IsZero() {
			return fmt.Errorf("MsgOpenSettlement.cap %q is not a positive coin", msg.Cap.String())
		}
		opener = msg.Opener
	case "/dydxprotocol.settlement.MsgSettle":
		var msg settlementmsgs.MsgSettle
		if err := proto.Unmarshal(value, &msg); err != nil {
			return fmt.Errorf("decode %s: %w", typeURL, err)
		}
		opener = msg.Opener
	case "/dydxprotocol.settlement.MsgRefundSettlement":
		var msg settlementmsgs.MsgRefundSettlement
		if err := proto.Unmarshal(value, &msg); err != nil {
			return fmt.Errorf("decode %s: %w", typeURL, err)
		}
		// A slash attribution is a governance move; this wallet never makes it.
		if msg.SlashAgentId != "" {
			return fmt.Errorf("MsgRefundSettlement.slash_agent_id must be empty")
		}
		opener = msg.Opener
	default:
		return fmt.Errorf("unrecognized settlement message %q", typeURL)
	}

	if _, err := sdk.AccAddressFromBech32(opener); err != nil {
		return fmt.Errorf("%s.opener %q is not a valid address: %w", typeURL, opener, err)
	}
	if signerAddr != "" && opener != signerAddr {
		return fmt.Errorf("%s.opener %q is not the signing key %q", typeURL, opener, signerAddr)
	}
	return nil
}
