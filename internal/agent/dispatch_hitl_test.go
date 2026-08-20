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
	"github.com/svpchain/svpchain-agent/internal/prefs"
)

// A nil confirm hook must deny sign_* before the (nil) signer is touched.
func TestDispatchNilConfirmDeniesSignWithoutCallingSigner(t *testing.T) {
	_, err := dispatchTool(context.Background(), "svp-2517-1", nil, nil, nil, nil,
		"sign_transaction", map[string]any{
			"payload": map[string]any{
				"chain_id":       "svp-2517-1",
				"signer_address": "svp1abc",
			},
		}, nil)
	var denied *hitl.Denied
	require.ErrorAs(t, err, &denied)
	require.Equal(t, hitl.KindSignTransaction, denied.Kind)
	require.Contains(t, denied.StopMessage(), "Signing declined")
}

func TestDispatchSignAsksHITLThenDenies(t *testing.T) {
	var got hitl.Request
	confirm := func(_ context.Context, req hitl.Request) bool {
		got = req
		return false
	}
	_, err := dispatchTool(context.Background(), "svp-2517-1", nil, nil, nil, confirm,
		"sign_transaction", map[string]any{
			"payload": map[string]any{
				"chain_id":       "svp_2517-1",
				"signer_address": "svp1abc",
				"summary":        map[string]any{"amount_human": "1 svp", "recipient_owner": "svp1dest"},
			},
		}, nil)
	require.ErrorAs(t, err, new(*hitl.Denied))
	require.Equal(t, hitl.KindSignTransaction, got.Kind)
	require.Contains(t, got.Lines, "Amount: 1 svp")
	require.Contains(t, got.Lines, "Recipient: svp1dest")
}

func TestDispatchSignEVMWhitelistRejectsBeforeHITL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prefs.json")
	allowed := common.HexToAddress("0x1111111111111111111111111111111111111111").Hex()
	blocked := common.HexToAddress("0x2222222222222222222222222222222222222222").Hex()
	require.NoError(t, os.WriteFile(path, []byte(`{"whitelist":[{"chain_id":"svp-2517-1","address_type":"evm","address":"`+allowed+`"}]}`), 0o600))
	t.Cleanup(func() { prefs.SetPathOverride("") })
	prefs.SetPathOverride(path)

	called := false
	confirm := func(context.Context, hitl.Request) bool {
		called = true
		return true
	}
	_, err := dispatchTool(context.Background(), "svp-2517-1", nil, nil, nil, confirm,
		"sign_evm_transaction", map[string]any{
			"payload": map[string]any{
				"evm_chain_id": "2517",
				"to":           blocked,
				"value":        "1",
			},
		}, nil)
	var rej *guard.Rejection
	require.ErrorAs(t, err, &rej)
	require.False(t, called, "HITL must not run after a whitelist rejection")
}
