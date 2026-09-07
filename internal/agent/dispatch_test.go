package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/svpchain/svpchain-agent/internal/agent/llm"
	"github.com/svpchain/svpchain-agent/internal/agent/memory"
)

// dispatchTool must short-circuit signer_whoami / whoami to cached session memory
// without touching the (nil) remote/local clients.
func TestDispatchToolUsesCachedWhoami(t *testing.T) {
	mem := &memory.Session{
		ChainID:      "svp-2517-1",
		RemoteURL:    "https://example.com/mcp",
		LocalOwner:   "svp1abc",
		SignerWhoami: `{"owner":"svp1abc","chain_id":"svp-2517-1"}`,
		RemoteWhoami: `{"tenant_id":"auto-1","owner":"svp1abc"}`,
	}
	out, err := dispatchTool(context.Background(), "svp-2517-1", nil, nil, nil, nil, nil, "signer_whoami", nil, mem)
	require.NoError(t, err)
	require.JSONEq(t, mem.SignerWhoami, out)

	out, err = dispatchTool(context.Background(), "svp-2517-1", nil, nil, nil, nil, nil, "whoami", nil, mem)
	require.NoError(t, err)
	require.JSONEq(t, mem.RemoteWhoami, out)
}

func TestHistoricalAgentToolNamesExcludesCurrentBaseTools(t *testing.T) {
	remembered := historicalAgentToolNames([]llm.Message{
		{Role: "tool", Name: "search_agents"},
		{Role: "tool", Name: "sign_evm_transaction"},
		{Role: "tool", Name: "lendora_build_borrow_tx"},
		{Role: "tool", Name: "lendora_get_account_summary"},
	}, toolList("search_agents", "sign_evm_transaction", BeginSettlementTool))
	require.Equal(t, map[string]struct{}{
		"lendora_build_borrow_tx":     {},
		"lendora_get_account_summary": {},
	}, remembered)
}
