package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	"github.com/svpchain/svpchain-agent/internal/agent/guard"
	"github.com/svpchain/svpchain-agent/internal/agent/hitl"
	"github.com/svpchain/svpchain-agent/internal/agent/writepath"
	"github.com/svpchain/svpchain-agent/internal/prefs"
)

func TestWrapAppliesOuterFirst(t *testing.T) {
	var order []string
	h := wrap(func(context.Context, string, map[string]any) (string, error) {
		order = append(order, "mux")
		return "ok", nil
	}, func(next toolFunc) toolFunc {
		return func(ctx context.Context, name string, args map[string]any) (string, error) {
			order = append(order, "observe")
			return next(ctx, name, args)
		}
	}, func(next toolFunc) toolFunc {
		return func(ctx context.Context, name string, args map[string]any) (string, error) {
			order = append(order, "guard")
			return next(ctx, name, args)
		}
	}, func(next toolFunc) toolFunc {
		return func(ctx context.Context, name string, args map[string]any) (string, error) {
			order = append(order, "writepath")
			return next(ctx, name, args)
		}
	})
	out, err := h(context.Background(), "whoami", nil)
	require.NoError(t, err)
	require.Equal(t, "ok", out)
	require.Equal(t, []string{"observe", "guard", "writepath", "mux"}, order)
}

type recObserver struct {
	name   string
	ok     *bool
	err    string
	called bool
}

func (r *recObserver) RecordTool(name, _ string) func(ok bool, result, errDetail string) {
	r.name = name
	r.called = true
	return func(ok bool, _ string, errDetail string) {
		r.ok = &ok
		r.err = errDetail
	}
}

func TestObserveRecordsGuardRejectionWithoutHITL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prefs.json")
	allowed := common.HexToAddress("0x1111111111111111111111111111111111111111").Hex()
	blocked := common.HexToAddress("0x2222222222222222222222222222222222222222").Hex()
	require.NoError(t, os.WriteFile(path, []byte(`{"whitelist":[{"chain_id":"svp-2517-1","address_type":"evm","address":"`+allowed+`"}]}`), 0o600))
	t.Cleanup(func() { prefs.SetPathOverride("") })
	prefs.SetPathOverride(path)

	obs := &recObserver{}
	hitlCalled := false
	env := dispatchEnv{
		chainID: "svp-2517-1",
		observe: obs,
		writes:  writepath.New(),
		confirm: func(context.Context, hitl.Request) bool {
			hitlCalled = true
			return true
		},
	}
	_, err := env.dispatch(context.Background(), "sign_evm_transaction", map[string]any{
		"payload": map[string]any{
			"evm_chain_id": "2517",
			"to":           blocked,
			"value":        "1",
		},
	})
	var rej *guard.Rejection
	require.ErrorAs(t, err, &rej)
	require.False(t, hitlCalled, "HITL must not run after a whitelist rejection")
	require.True(t, obs.called)
	require.Equal(t, "sign_evm_transaction", obs.name)
	require.NotNil(t, obs.ok)
	require.False(t, *obs.ok)
	require.NotEmpty(t, obs.err)
}
