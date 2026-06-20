package manage

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	AgentNameClaudeCode    = "Claude Code"
	AgentNameClaudeDesktop = "Claude Desktop"
	AgentNameCursor        = "Cursor"
	AgentNameWindsurf      = "Windsurf"

	RemoteMCPURL = "https://indexer.svpchain.com/mcp"
)

// AgentNames is the display order for AI Agent radio buttons in the GUI.
var AgentNames = []string{
	AgentNameClaudeCode,
	AgentNameClaudeDesktop,
	AgentNameCursor,
	AgentNameWindsurf,
}

type mcpServerEntry struct {
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
	Type    string   `json:"type,omitempty"`
	URL     string   `json:"url,omitempty"`
}

// MCPConfigText generates MCP configuration text for the given AI agent type.
func MCPConfigText(agentName, chainID, signerBinaryPath string) (string, error) {
	chainID = strings.TrimSpace(chainID)
	signerBinaryPath = strings.TrimSpace(signerBinaryPath)
	if chainID == "" {
		return "", fmt.Errorf("chain id is required")
	}
	if signerBinaryPath == "" {
		return "", fmt.Errorf("signer binary path is required")
	}

	switch strings.TrimSpace(agentName) {
	case AgentNameClaudeCode:
		return claudeCodeConfig(chainID, signerBinaryPath)
	case AgentNameClaudeDesktop, AgentNameCursor, AgentNameWindsurf:
		return mcpServersConfig(chainID, signerBinaryPath)
	default:
		return "", fmt.Errorf("unknown ai agent: %q", agentName)
	}
}

func claudeCodeConfig(chainID, signerBinaryPath string) (string, error) {
	signerJSON, err := json.MarshalIndent(map[string]any{
		"command": signerBinaryPath,
		"args":    []string{"--chain-id", chainID},
	}, "", "  ")
	if err != nil {
		return "", err
	}
	remoteJSON, err := json.MarshalIndent(map[string]string{
		"type": "http",
		"url":  RemoteMCPURL,
	}, "", "  ")
	if err != nil {
		return "", err
	}

	line1 := fmt.Sprintf("claude mcp add-json -s user svpchain-agent '%s'", string(signerJSON))
	line2 := fmt.Sprintf("claude mcp add-json -s user svpchain-remote '%s'", string(remoteJSON))
	return line1 + "\n\n" + line2 + "\n", nil
}

func mcpServersConfig(chainID, signerBinaryPath string) (string, error) {
	cfg := MCPConfig{
		MCPServers: map[string]mcpServerEntry{
			"svpchain-agent": {
				Command: signerBinaryPath,
				Args:    []string{"--chain-id", chainID},
			},
			"svpchain-remote": {
				Type: "http",
				URL:  RemoteMCPURL,
			},
		},
	}
	bz, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", err
	}
	return string(bz) + "\n", nil
}
