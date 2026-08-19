package delegatecall

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"iter"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/gogoproto/proto"
	"github.com/cosmos/evm/crypto/ethsecp256k1"
	"github.com/stretchr/testify/require"
	"github.com/svpchain/svpdt"

	svpa2a "github.com/svpchain/svpchain-agent/internal/a2a"
	"github.com/svpchain/svpchain-agent/internal/delegation"
	"github.com/svpchain/svpchain-agent/internal/registry"
)

const (
	testChainID = "svp-delegate-test"
	testNow     = int64(1_700_000_000)
	remoteDID   = "did:svp:svp1remoteagentoperator"
	testRootHex = "0101010101010101010101010101010101010101010101010101010101010101"
)

// echoExecutor returns the envelope it received, so a test can inspect exactly
// what the local agent sent — including the credential in the message metadata.
type echoExecutor struct {
	mu       sync.Mutex
	received string
	metadata map[string]any
}

func (e *echoExecutor) Execute(ctx context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		text := svpa2a.MessageText(execCtx.Message)
		e.mu.Lock()
		e.received = text
		e.metadata = execCtx.Message.Metadata
		e.mu.Unlock()

		if !yield(a2a.NewSubmittedTask(execCtx, execCtx.Message), nil) {
			return
		}
		reply, _ := json.Marshal(map[string]any{
			"skill": "svpchain-execution", "tool": "execute_place_order",
			"ok": true, "result": map[string]string{"tx_hash": "DEADBEEF"},
		})
		// Terminal status update carrying the result, mirroring how the real
		// DEX agent answers.
		msg := a2a.NewMessageForTask(a2a.MessageRoleAgent, execCtx, a2a.NewTextPart(string(reply)))
		yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateCompleted, msg), nil)
	}
}

func (e *echoExecutor) Cancel(ctx context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateCanceled, nil), nil)
	}
}

func (e *echoExecutor) envelope(t *testing.T) map[string]any {
	t.Helper()
	e.mu.Lock()
	defer e.mu.Unlock()
	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(e.received), &out))
	return out
}

// stubStack is a chain REST endpoint plus a remote A2A agent, wired to each
// other the way the real ones are: the registry's endpoint points at the
// agent, and the agent's DID is what credentials must be addressed to.
// txCapture records every tx the stub chain accepted, so a test can decode
// what the lifecycle actually signed and broadcast.
type txCapture struct {
	mu     sync.Mutex
	bodies [][]byte
}

func (c *txCapture) add(b []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bodies = append(c.bodies, b)
}

// soleMsg asserts exactly one captured tx since idx whose sole message has the
// given type URL, returning its value bytes.
func (c *txCapture) soleMsg(t *testing.T, typeURL string) []byte {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, raw := range c.bodies {
		var txRaw txtypes.TxRaw
		require.NoError(t, proto.Unmarshal(raw, &txRaw))
		var body txtypes.TxBody
		require.NoError(t, proto.Unmarshal(txRaw.BodyBytes, &body))
		for _, msg := range body.Messages {
			if msg.TypeUrl == typeURL {
				return msg.Value
			}
		}
	}
	t.Fatalf("no broadcast tx carries %s", typeURL)
	return nil
}

func stubStack(t *testing.T) (*Service, *echoExecutor, *delegation.Lifecycle, *txCapture) {
	t.Helper()

	exec := &echoExecutor{}
	agentMux := http.NewServeMux()
	agentMux.Handle("/invoke", a2asrv.NewJSONRPCHandler(a2asrv.NewHandler(exec)))
	agentSrv := httptest.NewServer(agentMux)
	t.Cleanup(agentSrv.Close)

	card := &a2a.AgentCard{
		Name:                "stub-remote-agent",
		Version:             "1.0.0",
		SupportedInterfaces: []*a2a.AgentInterface{a2a.NewAgentInterface(agentSrv.URL+"/invoke", a2a.TransportProtocolJSONRPC)},
		DefaultInputModes:   []string{"text/plain"},
		DefaultOutputModes:  []string{"application/json"},
	}
	agentMux.Handle(a2asrv.WellKnownAgentCardPath, a2asrv.NewStaticAgentCardHandler(card))

	cardBody, err := json.Marshal(card)
	require.NoError(t, err)
	cardHash := sha256.Sum256(cardBody)

	rootID, err := hex.DecodeString(testRootHex)
	require.NoError(t, err)

	priv, err := ethsecp256k1.GenerateKey()
	require.NoError(t, err)
	owner := (&delegation.Lifecycle{Priv: priv}).Owner()
	ownerDID := "did:svp:" + owner

	chainMux := http.NewServeMux()
	chainMux.HandleFunc("/dydxprotocol/agent/agent/", func(w http.ResponseWriter, r *http.Request) {
		// Echo the requested DID so any agent id — the direct audience or a
		// redelegate_to target — resolves as a registered active agent.
		id := strings.TrimPrefix(r.URL.Path, "/dydxprotocol/agent/agent/")
		if unescaped, err := url.PathUnescape(id); err == nil {
			id = unescaped
		}
		fmt.Fprintf(w, `{"agent":{"agent_id":%q,"operator":"svp1remoteagentoperator","endpoint":%q,
			"capability_hash":%q,"capabilities":["trading"],"status":"AGENT_STATUS_ACTIVE"}}`,
			id, agentSrv.URL, base64.StdEncoding.EncodeToString(cardHash[:]))
	})
	delegationJSON := func() string {
		return fmt.Sprintf(`{"root_id":%q,"delegator":%q,"agent_id":%q,
			"limits":{"actions":["clob.place_order","settlement.record_spend"],"subaccounts":[0],
			"svc_spend_limit_total":[{"denom":"uusdc","amount":"10000000"}],
			"max_depth":2,"max_token_ttl_seconds":600},
			"epoch":"1","paused":false,"expires_at":"1900000000","created_at_height":"10"}`,
			base64.StdEncoding.EncodeToString(rootID), owner, ownerDID)
	}
	chainMux.HandleFunc("/dydxprotocol/agentwallet/delegations_by_delegator/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"delegations":[%s]}`, delegationJSON())
	})
	// A second root that exists but cannot pay agents: no settlement action,
	// no service allowance. What the payment pre-check must catch.
	poorRootID := make([]byte, 32)
	for i := range poorRootID {
		poorRootID[i] = 0x22
	}
	chainMux.HandleFunc("/dydxprotocol/agentwallet/delegation/", func(w http.ResponseWriter, r *http.Request) {
		suffix := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		if suffix == base64.URLEncoding.EncodeToString(poorRootID) {
			fmt.Fprintf(w, `{"delegation":{"root_id":%q,"delegator":%q,"agent_id":%q,
				"limits":{"actions":["clob.place_order"],"subaccounts":[0],"max_depth":2,"max_token_ttl_seconds":600},
				"epoch":"1","paused":false,"expires_at":"1900000000","created_at_height":"10"}}`,
				base64.StdEncoding.EncodeToString(poorRootID), owner, ownerDID)
			return
		}
		fmt.Fprintf(w, `{"delegation":%s}`, delegationJSON())
	})
	chainMux.HandleFunc("/dydxprotocol/agentwallet/epoch/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"epoch":"1","paused":false}`)
	})
	chainMux.HandleFunc("/dydxprotocol/agentwallet/params", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"params":{"max_delegation_depth":4,"max_token_ttl_seconds":"600"}}`)
	})
	capture := &txCapture{}
	chainMux.HandleFunc("/cosmos/auth/v1beta1/accounts/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"account":{"account_number":"1","sequence":"0"}}`)
	})
	settlementID := make([]byte, 32)
	settlementID[0] = 0xC0
	chainMux.HandleFunc("/cosmos/tx/v1beta1/txs", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			TxBytes string `json:"tx_bytes"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&in))
		raw, err := base64.StdEncoding.DecodeString(in.TxBytes)
		require.NoError(t, err)
		capture.add(raw)
		fmt.Fprint(w, `{"tx_response":{"txhash":"AB12","code":0}}`)
	})
	chainMux.HandleFunc("/cosmos/tx/v1beta1/txs/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"tx_response":{"txhash":"AB12","code":0,"height":"5"}}`)
	})
	chainMux.HandleFunc("/dydxprotocol/settlement/settlements_by_opener/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"settlements":[{"id":%q,"opener":%q,
			"cap":{"denom":"uusdc","amount":"500000"},"fee_paid":{"denom":"uusdc","amount":"1000"},
			"total_recorded":{"denom":"uusdc","amount":"0"},"total_claimed":{"denom":"uusdc","amount":"0"},
			"refunded":{"denom":"uusdc","amount":"0"},"status":"SETTLEMENT_STATUS_OPEN",
			"created_at_height":"9","memo":"m"}]}`,
			base64.StdEncoding.EncodeToString(settlementID), owner)
	})
	chainSrv := httptest.NewServer(chainMux)
	t.Cleanup(chainSrv.Close)

	reg := registry.New(chainSrv.URL)
	life := &delegation.Lifecycle{Registry: reg, Priv: priv, ChainID: testChainID}
	svc := &Service{
		Registry:  reg,
		Lifecycle: life,
		Confirm:   func(context.Context, ConfirmRequest) bool { return true },
		Now:       func() int64 { return testNow },
	}
	return svc, exec, life, capture
}

func taskArgs() map[string]any {
	return map[string]any{
		"agent_id":    remoteDID,
		"skill":       "svpchain-execution",
		"tool":        "execute_place_order",
		"args":        map[string]any{"order": map[string]any{"ticker": "BTC-USD"}},
		"actions":     []any{"clob.place_order"},
		"subaccounts": []any{float64(0)},
		"budget":      []any{map[string]any{"denom": "uusdc", "amount": "1000000"}},
	}
}

// The end-to-end shape: a task delegated to a discovered agent arrives as
// {skill, tool, args} with a verifiable credential in the message metadata
// under the delegation extension's key, addressed to that agent and issued
// by the user.
func TestDelegateTaskSendsAVerifiableProof(t *testing.T) {
	svc, exec, life, _ := stubStack(t)

	out, err := svc.Call(context.Background(), "delegate_task", taskArgs())
	require.NoError(t, err)
	require.Contains(t, out, "DEADBEEF")

	env := exec.envelope(t)
	require.Equal(t, "svpchain-execution", env["skill"])
	require.Equal(t, "execute_place_order", env["tool"])

	args := env["args"].(map[string]any)
	require.NotNil(t, args["order"], "the caller's own args must survive")
	require.Nil(t, args["proof"], "the credential must not ride the envelope args")

	exec.mu.Lock()
	deleg, ok := exec.metadata[svpa2a.DelegationMetadataKey].(map[string]any)
	exec.mu.Unlock()
	require.True(t, ok, "message metadata must carry %s", svpa2a.DelegationMetadataKey)

	proof := deleg["tokens"].([]any)
	require.Len(t, proof, 1, "a task credential is one hop")
	raw, err := base64.StdEncoding.DecodeString(proof[0].(string))
	require.NoError(t, err)

	resolver := svpdt.SingleKeyResolver(map[string][]byte{
		life.OwnerDID(): life.Priv.PubKey().Bytes(),
	})
	verified, err := svpdt.VerifyChain([][]byte{raw}, resolver, svpdt.VerifyOpts{
		ChainID:  testChainID,
		Now:      testNow,
		MaxDepth: 4,
		Audience: remoteDID,
	})
	require.NoError(t, err)
	require.Equal(t, life.Owner(), verified.Principal)
	require.True(t, verified.Effective.Actions.Has("clob.place_order"))
	require.False(t, verified.Effective.Actions.Has("clob.cancel_order"))
}

// The safety-critical invariant: a declined dialog mints nothing and sends
// nothing.
func TestDelegateTaskRefusesWithoutApproval(t *testing.T) {
	svc, exec, _, _ := stubStack(t)
	svc.Confirm = func(context.Context, ConfirmRequest) bool { return false }

	_, err := svc.Call(context.Background(), "delegate_task", taskArgs())
	require.Error(t, err)
	require.Contains(t, err.Error(), "declined")

	exec.mu.Lock()
	defer exec.mu.Unlock()
	require.Empty(t, exec.received, "nothing may reach the remote agent")
}

// A nil hook is a denial, so a headless caller cannot silently mint.
func TestDelegateTaskRefusesWithoutAConfirmHook(t *testing.T) {
	svc, _, _, _ := stubStack(t)
	svc.Confirm = nil

	_, err := svc.Call(context.Background(), "delegate_task", taskArgs())
	require.Error(t, err)
	require.Contains(t, err.Error(), "declined")
}

// The chain prices a value-committing action against the credential's own
// budget, so an order credential with no budget is dead on arrival. Catching
// it here keeps a doomed credential from costing the user a confirmation.
func TestDelegateTaskRequiresABudgetForCommittingActions(t *testing.T) {
	svc, exec, _, _ := stubStack(t)
	args := taskArgs()
	delete(args, "budget")

	_, err := svc.Call(context.Background(), "delegate_task", args)
	require.Error(t, err)
	require.Contains(t, err.Error(), "budget is required")

	exec.mu.Lock()
	defer exec.mu.Unlock()
	require.Empty(t, exec.received)
}

// A cancellation commits nothing, so it needs no budget.
func TestDelegateTaskAllowsCancelWithoutBudget(t *testing.T) {
	svc, _, _, _ := stubStack(t)
	args := taskArgs()
	delete(args, "budget")
	args["tool"] = "execute_cancel_order"
	args["actions"] = []any{"clob.cancel_order"}

	_, err := svc.Call(context.Background(), "delegate_task", args)
	require.NoError(t, err)
}

// Empty grants deny in SVP-DT, so a task with no actions is a mistake worth
// catching before a credential is minted.
func TestDelegateTaskRequiresExplicitActions(t *testing.T) {
	svc, _, _, _ := stubStack(t)
	args := taskArgs()
	delete(args, "actions")

	_, err := svc.Call(context.Background(), "delegate_task", args)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "actions is required"))
}

// A redelegable grant still sends one token — the intermediary appends its
// own hop — with the redelegation caveats set, and the confirmation dialog
// names the widened blast radius before anything is minted.
func TestDelegateTaskRedelegableProof(t *testing.T) {
	const executorDID = "did:svp:svp1finalexecutor"
	svc, exec, life, _ := stubStack(t)

	var confirmed []ConfirmRequest
	svc.Confirm = func(_ context.Context, req ConfirmRequest) bool {
		confirmed = append(confirmed, req)
		return true
	}

	args := taskArgs()
	args["redelegable"] = true
	args["redelegate_to"] = []any{executorDID}
	_, err := svc.Call(context.Background(), "delegate_task", args)
	require.NoError(t, err)

	require.Len(t, confirmed, 1)
	dialog := strings.Join(confirmed[0].Lines, "\n")
	require.Contains(t, dialog, "RE-DELEGABLE")
	require.Contains(t, dialog, executorDID)

	exec.mu.Lock()
	deleg := exec.metadata[svpa2a.DelegationMetadataKey].(map[string]any)
	exec.mu.Unlock()
	proof := deleg["tokens"].([]any)
	require.Len(t, proof, 1, "the issuer still emits one token")

	raw, err := base64.StdEncoding.DecodeString(proof[0].(string))
	require.NoError(t, err)
	tok, err := svpdt.UnmarshalToken(raw)
	require.NoError(t, err)
	cav := tok.Payload.Caveats
	require.True(t, cav.Redelegable)
	require.Equal(t, uint32(2), cav.MaxDepth)
	require.True(t, cav.RedelegateTo.Constrained)
	require.True(t, cav.RedelegateTo.Set.Has(executorDID))
	_ = life
}

// Redelegable without targets is refused before the user ever sees a dialog.
func TestDelegateTaskRedelegableRequiresTargets(t *testing.T) {
	svc, exec, _, _ := stubStack(t)
	hookFired := false
	svc.Confirm = func(context.Context, ConfirmRequest) bool {
		hookFired = true
		return true
	}

	args := taskArgs()
	args["redelegable"] = true
	_, err := svc.Call(context.Background(), "delegate_task", args)
	require.Error(t, err)
	require.Contains(t, err.Error(), "redelegate_to")
	require.False(t, hookFired, "a doomed grant must not cost the user a dialog")

	args = taskArgs()
	args["redelegate_to"] = []any{"did:svp:svp1finalexecutor"} // without redelegable
	_, err = svc.Call(context.Background(), "delegate_task", args)
	require.Error(t, err)
	require.Contains(t, err.Error(), "redelegable is false")
	require.False(t, hookFired)

	exec.mu.Lock()
	defer exec.mu.Unlock()
	require.Empty(t, exec.received)
}

// A read-only grant needs no budget — and refuses one, since nothing prices
// a query action and a budget only widens what a leaked credential is worth.
func TestDelegateTaskReadOnlyGrants(t *testing.T) {
	svc, exec, life, _ := stubStack(t)

	args := taskArgs()
	args["tool"] = "get_balance"
	args["skill"] = "svpchain-account"
	args["actions"] = []any{"query.account"}
	delete(args, "budget")
	_, err := svc.Call(context.Background(), "delegate_task", args)
	require.NoError(t, err)

	exec.mu.Lock()
	deleg := exec.metadata[svpa2a.DelegationMetadataKey].(map[string]any)
	exec.mu.Unlock()
	raw, err := base64.StdEncoding.DecodeString(deleg["tokens"].([]any)[0].(string))
	require.NoError(t, err)
	verified, err := svpdt.VerifyChain([][]byte{raw}, svpdt.SingleKeyResolver(map[string][]byte{
		life.OwnerDID(): life.Priv.PubKey().Bytes(),
	}), svpdt.VerifyOpts{ChainID: testChainID, Now: testNow, MaxDepth: 4, Audience: remoteDID})
	require.NoError(t, err)
	require.True(t, verified.Effective.Actions.Has("query.account"))
	require.Empty(t, verified.Effective.Budget)

	args = taskArgs()
	args["tool"] = "get_balance"
	args["skill"] = "svpchain-account"
	args["actions"] = []any{"query.account"}
	_, err = svc.Call(context.Background(), "delegate_task", args) // budget kept
	require.Error(t, err)
	require.Contains(t, err.Error(), "no budget")
}
