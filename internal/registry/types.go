package registry

import (
	"encoding/json"
	"strconv"
)

// FlexUint64 tolerates the two encodings grpc-gateway REST produces for
// integer fields: uint64 arrives as a JSON string, uint32 as a number.
type FlexUint64 uint64

func (f *FlexUint64) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		if s == "" {
			*f = 0
			return nil
		}
		v, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return err
		}
		*f = FlexUint64(v)
		return nil
	}
	var n uint64
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*f = FlexUint64(n)
	return nil
}

// FlexInt64 is FlexUint64's signed twin, for unix-second timestamps.
type FlexInt64 int64

func (f *FlexInt64) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		if s == "" {
			*f = 0
			return nil
		}
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return err
		}
		*f = FlexInt64(v)
		return nil
	}
	var n int64
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*f = FlexInt64(n)
	return nil
}

// Coin is a chain coin with the amount kept as its decimal string.
type Coin struct {
	Denom  string `json:"denom"`
	Amount string `json:"amount"`
}

// Pricing is an agent's advertised price, informational only.
type Pricing struct {
	PerCall []Coin `json:"per_call"`
	Unit    string `json:"unit"`
}

// Agent is one x/agent registry record. []byte fields arrive base64-encoded
// in REST JSON, which encoding/json decodes natively.
type Agent struct {
	AgentID        string   `json:"agent_id"`
	Owner          string   `json:"owner"`
	Operator       string   `json:"operator"`
	PublicKey      []byte   `json:"public_key"`
	Endpoint       string   `json:"endpoint"`
	CapabilityHash []byte   `json:"capability_hash"`
	Capabilities   []string `json:"capabilities"`
	Pricing        Pricing  `json:"pricing"`
	Bond           Coin     `json:"bond"`
	Status         string   `json:"status"`
	Metadata       string   `json:"metadata"`
}

// Active reports whether the agent may currently act for delegators.
func (a Agent) Active() bool {
	return a.Status == "AGENT_STATUS_ACTIVE"
}

// Limits is the on-chain ceiling of a delegation.
type Limits struct {
	SpendLimitTotal    []Coin     `json:"spend_limit_total"`
	SpendLimitDaily    []Coin     `json:"spend_limit_daily"`
	SvcSpendLimitTotal []Coin     `json:"svc_spend_limit_total"`
	SvcSpendLimitDaily []Coin     `json:"svc_spend_limit_daily"`
	Denoms             []string   `json:"denoms"`
	Actions            []string   `json:"actions"`
	Skills             []string   `json:"skills"`
	Contracts          []string   `json:"contracts"`
	Subaccounts        []uint32   `json:"subaccounts"`
	MaxDepth           uint32     `json:"max_depth"`
	MaxTokenTtlSeconds FlexUint64 `json:"max_token_ttl_seconds"`
}

// Delegation is one x/agentwallet root delegation record.
type Delegation struct {
	RootID          []byte     `json:"root_id"`
	Delegator       string     `json:"delegator"`
	AgentID         string     `json:"agent_id"`
	Limits          Limits     `json:"limits"`
	Epoch           FlexUint64 `json:"epoch"`
	Paused          bool       `json:"paused"`
	ExpiresAt       FlexInt64  `json:"expires_at"`
	CreatedAtHeight FlexUint64 `json:"created_at_height"`
}

// Settlement is one x/settlement escrow order record.
type Settlement struct {
	ID               []byte     `json:"id"`
	Opener           string     `json:"opener"`
	Cap              Coin       `json:"cap"`
	FeePaid          Coin       `json:"fee_paid"`
	TotalRecorded    Coin       `json:"total_recorded"`
	TotalClaimed     Coin       `json:"total_claimed"`
	Refunded         Coin       `json:"refunded"`
	Status           string     `json:"status"`
	RootDelegationID []byte     `json:"root_delegation_id"`
	CreatedAtHeight  FlexUint64 `json:"created_at_height"`
	Memo             string     `json:"memo"`
}

// Open reports whether the order still accepts records.
func (s Settlement) Open() bool {
	return s.Status == "SETTLEMENT_STATUS_OPEN"
}

// Claimable is one agent's unclaimed accrual on one settlement order.
type Claimable struct {
	SettlementID []byte `json:"settlement_id"`
	AgentID      string `json:"agent_id"`
	Amount       Coin   `json:"amount"`
}

// Spend is a delegation's consumed budget in one bucket.
type Spend struct {
	Spent    []Coin `json:"spent"`
	SvcSpent []Coin `json:"svc_spent"`
}

// WalletParams are the x/agentwallet ceilings a credential must respect.
type WalletParams struct {
	MaxDelegationDepth       uint32     `json:"max_delegation_depth"`
	MaxTokenTtlSeconds       FlexUint64 `json:"max_token_ttl_seconds"`
	MaxProofTokens           uint32     `json:"max_proof_tokens"`
	NonceRetentionBlocks     FlexUint64 `json:"nonce_retention_blocks"`
	MaxDelegationsPerAccount FlexUint64 `json:"max_delegations_per_account"`
}

// AccountInfo is the x/auth subset transaction signing needs.
type AccountInfo struct {
	AccountNumber uint64
	Sequence      uint64
}

// TxResult is a broadcast or lookup outcome.
type TxResult struct {
	TxHash string      `json:"txhash"`
	Code   uint32      `json:"code"`
	RawLog string      `json:"raw_log"`
	Height string      `json:"height"`
	Events []AbciEvent `json:"events,omitempty"`
	Logs   []TxLog     `json:"logs,omitempty"`
}

// AbciEvent is one Tendermint/ABCI event from a tx response.
type AbciEvent struct {
	Type       string          `json:"type"`
	Attributes []AbciAttribute `json:"attributes"`
}

// AbciAttribute is one event key/value.
type AbciAttribute struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Index bool   `json:"index,omitempty"`
}

// TxLog is one message log inside tx_response.logs.
type TxLog struct {
	Events []AbciEvent `json:"events"`
}

// AllEvents prefers top-level tx_response.events and falls back to logs[].events.
func (r TxResult) AllEvents() []AbciEvent {
	if len(r.Events) > 0 {
		return r.Events
	}
	var out []AbciEvent
	for _, log := range r.Logs {
		out = append(out, log.Events...)
	}
	return out
}
