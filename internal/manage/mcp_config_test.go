package manage

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMCPConfigJSON(t *testing.T) {
	got, err := MCPConfigJSON("localsvp-1", "/opt/bin/svpchain-mcp")
	require.NoError(t, err)
	require.Contains(t, got, `"command": "/opt/bin/svpchain-mcp"`)
	require.Contains(t, got, `"--chain-id"`)
	require.Contains(t, got, `"localsvp-1"`)
	require.Contains(t, got, `"svpchain-remote"`)
	require.Contains(t, got, RemoteMCPURL)

	_, err = MCPConfigJSON("", "/bin/svpchain-mcp")
	require.Error(t, err)

	_, err = MCPConfigJSON("localsvp-1", "")
	require.Error(t, err)
}

func TestMCPConfigText_ClaudeCode(t *testing.T) {
	got, err := MCPConfigText(AgentNameClaudeCode, "svp-2517-1", "/usr/local/bin/svpchain-mcp")
	require.NoError(t, err)
	require.Contains(t, got, "claude mcp add-json -s user svpchain-agent")
	require.Contains(t, got, "claude mcp add-json -s user svpchain-remote")
	require.Contains(t, got, `"svp-2517-1"`)
	require.Contains(t, got, RemoteMCPURL)
}

func TestMCPConfigText_Cursor(t *testing.T) {
	got, err := MCPConfigText(AgentNameCursor, "svp-2517-1", "/usr/local/bin/svpchain-mcp")
	require.NoError(t, err)
	require.Contains(t, got, `"mcpServers"`)
	require.Contains(t, got, `"svpchain-agent"`)
	require.Contains(t, got, `"svpchain-remote"`)
	require.Contains(t, got, `"type": "http"`)
}

func TestMCPConfigText_sameJSONForDesktopCursorWindsurf(t *testing.T) {
	args := []string{AgentNameClaudeDesktop, AgentNameCursor, AgentNameWindsurf}
	var first string
	for i, agent := range args {
		got, err := MCPConfigText(agent, "chain-a", "/bin/signer")
		require.NoError(t, err)
		if i == 0 {
			first = got
			continue
		}
		require.Equal(t, first, got)
	}
}

func TestMCPConfigText_unknownAgent(t *testing.T) {
	_, err := MCPConfigText("Unknown", "chain-a", "/bin/signer")
	require.Error(t, err)
}
