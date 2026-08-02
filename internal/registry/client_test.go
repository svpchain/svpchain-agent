package registry

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

const cardBody = `{"name":"svpchain-dex-agent","protocolVersion":"1.0"}`

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	cardHash := sha256.Sum256([]byte(cardBody))

	mux := http.NewServeMux()
	var cardEndpoint string

	agentJSON := func() string {
		return fmt.Sprintf(`{
			"agent_id": "did:svp:svp1operator",
			"owner": "svp1owner",
			"operator": "svp1operator",
			"public_key": %q,
			"endpoint": %q,
			"capability_hash": %q,
			"capabilities": ["trading","market-data"],
			"pricing": {"per_call": [{"denom":"asvp","amount":"100"}], "unit": "call"},
			"bond": {"denom":"asvp","amount":"5000000000000000000000"},
			"status": "AGENT_STATUS_ACTIVE",
			"metadata": "dex",
			"registered_at_height": "42"
		}`,
			base64.StdEncoding.EncodeToString(make([]byte, 33)),
			cardEndpoint,
			base64.StdEncoding.EncodeToString(cardHash[:]),
		)
	}

	mux.HandleFunc("/dydxprotocol/agent/agents", func(w http.ResponseWriter, r *http.Request) {
		suspended := `{"agent_id":"did:svp:svp1sleepy","status":"AGENT_STATUS_SUSPENDED"}`
		fmt.Fprintf(w, `{"agents":[%s,%s],"pagination":{"total":"2"}}`, agentJSON(), suspended)
	})
	mux.HandleFunc("/dydxprotocol/agent/agents_by_capability/trading", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "true", r.URL.Query().Get("active_only"))
		fmt.Fprintf(w, `{"agents":[%s]}`, agentJSON())
	})
	mux.HandleFunc("/dydxprotocol/agent/agent/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"agent":%s}`, agentJSON())
	})
	mux.HandleFunc("/.well-known/agent-card.json", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, cardBody)
	})
	mux.HandleFunc("/dydxprotocol/agentwallet/epoch/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"epoch":"3","paused":false}`)
	})
	mux.HandleFunc("/dydxprotocol/agentwallet/params", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"params":{"max_delegation_depth":4,"max_token_ttl_seconds":"600","max_proof_tokens":4}}`)
	})
	mux.HandleFunc("/cosmos/auth/v1beta1/accounts/", func(w http.ResponseWriter, r *http.Request) {
		// The EVM chain wraps BaseAccount one level down.
		fmt.Fprint(w, `{"account":{"@type":"/cosmos.evm.types.v1.EthAccount",
			"base_account":{"address":"svp1user","account_number":"7","sequence":"3"}}}`)
	})
	mux.HandleFunc("/cosmos/tx/v1beta1/txs", func(w http.ResponseWriter, r *http.Request) {
		var in map[string]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&in))
		require.Equal(t, "BROADCAST_MODE_SYNC", in["mode"])
		fmt.Fprint(w, `{"tx_response":{"txhash":"ABC123","code":0,"raw_log":""}}`)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	cardEndpoint = srv.URL
	return srv
}

func TestAgentsFiltersToActive(t *testing.T) {
	srv := newTestServer(t)
	c := New(srv.URL)

	agents, err := c.Agents(context.Background(), "")
	require.NoError(t, err)
	require.Len(t, agents, 1)
	require.Equal(t, "did:svp:svp1operator", agents[0].AgentID)
	require.Equal(t, []string{"trading", "market-data"}, agents[0].Capabilities)
	require.Len(t, agents[0].PublicKey, 33)
}

func TestAgentsByCapability(t *testing.T) {
	srv := newTestServer(t)
	c := New(srv.URL)

	agents, err := c.Agents(context.Background(), "trading")
	require.NoError(t, err)
	require.Len(t, agents, 1)
}

func TestFetchCardVerifiesTheRegisteredHash(t *testing.T) {
	srv := newTestServer(t)
	c := New(srv.URL)

	agent, err := c.AgentByID(context.Background(), "did:svp:svp1operator")
	require.NoError(t, err)

	card, err := c.FetchCard(context.Background(), agent)
	require.NoError(t, err)
	require.True(t, card.Verified)
	require.JSONEq(t, cardBody, string(card.Raw))

	// A drifted hash must surface as unverified, not as an error.
	agent.CapabilityHash[0] ^= 0xFF
	c2 := New(srv.URL)
	card, err = c2.FetchCard(context.Background(), agent)
	require.NoError(t, err)
	require.False(t, card.Verified)
}

func TestEpochAndParams(t *testing.T) {
	srv := newTestServer(t)
	c := New(srv.URL)

	epoch, paused, err := c.Epoch(context.Background(), make([]byte, 32))
	require.NoError(t, err)
	require.Equal(t, uint64(3), epoch)
	require.False(t, paused)

	params, err := c.WalletParams(context.Background())
	require.NoError(t, err)
	require.Equal(t, uint32(4), params.MaxDelegationDepth)
	require.Equal(t, uint64(600), uint64(params.MaxTokenTtlSeconds))
}

func TestAccountUnwrapsNestedBaseAccount(t *testing.T) {
	srv := newTestServer(t)
	c := New(srv.URL)

	info, err := c.Account(context.Background(), "svp1user")
	require.NoError(t, err)
	require.Equal(t, uint64(7), info.AccountNumber)
	require.Equal(t, uint64(3), info.Sequence)
}

func TestBroadcastTx(t *testing.T) {
	srv := newTestServer(t)
	c := New(srv.URL)

	res, err := c.BroadcastTx(context.Background(), []byte{0x01, 0x02})
	require.NoError(t, err)
	require.Equal(t, "ABC123", res.TxHash)
	require.Equal(t, uint32(0), res.Code)
}

// After a chain reset the cached answer is worse than no answer, so the
// refresh path must actually re-read rather than serve the old body.
func TestInvalidateCacheForcesAReRead(t *testing.T) {
	var calls int
	mux := http.NewServeMux()
	mux.HandleFunc("/dydxprotocol/agent/agents", func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			fmt.Fprint(w, `{"agents":[{"agent_id":"did:svp:svp1old","status":"AGENT_STATUS_ACTIVE"}]}`)
			return
		}
		fmt.Fprint(w, `{"agents":[]}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(srv.URL)
	first, err := c.Agents(context.Background(), "")
	require.NoError(t, err)
	require.Len(t, first, 1)

	// Still cached: the emptied registry is not visible yet.
	again, err := c.Agents(context.Background(), "")
	require.NoError(t, err)
	require.Len(t, again, 1)
	require.Equal(t, 1, calls)

	c.InvalidateCache()
	after, err := c.Agents(context.Background(), "")
	require.NoError(t, err)
	require.Empty(t, after, "a reset registry must show as empty after refresh")
	require.Equal(t, 2, calls)
}
