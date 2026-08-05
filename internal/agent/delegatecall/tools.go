package delegatecall

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/svpchain/svpchain-local-agent/internal/a2a"
	"github.com/svpchain/svpchain-local-agent/internal/chainmsgs"
	"github.com/svpchain/svpchain-local-agent/internal/delegation"
	"github.com/svpchain/svpchain-local-agent/internal/registry"
)

// defaultDelegationDays is a root delegation's default life.
const defaultDelegationDays = 30

// committingActions are the actions the chain prices and charges against a
// credential's budget. Cancellations commit nothing and need no budget.
var committingActions = map[string]bool{
	"clob.place_order": true,
}

func (s *Service) now() int64 {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().Unix()
}

// ---- args helpers ----

func argString(args map[string]any, key string) string {
	v, _ := args[key].(string)
	return strings.TrimSpace(v)
}

func argStrings(args map[string]any, key string) []string {
	raw, ok := args[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if str, ok := v.(string); ok && strings.TrimSpace(str) != "" {
			out = append(out, strings.TrimSpace(str))
		}
	}
	return out
}

func argUint32s(args map[string]any, key string) []uint32 {
	raw, ok := args[key].([]any)
	if !ok {
		return nil
	}
	out := make([]uint32, 0, len(raw))
	for _, v := range raw {
		if n, ok := v.(float64); ok && n >= 0 {
			out = append(out, uint32(n))
		}
	}
	return out
}

func argInt64(args map[string]any, key string) int64 {
	if n, ok := args[key].(float64); ok {
		return int64(n)
	}
	return 0
}

// argCoins parses [{"denom":..,"amount":..}] into registry coins.
func argCoins(args map[string]any, key string) ([]registry.Coin, error) {
	raw, ok := args[key].([]any)
	if !ok {
		return nil, nil
	}
	out := make([]registry.Coin, 0, len(raw))
	for i, v := range raw {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s[%d] must be an object with denom and amount", key, i)
		}
		denom, _ := m["denom"].(string)
		amount, _ := m["amount"].(string)
		denom, amount = strings.TrimSpace(denom), strings.TrimSpace(amount)
		if denom == "" || amount == "" {
			return nil, fmt.Errorf("%s[%d] needs non-empty denom and amount (amount as integer string)", key, i)
		}
		if _, ok := sdkmath.NewIntFromString(amount); !ok {
			return nil, fmt.Errorf("%s[%d].amount %q is not a base-10 integer", key, i, amount)
		}
		out = append(out, registry.Coin{Denom: denom, Amount: amount})
	}
	return out, nil
}

func argRootID(args map[string]any) ([]byte, error) {
	hexID := argString(args, "root_id")
	if hexID == "" {
		return nil, fmt.Errorf("root_id (hex) is required")
	}
	rootID, err := hex.DecodeString(strings.TrimPrefix(hexID, "0x"))
	if err != nil || len(rootID) != 32 {
		return nil, fmt.Errorf("root_id must be 32 bytes of hex")
	}
	return rootID, nil
}

func coinsText(coins []registry.Coin) string {
	if len(coins) == 0 {
		return "(none)"
	}
	parts := make([]string, 0, len(coins))
	for _, c := range coins {
		parts = append(parts, c.Amount+" "+c.Denom)
	}
	return strings.Join(parts, ", ")
}

// toSDKCoinList converts wire coins into the chain's coin type for Limits.
// Amounts were validated as integer strings by argCoins.
func toSDKCoinList(coins []registry.Coin) []sdk.Coin {
	out := make([]sdk.Coin, 0, len(coins))
	for _, c := range coins {
		amount, ok := sdkmath.NewIntFromString(c.Amount)
		if !ok {
			continue
		}
		out = append(out, sdk.Coin{Denom: c.Denom, Amount: amount})
	}
	return out
}

func jsonResult(v any) (string, error) {
	out, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// ---- delegation lifecycle tools ----

type delegationRow struct {
	RootIDHex string          `json:"root_id"`
	AgentID   string          `json:"agent_id"`
	Limits    registry.Limits `json:"limits"`
	Epoch     uint64          `json:"epoch"`
	Paused    bool            `json:"paused"`
	ExpiresAt int64           `json:"expires_at"`
	Spend     any             `json:"spend,omitempty"`
}

func (s *Service) listDelegations(ctx context.Context) (string, error) {
	if !s.canDelegate() {
		return "", fmt.Errorf("delegation requires a signing key; import one in the Keys tab")
	}
	delegations, err := s.Registry.DelegationsByDelegator(ctx, s.Lifecycle.Owner())
	if err != nil {
		return "", err
	}
	rows := make([]delegationRow, 0, len(delegations))
	for _, d := range delegations {
		row := delegationRow{
			RootIDHex: hex.EncodeToString(d.RootID),
			AgentID:   d.AgentID,
			Limits:    d.Limits,
			Epoch:     uint64(d.Epoch),
			Paused:    d.Paused,
			ExpiresAt: int64(d.ExpiresAt),
		}
		if spend, err := s.Registry.SpendByRoot(ctx, d.RootID); err == nil {
			row.Spend = spend
		}
		rows = append(rows, row)
	}
	return jsonResult(map[string]any{
		"delegator":   s.Lifecycle.Owner(),
		"issuer_did":  s.Lifecycle.OwnerDID(),
		"delegations": rows,
	})
}

func (s *Service) createRootDelegation(ctx context.Context, args map[string]any) (string, error) {
	if !s.canDelegate() {
		return "", fmt.Errorf("delegation requires a signing key; import one in the Keys tab")
	}

	// The root delegation is to the user's own DID: the user is the root
	// issuer, and per-task credentials narrow from here.
	agentID := s.Lifecycle.OwnerDID()

	actions := argStrings(args, "actions")
	skills := argStrings(args, "skills")
	subaccounts := argUint32s(args, "subaccounts")
	denoms := argStrings(args, "denoms")
	if len(actions) == 0 {
		return "", fmt.Errorf("actions is required — empty grants deny everything (e.g. [\"clob.place_order\"])")
	}
	if len(subaccounts) == 0 {
		return "", fmt.Errorf("subaccounts is required — list the subaccount numbers the delegation may touch")
	}

	spendTotal, err := argCoins(args, "spend_limit_total")
	if err != nil {
		return "", err
	}
	spendDaily, err := argCoins(args, "spend_limit_daily")
	if err != nil {
		return "", err
	}
	if len(spendTotal) == 0 {
		return "", fmt.Errorf("spend_limit_total is required — an unbounded delegation is refused")
	}

	expiresAt := argInt64(args, "expires_at")
	if expiresAt == 0 {
		expiresAt = s.now() + defaultDelegationDays*86_400
	}

	params, err := s.Registry.WalletParams(ctx)
	if err != nil {
		return "", fmt.Errorf("read agentwallet params: %w", err)
	}
	maxTTL := uint32(delegation.DefaultTaskTTLSeconds)
	if p := uint32(params.MaxTokenTtlSeconds); p > 0 && p < maxTTL {
		maxTTL = p
	}

	limits := chainmsgs.Limits{
		SpendLimitTotal:    toSDKCoinList(spendTotal),
		SpendLimitDaily:    toSDKCoinList(spendDaily),
		Denoms:             denoms,
		Actions:            actions,
		Skills:             skills,
		Subaccounts:        subaccounts,
		MaxDepth:           2, // user → executor is depth 1; leave one hop of headroom
		MaxTokenTtlSeconds: maxTTL,
	}

	lines := []string{
		"Delegate to: yourself (" + agentID + ") — the root your per-task grants narrow from",
		"Actions: " + strings.Join(actions, ", "),
		"Subaccounts: " + fmt.Sprint(subaccounts),
		"Total spend cap: " + coinsText(spendTotal),
		"Daily spend cap: " + coinsText(spendDaily),
		"Expires: " + time.Unix(expiresAt, 0).UTC().Format(time.RFC3339),
	}
	if err := s.confirm(ctx, ConfirmRequest{
		Kind:  "create_delegation",
		Title: "Create root delegation",
		Lines: lines,
	}); err != nil {
		return "", err
	}

	created, err := s.Lifecycle.Create(ctx, agentID, limits, expiresAt)
	if err != nil {
		return "", err
	}
	return jsonResult(map[string]any{
		"root_id":    hex.EncodeToString(created.RootID),
		"agent_id":   created.AgentID,
		"epoch":      uint64(created.Epoch),
		"expires_at": int64(created.ExpiresAt),
	})
}

func (s *Service) pauseDelegation(ctx context.Context, args map[string]any) (string, error) {
	if !s.canDelegate() {
		return "", fmt.Errorf("delegation requires a signing key; import one in the Keys tab")
	}
	rootID, err := argRootID(args)
	if err != nil {
		return "", err
	}
	// The emergency stop deliberately skips confirmation: pausing only ever
	// removes authority.
	res, err := s.Lifecycle.Pause(ctx, rootID)
	if err != nil {
		return "", err
	}
	return jsonResult(map[string]any{"tx_hash": res.TxHash, "paused": true})
}

func (s *Service) resumeDelegation(ctx context.Context, args map[string]any) (string, error) {
	if !s.canDelegate() {
		return "", fmt.Errorf("delegation requires a signing key; import one in the Keys tab")
	}
	rootID, err := argRootID(args)
	if err != nil {
		return "", err
	}
	if err := s.confirm(ctx, ConfirmRequest{
		Kind:  "resume_delegation",
		Title: "Resume delegation",
		Lines: []string{"Root: " + hex.EncodeToString(rootID), "Resuming re-enables delegated spending under this grant."},
	}); err != nil {
		return "", err
	}
	res, err := s.Lifecycle.Resume(ctx, rootID)
	if err != nil {
		return "", err
	}
	return jsonResult(map[string]any{"tx_hash": res.TxHash, "paused": false})
}

func (s *Service) revokeDelegation(ctx context.Context, args map[string]any) (string, error) {
	if !s.canDelegate() {
		return "", fmt.Errorf("delegation requires a signing key; import one in the Keys tab")
	}
	rootID, err := argRootID(args)
	if err != nil {
		return "", err
	}
	if err := s.confirm(ctx, ConfirmRequest{
		Kind:  "revoke_delegation",
		Title: "Revoke delegation",
		Lines: []string{"Root: " + hex.EncodeToString(rootID), "Revocation is permanent; a new delegation must be created to delegate again."},
	}); err != nil {
		return "", err
	}
	res, err := s.Lifecycle.Revoke(ctx, rootID)
	if err != nil {
		return "", err
	}
	return jsonResult(map[string]any{"tx_hash": res.TxHash, "revoked": true})
}

// ---- delegate_task ----

func (s *Service) delegateTask(ctx context.Context, args map[string]any) (string, error) {
	if !s.canDelegate() {
		return "", fmt.Errorf("delegation requires a signing key; import one in the Keys tab")
	}

	agentID := argString(args, "agent_id")
	skill := argString(args, "skill")
	tool := argString(args, "tool")
	if agentID == "" || skill == "" || tool == "" {
		return "", fmt.Errorf("agent_id, skill and tool are required")
	}
	taskArgs, _ := args["args"].(map[string]any)
	if taskArgs == nil {
		taskArgs = map[string]any{}
	}

	actions := argStrings(args, "actions")
	subaccounts := argUint32s(args, "subaccounts")
	budget, err := argCoins(args, "budget")
	if err != nil {
		return "", err
	}
	if len(actions) == 0 {
		return "", fmt.Errorf("actions is required — the credential grants nothing by default (e.g. [\"clob.place_order\"])")
	}
	// A credential with no budget cannot commit value: the chain prices the
	// action and refuses it against an empty per-action budget. Caught here so
	// a doomed credential never costs the user a confirmation dialog.
	if len(budget) == 0 {
		for _, a := range actions {
			if committingActions[a] {
				return "", fmt.Errorf(
					"action %q commits value, so budget is required — the chain refuses it against an empty budget", a,
				)
			}
		}
	}
	ttl := argInt64(args, "expires_in_seconds")

	// Resolve the executor from the registry — the credential is addressed to
	// the DID the chain knows, not to whatever a card claims.
	remote, err := s.Registry.AgentByID(ctx, agentID)
	if err != nil {
		return "", fmt.Errorf("agent %s is not registered: %w", agentID, err)
	}
	if !remote.Active() {
		return "", fmt.Errorf("agent %s is not active (status %s)", agentID, remote.Status)
	}
	if strings.TrimSpace(remote.Endpoint) == "" {
		return "", fmt.Errorf("agent %s registered no endpoint", agentID)
	}

	rootID, err := s.resolveRoot(ctx, args)
	if err != nil {
		return "", err
	}

	ttlShown := ttl
	if ttlShown <= 0 {
		ttlShown = delegation.DefaultTaskTTLSeconds
	}
	if err := s.confirm(ctx, ConfirmRequest{
		Kind:  "delegate_task",
		Title: "Delegate task to " + agentID,
		Lines: []string{
			"Agent: " + agentID + " (" + remote.Endpoint + ")",
			"Task: " + skill + " / " + tool,
			"Actions: " + strings.Join(actions, ", "),
			"Subaccounts: " + fmt.Sprint(subaccounts),
			"Budget: " + coinsText(budget),
			fmt.Sprintf("Credential expires in %d seconds, single use", ttlShown),
		},
	}); err != nil {
		return "", err
	}

	proof, err := s.Lifecycle.Mint(ctx, delegation.MintParams{
		RootID:      rootID,
		AudienceDID: agentID,
		Actions:     actions,
		Skills:      argStrings(args, "skills"),
		Subaccounts: subaccounts,
		Denoms:      argStrings(args, "denoms"),
		Budget:      budget,
		TTLSeconds:  ttl,
		Now:         s.now(),
	})
	if err != nil {
		return "", err
	}

	envelope, err := json.Marshal(map[string]any{
		"skill": skill,
		"tool":  tool,
		"args":  taskArgs,
	})
	if err != nil {
		return "", err
	}

	// The credential rides the message metadata (the delegation extension's
	// carrier), not the envelope args, so the callee can verify it before
	// parsing any skill-specific request.
	res, err := a2a.SendToAgentWithDelegation(ctx, remote.Endpoint, string(envelope), proof)
	if err != nil {
		return "", fmt.Errorf("send task to %s: %w", agentID, err)
	}
	return jsonResult(map[string]any{
		"agent_id": agentID,
		"task_id":  res.TaskID,
		"state":    res.State,
		"response": res.Response,
	})
}

// resolveRoot picks the delegation the credential descends from: an explicit
// root_id argument, or the user's newest usable self-delegation.
func (s *Service) resolveRoot(ctx context.Context, args map[string]any) ([]byte, error) {
	if argString(args, "root_id") != "" {
		return argRootID(args)
	}
	delegations, err := s.Registry.DelegationsByDelegator(ctx, s.Lifecycle.Owner())
	if err != nil {
		return nil, err
	}
	ownDID := s.Lifecycle.OwnerDID()
	now := s.now()
	var best registry.Delegation
	for _, d := range delegations {
		if d.AgentID != ownDID || d.Paused || int64(d.ExpiresAt) <= now {
			continue
		}
		if uint64(d.CreatedAtHeight) >= uint64(best.CreatedAtHeight) {
			best = d
		}
	}
	if len(best.RootID) == 0 {
		return nil, fmt.Errorf(
			"no usable root delegation found — create one first with create_root_delegation",
		)
	}
	return best.RootID, nil
}
