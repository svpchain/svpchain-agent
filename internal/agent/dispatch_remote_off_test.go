package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/svpchain/svpchain-agent/internal/agent/discovery"
)

// With the remote MCP switched off, the tool list must contain no remote
// tools — the model should never be offered something that cannot run.
func TestBuildToolListWithoutRemote(t *testing.T) {
	tools, err := buildToolList(context.Background(), nil, &discovery.Service{})
	require.NoError(t, err)
	require.NotEmpty(t, tools, "local signing tools must still be offered")

	for _, name := range toolNames(tools) {
		require.NotEqual(t, "whoami", name, "remote tools must not be advertised")
		require.False(t, strings.HasPrefix(name, "build_"),
			"remote builder tool %q must not be advertised", name)
	}
}

// A capability called before an agent is attached must be refused with the
// Agent Market flow, not a nil dereference.
func TestDispatchRefusesRemoteToolsWhenDisabled(t *testing.T) {
	_, err := dispatchTool(context.Background(), "svp-2517-1", nil, nil, nil, nil, nil,
		"build_bank_send", map[string]any{}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "search_agents")
	require.NotContains(t, err.Error(), "MCP")
}

// The remote handler is an unconditional catch-all, so it also collects names
// nothing serves — a model inventing "list_tools" lands here. The refusal must
// not claim such a name is a remote MCP tool: that sends the user to a Settings
// toggle that would not have helped and stops a run that could still finish.
func TestDispatchDoesNotCallUnknownToolsRemote(t *testing.T) {
	_, err := dispatchTool(context.Background(), "svp-2517-1", nil, nil, nil, nil, nil,
		"list_tools", map[string]any{}, nil)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "is a remote MCP tool",
		"an invented name must not be described as a remote tool")
	require.Contains(t, err.Error(), "Do not retry")
	require.Contains(t, err.Error(), "a2a_connect_agent")
}
