package a2aserver

import (
	"fmt"

	"github.com/a2aproject/a2a-go/v2/a2a"
)

// BuildAgentCard returns the public Agent Card for this svpchain agent.
func BuildAgentCard(publicURL string) *a2a.AgentCard {
	invokeURL := publicURL + "/invoke"
	return &a2a.AgentCard{
		Name:        AgentName,
		Description: AgentDescription,
		Version:     AgentVersion,
		SupportedInterfaces: []*a2a.AgentInterface{
			a2a.NewAgentInterface(invokeURL, a2a.TransportProtocolJSONRPC),
		},
		DefaultInputModes:  []string{"text/plain"},
		DefaultOutputModes: []string{"text/plain"},
		Capabilities: a2a.AgentCapabilities{
			Streaming: true,
		},
		Provider: &a2a.AgentProvider{
			Org: "svpchain",
			URL: "https://www.svpchain.org",
		},
		Skills: []a2a.AgentSkill{
			{
				ID:          "svpchain-trading",
				Name:        "svpchain Trading Assistant",
				Description: "Cosmos/EVM DEX trading via remote MCP build/broadcast and local signing. Keys never leave the machine.",
				Tags:        []string{"trading", "cosmos", "evm", "defi", "dex", "dydx"},
				Examples: []string{
					"What is my account balance?",
					"Place a limit buy order on BTC-USD",
					"Transfer 10 SVP to svp1…",
				},
			},
			{
				ID:          "x402-content",
				Name:        "x402 Paid Content",
				Description: "Access x402 v2 paywalled HTTP resources with off-chain EIP-3009 authorization.",
				Tags:        []string{"x402", "http", "payments"},
				Examples: []string{
					fmt.Sprintf("Read the x402 article at %s", "https://www.svpchain.org/zh-TW/x402/article"),
				},
			},
			{
				ID:          "agent-delegation",
				Name:        "Agent Delegation",
				Description: "Delegate sub-tasks to other A2A-compatible agents via a2a_send_message.",
				Tags:        []string{"a2a", "multi-agent"},
				Examples: []string{
					"Ask the compliance agent at http://localhost:9001 to review this transfer",
				},
			},
		},
	}
}
