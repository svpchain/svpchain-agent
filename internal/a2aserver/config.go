package a2aserver

import (
	"strings"

	"github.com/svpchain/svpchain-agent/internal/manage"
	"github.com/svpchain/svpchain-agent/internal/prefs"
)

const (
	AgentName        = "svpchain agent"
	AgentVersion     = "v0.1.0"
	AgentDescription = "Local-key svpchain on-chain assistant for Cosmos/EVM. Remote MCP builds unsigned payloads and broadcasts signed transactions; local signer holds the key. Supports trading, swap, transfer, bridge, ERC-20/721, x402 paid HTTP content, and A2A delegation."
)

// LLMConfig holds LLM credentials for the A2A server executor.
type LLMConfig struct {
	APIKey  string
	BaseURL string
	Model   string
}

// ServerConfig drives the A2A HTTP server.
type ServerConfig struct {
	ChainID      string
	ListenAddr   string
	PublicURL    string
	RemoteMCPURL string
	LLM          LLMConfig
}

// ConfigFromPrefs builds server config from prefs.json with CLI overrides.
func ConfigFromPrefs(chainID, listenAddr, publicURL string) (ServerConfig, error) {
	f := prefs.Read()
	if chainID == "" {
		chainID = strings.TrimSpace(f.AgentChainID)
	}
	if chainID == "" {
		return ServerConfig{}, errChainIDRequired
	}
	remoteURL := strings.TrimSpace(f.RemoteMCPURL)
	if remoteURL == "" {
		remoteURL = manage.RemoteMCPURL
	}
	if listenAddr == "" {
		listenAddr = ":8080"
	}
	if publicURL == "" {
		publicURL = "http://127.0.0.1" + listenAddr
	}
	publicURL = strings.TrimRight(publicURL, "/")

	return ServerConfig{
		ChainID:      chainID,
		ListenAddr:   listenAddr,
		PublicURL:    publicURL,
		RemoteMCPURL: remoteURL,
		LLM: LLMConfig{
			APIKey:  f.LLMAPIKey,
			BaseURL: f.LLMBaseURL,
			Model:   f.LLMModel,
		},
	}, nil
}
