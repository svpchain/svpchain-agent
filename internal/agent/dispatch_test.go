package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

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
	out, err := dispatchTool(context.Background(), "svp-2517-1", nil, nil, "signer_whoami", nil, mem)
	require.NoError(t, err)
	require.JSONEq(t, mem.SignerWhoami, out)

	out, err = dispatchTool(context.Background(), "svp-2517-1", nil, nil, "whoami", nil, mem)
	require.NoError(t, err)
	require.JSONEq(t, mem.RemoteWhoami, out)
}
