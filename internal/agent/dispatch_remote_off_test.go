package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/svpchain/svpchain-agent/internal/agent/delegatecall"
)

// With the remote MCP switched off, the tool list must contain no remote
// tools — the model should never be offered something that cannot run.
func TestBuildToolListWithoutRemote(t *testing.T) {
	tools, err := buildToolList(context.Background(), nil, &delegatecall.Service{})
	require.NoError(t, err)
	require.NotEmpty(t, tools, "local signing tools must still be offered")

	for _, name := range toolNames(tools) {
		require.NotEqual(t, "whoami", name, "remote tools must not be advertised")
		require.False(t, strings.HasPrefix(name, "build_"),
			"remote builder tool %q must not be advertised", name)
	}
}

// A remote tool called anyway must be refused with an explanation, not a nil
// dereference.
func TestDispatchRefusesRemoteToolsWhenDisabled(t *testing.T) {
	_, err := dispatchTool(context.Background(), "svp-2517-1", nil, nil, nil, nil,
		"build_bank_send", map[string]any{}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "remote MCP is disabled")
}
