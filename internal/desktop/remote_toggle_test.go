package desktop

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/svpchain/svpchain-local-agent/internal/manage"
)

// The runner reads an empty URL as "remote MCP off", so the toggle has to
// produce one — and has to win over both a configured URL and the default,
// or "disabled" would quietly still connect.
func TestResolveRemoteURL(t *testing.T) {
	const custom = "https://mcp.example.com/"

	require.Equal(t, custom, resolveRemoteURL(AgentSettings{RemoteMCPURL: custom}))
	require.Equal(t, manage.RemoteMCPURL, resolveRemoteURL(AgentSettings{}),
		"an empty field still means the default endpoint")
	require.Equal(t, manage.RemoteMCPURL, resolveRemoteURL(AgentSettings{RemoteMCPURL: "   "}),
		"whitespace is not a URL")

	require.Empty(t, resolveRemoteURL(AgentSettings{RemoteMCPDisabled: true, RemoteMCPURL: custom}),
		"the toggle must override a configured URL")
	require.Empty(t, resolveRemoteURL(AgentSettings{RemoteMCPDisabled: true}),
		"the toggle must override the default")
}
