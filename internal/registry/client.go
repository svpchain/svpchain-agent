// Package registry is the local agent's read/broadcast window onto the chain:
// x/agent discovery, x/agentwallet delegation state, x/auth account facts and
// transaction broadcast — all over the configurable REST (grpc-gateway)
// endpoint, with no cosmos-sdk client machinery.
package registry

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// cacheTTL bounds how stale discovery answers may be. Mirrors the DEX agent's
// params cache: registry records change rarely, refetching per call is pure
// latency, and a minute bounds how long a deregistration goes unnoticed.
const cacheTTL = 60 * time.Second

// maxBodyBytes bounds any single REST or card response.
const maxBodyBytes = 4 << 20

// Client queries one chain REST endpoint.
type Client struct {
	base string
	http *http.Client

	mu    sync.Mutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	at   time.Time
	body []byte
}

// New builds a client for a REST base URL such as "https://dev02.svpchain.org".
func New(baseURL string) *Client {
	return &Client{
		base:  strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		http:  &http.Client{Timeout: 15 * time.Second},
		cache: map[string]cacheEntry{},
	}
}

// BaseURL reports the endpoint this client is bound to.
func (c *Client) BaseURL() string { return c.base }

func (c *Client) getJSON(ctx context.Context, path string, cached bool, out any) error {
	if c.base == "" {
		return fmt.Errorf("chain REST endpoint is not configured")
	}
	full := c.base + path

	if cached {
		c.mu.Lock()
		if e, ok := c.cache[full]; ok && time.Since(e.at) < cacheTTL {
			body := e.body
			c.mu.Unlock()
			return json.Unmarshal(body, out)
		}
		c.mu.Unlock()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("chain REST %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("chain REST %s: %s: %s", path, resp.Status, restErrorMessage(body))
	}
	if cached {
		c.mu.Lock()
		c.cache[full] = cacheEntry{at: time.Now(), body: body}
		c.mu.Unlock()
	}
	return json.Unmarshal(body, out)
}

func (c *Client) postJSON(ctx context.Context, path string, in, out any) error {
	if c.base == "" {
		return fmt.Errorf("chain REST endpoint is not configured")
	}
	payload, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, c.base+path, bytes.NewReader(payload),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("chain REST %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("chain REST %s: %s: %s", path, resp.Status, restErrorMessage(body))
	}
	return json.Unmarshal(body, out)
}

// restErrorMessage extracts the gateway's error message, falling back to a
// trimmed body so an nginx HTML page doesn't flood a tool result.
func restErrorMessage(body []byte) string {
	var e struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &e); err == nil && e.Message != "" {
		return e.Message
	}
	s := strings.TrimSpace(string(body))
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return s
}

// ---- x/agent discovery ----

// Agents lists registry records, optionally filtered to one capability tag.
// Only ACTIVE agents are returned: those are the only ones a delegation can
// execute through, so anything else is noise to a discovery caller.
func (c *Client) Agents(ctx context.Context, capability string) ([]Agent, error) {
	var (
		path string
		out  struct {
			Agents []Agent `json:"agents"`
		}
	)
	if capability = strings.TrimSpace(capability); capability != "" {
		path = "/dydxprotocol/agent/agents_by_capability/" + url.PathEscape(capability) +
			"?active_only=true&pagination.limit=200"
	} else {
		path = "/dydxprotocol/agent/agents?pagination.limit=200"
	}
	if err := c.getJSON(ctx, path, true, &out); err != nil {
		return nil, err
	}
	agents := make([]Agent, 0, len(out.Agents))
	for _, a := range out.Agents {
		if a.Active() {
			agents = append(agents, a)
		}
	}
	return agents, nil
}

// AgentByID fetches one registry record.
func (c *Client) AgentByID(ctx context.Context, agentID string) (Agent, error) {
	var out struct {
		Agent Agent `json:"agent"`
	}
	err := c.getJSON(
		ctx, "/dydxprotocol/agent/agent/"+url.PathEscape(agentID), true, &out,
	)
	return out.Agent, err
}

// Card is a fetched A2A agent card plus its integrity check against the
// capability hash the agent registered on chain.
type Card struct {
	// Raw is the card exactly as served.
	Raw json.RawMessage `json:"card"`
	// Verified reports whether sha256(Raw) matches the registered
	// capability_hash. False means the served card has drifted from what was
	// registered — worth surfacing, not necessarily fatal.
	Verified bool `json:"verified"`
}

// FetchCard retrieves an agent's A2A card from its registered endpoint and
// checks it against the on-chain capability hash.
func (c *Client) FetchCard(ctx context.Context, agent Agent) (Card, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(agent.Endpoint), "/")
	if endpoint == "" {
		return Card{}, fmt.Errorf("agent %s registered no endpoint", agent.AgentID)
	}
	cardURL := endpoint + "/.well-known/agent-card.json"

	c.mu.Lock()
	if e, ok := c.cache[cardURL]; ok && time.Since(e.at) < cacheTTL {
		body := e.body
		c.mu.Unlock()
		return cardFromBody(body, agent.CapabilityHash)
	}
	c.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cardURL, nil)
	if err != nil {
		return Card{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return Card{}, fmt.Errorf("fetch card %s: %w", cardURL, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return Card{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Card{}, fmt.Errorf("fetch card %s: %s", cardURL, resp.Status)
	}
	if !json.Valid(body) {
		return Card{}, fmt.Errorf("card at %s is not JSON", cardURL)
	}

	c.mu.Lock()
	c.cache[cardURL] = cacheEntry{at: time.Now(), body: body}
	c.mu.Unlock()

	return cardFromBody(body, agent.CapabilityHash)
}

func cardFromBody(body, wantHash []byte) (Card, error) {
	sum := sha256.Sum256(body)
	return Card{
		Raw:      json.RawMessage(body),
		Verified: len(wantHash) == 32 && bytes.Equal(sum[:], wantHash),
	}, nil
}

// ---- x/agentwallet delegation state ----

// rootIDPath encodes a root id the way grpc-gateway expects bytes in a path.
func rootIDPath(rootID []byte) string {
	return url.PathEscape(base64.URLEncoding.EncodeToString(rootID))
}

// DelegationsByDelegator lists a user's root delegations.
func (c *Client) DelegationsByDelegator(ctx context.Context, delegator string) ([]Delegation, error) {
	var out struct {
		Delegations []Delegation `json:"delegations"`
	}
	err := c.getJSON(
		ctx,
		"/dydxprotocol/agentwallet/delegations_by_delegator/"+url.PathEscape(delegator)+
			"?pagination.limit=200",
		false, &out,
	)
	return out.Delegations, err
}

// DelegationByRoot fetches one delegation record.
func (c *Client) DelegationByRoot(ctx context.Context, rootID []byte) (Delegation, error) {
	var out struct {
		Delegation Delegation `json:"delegation"`
	}
	err := c.getJSON(
		ctx, "/dydxprotocol/agentwallet/delegation/"+rootIDPath(rootID), false, &out,
	)
	return out.Delegation, err
}

// Epoch reads a delegation's revocation heartbeat. Never cached: this is the
// value a credential must carry at mint time, and a stale read mints a token
// the chain rejects.
func (c *Client) Epoch(ctx context.Context, rootID []byte) (epoch uint64, paused bool, err error) {
	var out struct {
		Epoch  FlexUint64 `json:"epoch"`
		Paused bool       `json:"paused"`
	}
	err = c.getJSON(
		ctx, "/dydxprotocol/agentwallet/epoch/"+rootIDPath(rootID), false, &out,
	)
	return uint64(out.Epoch), out.Paused, err
}

// SpendState is a delegation's consumption against its caps.
type SpendState struct {
	Lifetime Spend `json:"lifetime"`
	Daily    Spend `json:"daily"`
}

// SpendByRoot reads a delegation's spend ledgers.
func (c *Client) SpendByRoot(ctx context.Context, rootID []byte) (SpendState, error) {
	var out SpendState
	err := c.getJSON(
		ctx, "/dydxprotocol/agentwallet/spend/"+rootIDPath(rootID), false, &out,
	)
	return out, err
}

// WalletParams reads the x/agentwallet ceilings. Cached: they change only by
// governance.
func (c *Client) WalletParams(ctx context.Context) (WalletParams, error) {
	var out struct {
		Params WalletParams `json:"params"`
	}
	err := c.getJSON(ctx, "/dydxprotocol/agentwallet/params", true, &out)
	return out.Params, err
}

// ---- x/auth + tx ----

// Account reads the number and sequence signing a transaction needs. The
// account payload's shape depends on the account type (BaseAccount vs an EVM
// wrapper), so the fields are found wherever they nest.
func (c *Client) Account(ctx context.Context, address string) (AccountInfo, error) {
	var out struct {
		Account json.RawMessage `json:"account"`
	}
	if err := c.getJSON(
		ctx, "/cosmos/auth/v1beta1/accounts/"+url.PathEscape(address), false, &out,
	); err != nil {
		return AccountInfo{}, err
	}
	info, ok := findAccountInfo(out.Account)
	if !ok {
		return AccountInfo{}, fmt.Errorf("account %s: no account_number in response", address)
	}
	return info, nil
}

func findAccountInfo(raw json.RawMessage) (AccountInfo, bool) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return AccountInfo{}, false
	}
	if numRaw, ok := m["account_number"]; ok {
		var num, seq FlexUint64
		if err := json.Unmarshal(numRaw, &num); err != nil {
			return AccountInfo{}, false
		}
		if seqRaw, ok := m["sequence"]; ok {
			_ = json.Unmarshal(seqRaw, &seq)
		}
		return AccountInfo{AccountNumber: uint64(num), Sequence: uint64(seq)}, true
	}
	// EthAccount and vesting wrappers nest the base account one level down.
	for _, key := range []string{"base_account", "base_vesting_account"} {
		if inner, ok := m[key]; ok {
			if info, ok := findAccountInfo(inner); ok {
				return info, true
			}
		}
	}
	return AccountInfo{}, false
}

// BroadcastTx submits signed transaction bytes in sync mode and returns the
// CheckTx outcome. Code 0 means accepted into the mempool, not executed.
func (c *Client) BroadcastTx(ctx context.Context, txBytes []byte) (TxResult, error) {
	in := map[string]string{
		"tx_bytes": base64.StdEncoding.EncodeToString(txBytes),
		"mode":     "BROADCAST_MODE_SYNC",
	}
	var out struct {
		TxResponse TxResult `json:"tx_response"`
	}
	if err := c.postJSON(ctx, "/cosmos/tx/v1beta1/txs", in, &out); err != nil {
		return TxResult{}, err
	}
	return out.TxResponse, nil
}

// TxByHash looks a transaction up, for polling execution results.
func (c *Client) TxByHash(ctx context.Context, hash string) (TxResult, error) {
	var out struct {
		TxResponse TxResult `json:"tx_response"`
	}
	err := c.getJSON(
		ctx, "/cosmos/tx/v1beta1/txs/"+url.PathEscape(hash), false, &out,
	)
	return out.TxResponse, err
}
