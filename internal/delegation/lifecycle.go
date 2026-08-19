// Package delegation manages the user's root delegation on chain and mints
// SVP-DT credentials under it. The transaction path is fully local: messages
// are built from vendored chain types, signed by the copied signer through its
// own allowlist, and broadcast over the chain REST endpoint — no remote
// builder is trusted with the user's delegation terms.
package delegation

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"time"

	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/evm/crypto/ethsecp256k1"
	"github.com/cosmos/gogoproto/proto"
	"github.com/google/uuid"

	"github.com/svpchain/svpchain-agent/internal/chainmsgs"
	"github.com/svpchain/svpchain-agent/internal/payload"
	"github.com/svpchain/svpchain-agent/internal/registry"
	"github.com/svpchain/svpchain-agent/internal/signer"
)

// Fee defaults mirror the DEX agent's deployment config: stateful messages on
// this chain carry a flat fee.
const (
	defaultFeeDenom  = "asvp"
	defaultFeeAmount = "25000000000000000"
	defaultGasLimit  = "1000000"
)

// waitForTx polling bounds.
const (
	txPollInterval = 2 * time.Second
	txPollTimeout  = 45 * time.Second
)

// Lifecycle signs and lands the user's delegation transactions.
type Lifecycle struct {
	Registry *registry.Client
	Priv     *ethsecp256k1.PrivKey
	ChainID  string
}

// Owner returns the user's bech32 address.
func (l *Lifecycle) Owner() string {
	return sdk.AccAddress(l.Priv.PubKey().Address()).String()
}

// OwnerDID returns the DID the user's root credentials are issued under.
func (l *Lifecycle) OwnerDID() string {
	return "did:svp:" + l.Owner()
}

// Create lands MsgCreateDelegation and returns the created record. The
// transaction that lands it also publishes the user's pubkey to x/auth, which
// is what lets verifiers resolve the user's root-issuer DID afterwards.
func (l *Lifecycle) Create(
	ctx context.Context,
	agentID string,
	limits chainmsgs.Limits,
	expiresAt int64,
) (registry.Delegation, error) {
	msg := &chainmsgs.MsgCreateDelegation{
		Delegator: l.Owner(),
		AgentId:   agentID,
		Limits:    limits,
		ExpiresAt: expiresAt,
	}
	if _, err := l.submit(ctx, msg, "/dydxprotocol.agentwallet.MsgCreateDelegation"); err != nil {
		return registry.Delegation{}, err
	}

	// The tx response does not surface MsgCreateDelegationResponse.RootId
	// through REST in a form worth parsing; the record is unambiguous by
	// (delegator, agent_id, max height) instead.
	delegations, err := l.Registry.DelegationsByDelegator(ctx, l.Owner())
	if err != nil {
		return registry.Delegation{}, fmt.Errorf("delegation landed but listing it failed: %w", err)
	}
	var newest registry.Delegation
	for _, d := range delegations {
		if d.AgentID == agentID && uint64(d.CreatedAtHeight) >= uint64(newest.CreatedAtHeight) {
			newest = d
		}
	}
	if len(newest.RootID) == 0 {
		return registry.Delegation{}, fmt.Errorf("delegation tx succeeded but the record was not found")
	}
	return newest, nil
}

// Pause lands the user's emergency stop for one delegation.
func (l *Lifecycle) Pause(ctx context.Context, rootID []byte) (registry.TxResult, error) {
	return l.submit(ctx, &chainmsgs.MsgPauseDelegation{
		Delegator: l.Owner(), RootId: rootID,
	}, "/dydxprotocol.agentwallet.MsgPauseDelegation")
}

// Resume lifts a pause. The epoch bumps again, so credentials minted before
// the pause stay dead.
func (l *Lifecycle) Resume(ctx context.Context, rootID []byte) (registry.TxResult, error) {
	return l.submit(ctx, &chainmsgs.MsgResumeDelegation{
		Delegator: l.Owner(), RootId: rootID,
	}, "/dydxprotocol.agentwallet.MsgResumeDelegation")
}

// Revoke removes a delegation permanently.
func (l *Lifecycle) Revoke(ctx context.Context, rootID []byte) (registry.TxResult, error) {
	return l.submit(ctx, &chainmsgs.MsgRevokeDelegation{
		Delegator: l.Owner(), RootId: rootID,
	}, "/dydxprotocol.agentwallet.MsgRevokeDelegation")
}

// submit builds, signs and broadcasts one message, then waits for inclusion.
func (l *Lifecycle) submit(
	ctx context.Context,
	msg proto.Message,
	typeURL string,
) (registry.TxResult, error) {
	if l.Registry == nil || l.Registry.BaseURL() == "" {
		return registry.TxResult{}, fmt.Errorf("chain REST endpoint is not configured")
	}

	value, err := proto.Marshal(msg)
	if err != nil {
		return registry.TxResult{}, fmt.Errorf("marshal %s: %w", typeURL, err)
	}
	body := &txtypes.TxBody{
		Messages: []*codectypes.Any{{TypeUrl: typeURL, Value: value}},
	}
	bodyBytes, err := proto.Marshal(body)
	if err != nil {
		return registry.TxResult{}, fmt.Errorf("marshal tx body: %w", err)
	}

	owner := l.Owner()
	acct, err := l.Registry.Account(ctx, owner)
	if err != nil {
		return registry.TxResult{}, fmt.Errorf("fetch account %s: %w", owner, err)
	}

	p := &payload.TxPayload{
		Version:        payload.CurrentVersion,
		ClientID:       uuid.NewString(),
		ChainID:        l.ChainID,
		SignerAddress:  owner,
		AccountNumber:  strconv.FormatUint(acct.AccountNumber, 10),
		Sequence:       strconv.FormatUint(acct.Sequence, 10),
		TxBodyBytesB64: base64.StdEncoding.EncodeToString(bodyBytes),
		Fee: payload.Fee{
			GasLimit: defaultGasLimit,
			Amount:   []payload.Coin{{Denom: defaultFeeDenom, Amount: defaultFeeAmount}},
		},
		Summary:   payload.Summary{ToolName: "delegation", MsgTypeURL: typeURL},
		ExpiresAt: time.Now().Add(2 * time.Minute),
	}

	signed, err := signer.Sign(l.Priv, p)
	if err != nil {
		return registry.TxResult{}, fmt.Errorf("sign %s: %w", typeURL, err)
	}
	txRaw, err := base64.StdEncoding.DecodeString(signed.TxRawBytesB64)
	if err != nil {
		return registry.TxResult{}, err
	}

	res, err := l.Registry.BroadcastTx(ctx, txRaw)
	if err != nil {
		return registry.TxResult{}, err
	}
	if res.Code != 0 {
		return res, fmt.Errorf("%s rejected (code %d): %s", typeURL, res.Code, res.RawLog)
	}
	return l.waitForTx(ctx, res.TxHash)
}

// waitForTx polls until the transaction executes, and fails on a non-zero
// execution code — a broadcast acceptance is not a result.
func (l *Lifecycle) waitForTx(ctx context.Context, hash string) (registry.TxResult, error) {
	deadline := time.Now().Add(txPollTimeout)
	for {
		res, err := l.Registry.TxByHash(ctx, hash)
		if err == nil && res.Height != "" && res.Height != "0" {
			if res.Code != 0 {
				return res, fmt.Errorf("tx %s failed on chain (code %d): %s", hash, res.Code, res.RawLog)
			}
			return res, nil
		}
		if time.Now().After(deadline) {
			return registry.TxResult{TxHash: hash}, fmt.Errorf(
				"tx %s was accepted but not observed on chain within %s", hash, txPollTimeout,
			)
		}
		select {
		case <-ctx.Done():
			return registry.TxResult{TxHash: hash}, ctx.Err()
		case <-time.After(txPollInterval):
		}
	}
}
