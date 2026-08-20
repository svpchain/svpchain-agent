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
	"github.com/ethereum/go-ethereum/common"

	"github.com/svpchain/svpchain-agent/internal/a2a"
	"github.com/svpchain/svpchain-agent/internal/agent/hitl"
	"github.com/svpchain/svpchain-agent/internal/chainmsgs"
	"github.com/svpchain/svpchain-agent/internal/delegation"
	"github.com/svpchain/svpchain-agent/internal/registry"
)

// defaultDelegationDays is a root delegation's default life.
const defaultDelegationDays = 30

// committingActions are the actions the chain prices and charges against a
// credential's budget. Cancellations commit nothing and need no budget.
var committingActions = map[string]bool{
	"clob.place_order": true,
	// A delegated native EVM transfer reserves its exact asvp value against
	// the credential and root delegation budgets.
	"evm.native_transfer": true,
}

// allQueryActions reports whether every action is a read-only query.* grant —
// the kind that authorizes off-chain reads and never commits value.
func allQueryActions(actions []string) bool {
	for _, a := range actions {
		if !strings.HasPrefix(a, "query.") {
			return false
		}
	}
	return len(actions) > 0
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

// argEVMContracts parses the existing contract/recipient allowlist and keeps
// it in the canonical lowercase form the chain uses for byte-exact matching.
// Native EVM transfers reuse this set as their recipient allowlist.
func argEVMContracts(args map[string]any, key string) ([]string, error) {
	contracts := argStrings(args, key)
	for _, contract := range contracts {
		if !strings.HasPrefix(contract, "0x") || contract != strings.ToLower(contract) || !common.IsHexAddress(contract) {
			return nil, fmt.Errorf("%s entry %q must be a lowercase 0x-prefixed EVM address", key, contract)
		}
	}
	return contracts, nil
}

func argInt64(args map[string]any, key string) int64 {
	if n, ok := args[key].(float64); ok {
		return int64(n)
	}
	return 0
}

// argCoin parses one {"denom":..,"amount":..} object; absent returns a zero
// Coin and no error.
func argCoin(args map[string]any, key string) (registry.Coin, error) {
	m, ok := args[key].(map[string]any)
	if !ok {
		return registry.Coin{}, nil
	}
	denom, _ := m["denom"].(string)
	amount, _ := m["amount"].(string)
	denom, amount = strings.TrimSpace(denom), strings.TrimSpace(amount)
	if denom == "" || amount == "" {
		return registry.Coin{}, fmt.Errorf("%s needs non-empty denom and amount (amount as integer string)", key)
	}
	if _, ok := sdkmath.NewIntFromString(amount); !ok {
		return registry.Coin{}, fmt.Errorf("%s.amount %q is not a base-10 integer", key, amount)
	}
	return registry.Coin{Denom: denom, Amount: amount}, nil
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
	contracts, err := argEVMContracts(args, "contracts")
	if err != nil {
		return "", err
	}
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
	svcTotal, err := argCoins(args, "svc_spend_limit_total")
	if err != nil {
		return "", err
	}
	svcDaily, err := argCoins(args, "svc_spend_limit_daily")
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

	// Root Limits govern chain-side execution only: the keeper checks them
	// when a delegated write lands, but off-chain reads (query.* credentials)
	// are authorized purely from the token chain plus the verifier's
	// epoch/pause heartbeat, so query.* actions never belong in here.
	limits := chainmsgs.Limits{
		SpendLimitTotal:    toSDKCoinList(spendTotal),
		SpendLimitDaily:    toSDKCoinList(spendDaily),
		SvcSpendLimitTotal: toSDKCoinList(svcTotal),
		SvcSpendLimitDaily: toSDKCoinList(svcDaily),
		Denoms:             denoms,
		Actions:            actions,
		Skills:             skills,
		Contracts:          contracts,
		Subaccounts:        subaccounts,
		MaxDepth:           2, // user → executor is depth 1; a redelegable credential uses the one hop of headroom
		MaxTokenTtlSeconds: maxTTL,
	}

	lines := []string{
		"Delegate to: yourself (" + agentID + ") — the root your per-task grants narrow from",
		"Actions: " + strings.Join(actions, ", "),
		"EVM contracts / native-transfer recipients: " + strings.Join(contracts, ", "),
		"Subaccounts: " + fmt.Sprint(subaccounts),
		"Total spend cap: " + coinsText(spendTotal),
		"Daily spend cap: " + coinsText(spendDaily),
		"Service payment cap (total): " + coinsText(svcTotal),
		"Expires: " + time.Unix(expiresAt, 0).UTC().Format(time.RFC3339),
	}
	if err := s.confirm(ctx, ConfirmRequest{
		Kind:  hitl.KindCreateDelegation,
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
		Kind:  hitl.KindResumeDelegation,
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
		Kind:  hitl.KindRevokeDelegation,
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
	contracts, err := argEVMContracts(args, "contracts")
	if err != nil {
		return "", err
	}
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
	// The inverse guard: read-only grants carry no budget. Nothing prices a
	// query.* action, so a budget on one only widens what a leaked credential
	// is worth.
	if len(budget) > 0 && allQueryActions(actions) {
		return "", fmt.Errorf("read-only actions take no budget — remove it, or add the committing action it funds")
	}
	serviceBudget, err := argCoin(args, "service_budget")
	if err != nil {
		return "", err
	}
	payingForService := serviceBudget.Denom != ""
	if payingForService {
		// The paid agent records its spend on subaccount 0 (settlement is
		// bank-level); the grant must say so explicitly, and the user sees
		// both in the dialog below.
		if !containsStr(actions, settlementRecordAction) {
			actions = append(actions, settlementRecordAction)
		}
		if !containsU32(subaccounts, 0) {
			subaccounts = append(subaccounts, 0)
		}
	}
	redelegable, _ := args["redelegable"].(bool)
	redelegateTo := argStrings(args, "redelegate_to")
	if redelegable && len(redelegateTo) == 0 {
		return "", fmt.Errorf(
			"redelegable requires redelegate_to — name the agent DID(s) the credential may be passed on to")
	}
	if !redelegable && len(redelegateTo) > 0 {
		return "", fmt.Errorf("redelegate_to is set but redelegable is false")
	}
	// Every re-delegation target must be a real, active registered agent —
	// the same standard the direct audience is held to below.
	for _, did := range redelegateTo {
		target, err := s.Registry.AgentByID(ctx, did)
		if err != nil {
			return "", fmt.Errorf("redelegate_to agent %s is not registered: %w", did, err)
		}
		if !target.Active() {
			return "", fmt.Errorf("redelegate_to agent %s is not active (status %s)", did, target.Status)
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
	if payingForService {
		// Fail before the user is asked: a root that never granted the
		// settlement action or a service allowance would let the task run and
		// then refuse the agent's payment — the worst place to find out.
		root, err := s.Registry.DelegationByRoot(ctx, rootID)
		if err != nil {
			return "", fmt.Errorf("read root delegation: %w", err)
		}
		if !containsStr(root.Limits.Actions, settlementRecordAction) {
			return "", fmt.Errorf(
				"the root delegation does not grant %s — recreate it with the action (and a svc_spend_limit_total) to pay agents",
				settlementRecordAction)
		}
		if !containsU32(root.Limits.Subaccounts, 0) {
			return "", fmt.Errorf(
				"the root delegation does not cover subaccount 0, which settlement recording is pinned to")
		}
		if len(root.Limits.SvcSpendLimitTotal) == 0 {
			return "", fmt.Errorf(
				"the root delegation has no svc_spend_limit_total — an empty service allowance denies all agent payments")
		}
	}

	ttlShown := ttl
	if ttlShown <= 0 {
		ttlShown = delegation.DefaultTaskTTLSeconds
	}
	lines := []string{
		"Agent: " + agentID + " (" + remote.Endpoint + ")",
		"Task: " + skill + " / " + tool,
		"Actions: " + strings.Join(actions, ", "),
		"EVM contracts / native-transfer recipients: " + strings.Join(contracts, ", "),
		"Subaccounts: " + fmt.Sprint(subaccounts),
		"Budget: " + coinsText(budget),
		fmt.Sprintf("Credential expires in %d seconds; usable only by %s", ttlShown, agentID),
	}
	if redelegable {
		// The one place the user learns the blast radius grew: the executor
		// may hand a narrowed copy of this grant to the listed agents.
		lines = append(lines, fmt.Sprintf(
			"RE-DELEGABLE: %s may pass a narrowed credential to: %s (one further hop, same expiry ceiling)",
			agentID, strings.Join(redelegateTo, ", ")))
	}
	if payingForService {
		lines = append(lines, fmt.Sprintf(
			"SERVICE PAYMENT: %s %s escrowed for this task (plus the chain's payment fee); "+
				"%s may record spend up to that cap and be paid from it. Unspent escrow "+
				"returns when you settle.",
			serviceBudget.Amount, serviceBudget.Denom, agentID))
	}
	if err := s.confirm(ctx, ConfirmRequest{
		Kind:  hitl.KindDelegateTask,
		Title: "Delegate task to " + agentID,
		Lines: lines,
	}); err != nil {
		return "", err
	}

	// The escrow opens only after the user approved the whole grant — an
	// order without a task would strand funds behind an extra settle step.
	var settlementHex string
	var svcBudget []registry.Coin
	if payingForService {
		order, err := s.Lifecycle.OpenSettlement(
			ctx, serviceBudget, "task:"+skill+"/"+tool+" agent:"+agentID)
		if err != nil {
			return "", fmt.Errorf("open settlement escrow: %w", err)
		}
		settlementHex = hex.EncodeToString(order.ID)
		svcBudget = []registry.Coin{serviceBudget}
	}

	proof, err := s.Lifecycle.Mint(ctx, delegation.MintParams{
		RootID:       rootID,
		AudienceDID:  agentID,
		Actions:      actions,
		Skills:       argStrings(args, "skills"),
		Subaccounts:  subaccounts,
		Denoms:       argStrings(args, "denoms"),
		Contracts:    contracts,
		Budget:       budget,
		SvcBudget:    svcBudget,
		Settlement:   settlementHex,
		TTLSeconds:   ttl,
		Now:          s.now(),
		Redelegable:  redelegable,
		RedelegateTo: redelegateTo,
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
	result := map[string]any{
		"agent_id": agentID,
		"task_id":  res.TaskID,
		"state":    res.State,
		"response": res.Response,
	}
	if settlementHex != "" {
		result["settlement_id"] = settlementHex
		result["settlement_note"] = "settle_settlement pays out what the agent recorded and refunds the rest; refund_settlement claws everything unclaimed back"
	}
	return jsonResult(result)
}

// settlementRecordAction is the chain's tag for recording spend against a
// settlement order; byte-identical to the app registry's constant.
const settlementRecordAction = "settlement.record_spend"

func containsStr(haystack []string, needle string) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}
	return false
}

func containsU32(haystack []uint32, needle uint32) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}
	return false
}

// ---- settlement lifecycle tools ----

func (s *Service) listSettlements(ctx context.Context) (string, error) {
	if !s.canDelegate() {
		return "", fmt.Errorf("settlement requires a signing key; import one in the Keys tab")
	}
	orders, err := s.Registry.SettlementsByOpener(ctx, s.Lifecycle.Owner())
	if err != nil {
		return "", err
	}
	out := make([]map[string]any, 0, len(orders))
	for _, o := range orders {
		entry := map[string]any{
			"settlement_id":  hex.EncodeToString(o.ID),
			"status":         o.Status,
			"cap":            o.Cap,
			"total_recorded": o.TotalRecorded,
			"total_claimed":  o.TotalClaimed,
			"refunded":       o.Refunded,
			"memo":           o.Memo,
		}
		if o.Open() {
			if claims, err := s.Registry.ClaimablesBySettlement(ctx, o.ID); err == nil {
				entry["claimables"] = claims
			}
		}
		out = append(out, entry)
	}
	return jsonResult(map[string]any{"settlements": out})
}

func (s *Service) settleSettlement(ctx context.Context, args map[string]any) (string, error) {
	if !s.canDelegate() {
		return "", fmt.Errorf("settlement requires a signing key; import one in the Keys tab")
	}
	id, err := argSettlementID(args)
	if err != nil {
		return "", err
	}
	res, err := s.Lifecycle.SettleSettlement(ctx, id)
	if err != nil {
		return "", err
	}
	return jsonResult(map[string]any{"tx_hash": res.TxHash, "settled": true})
}

func (s *Service) refundSettlement(ctx context.Context, args map[string]any) (string, error) {
	if !s.canDelegate() {
		return "", fmt.Errorf("settlement requires a signing key; import one in the Keys tab")
	}
	id, err := argSettlementID(args)
	if err != nil {
		return "", err
	}
	res, err := s.Lifecycle.RefundSettlement(ctx, id)
	if err != nil {
		return "", err
	}
	return jsonResult(map[string]any{"tx_hash": res.TxHash, "refunded": true})
}

func argSettlementID(args map[string]any) ([]byte, error) {
	hexID := argString(args, "settlement_id")
	if hexID == "" {
		return nil, fmt.Errorf("settlement_id (hex) is required")
	}
	id, err := hex.DecodeString(strings.TrimPrefix(hexID, "0x"))
	if err != nil || len(id) != 32 {
		return nil, fmt.Errorf("settlement_id must be 32 bytes of hex")
	}
	return id, nil
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
