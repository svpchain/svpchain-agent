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
	"strings"
	"sync"
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
	"github.com/cosmos/evm/crypto/ethsecp256k1"
	"github.com/stretchr/testify/require"
	"github.com/svpchain/svpdt"

	svpa2a "github.com/svpchain/svpchain-local-agent/internal/a2a"
	"github.com/svpchain/svpchain-local-agent/internal/delegation"
	"github.com/svpchain/svpchain-local-agent/internal/registry"
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
func stubStack(t *testing.T) (*Service, *echoExecutor, *delegation.Lifecycle) {
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
		fmt.Fprintf(w, `{"agent":{"agent_id":%q,"operator":"svp1remoteagentoperator","endpoint":%q,
			"capability_hash":%q,"capabilities":["trading"],"status":"AGENT_STATUS_ACTIVE"}}`,
			remoteDID, agentSrv.URL, base64.StdEncoding.EncodeToString(cardHash[:]))
	})
	chainMux.HandleFunc("/dydxprotocol/agentwallet/delegations_by_delegator/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"delegations":[{"root_id":%q,"delegator":%q,"agent_id":%q,
			"limits":{"actions":["clob.place_order"],"subaccounts":[0],"max_depth":2,"max_token_ttl_seconds":600},
			"epoch":"1","paused":false,"expires_at":"1900000000","created_at_height":"10"}]}`,
			base64.StdEncoding.EncodeToString(rootID), owner, ownerDID)
	})
	chainMux.HandleFunc("/dydxprotocol/agentwallet/epoch/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"epoch":"1","paused":false}`)
	})
	chainMux.HandleFunc("/dydxprotocol/agentwallet/params", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"params":{"max_delegation_depth":4,"max_token_ttl_seconds":"600"}}`)
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
	return svc, exec, life
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
	svc, exec, life := stubStack(t)

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
	svc, exec, _ := stubStack(t)
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
	svc, _, _ := stubStack(t)
	svc.Confirm = nil

	_, err := svc.Call(context.Background(), "delegate_task", taskArgs())
	require.Error(t, err)
	require.Contains(t, err.Error(), "declined")
}

// The chain prices a value-committing action against the credential's own
// budget, so an order credential with no budget is dead on arrival. Catching
// it here keeps a doomed credential from costing the user a confirmation.
func TestDelegateTaskRequiresABudgetForCommittingActions(t *testing.T) {
	svc, exec, _ := stubStack(t)
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
	svc, _, _ := stubStack(t)
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
	svc, _, _ := stubStack(t)
	args := taskArgs()
	delete(args, "actions")

	_, err := svc.Call(context.Background(), "delegate_task", args)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "actions is required"))
}
