package desktop

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/svpchain/svpchain-local-agent/internal/registry"
)

// DiscoveredAgent is one registry row for the Agents tab.
type DiscoveredAgent struct {
	AgentID      string   `json:"agent_id"`
	Endpoint     string   `json:"endpoint"`
	Capabilities []string `json:"capabilities"`
	PricingText  string   `json:"pricing_text"`
	BondText     string   `json:"bond_text"`
	Metadata     string   `json:"metadata"`
	CardVerified bool     `json:"card_verified"`
	CardError    string   `json:"card_error,omitempty"`
}

// AgentsDiscover lists ACTIVE registered agents, optionally filtered by one
// capability tag, with each agent's A2A card checked against its on-chain
// capability hash.
func (a *App) AgentsDiscover(capability string) ([]DiscoveredAgent, error) {
	restURL := strings.TrimSpace(a.store.AgentSettings().AgentHubURL)
	if restURL == "" {
		return nil, errors.New("chain REST endpoint is not configured; set it in Settings")
	}
	client := registry.New(restURL)

	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()

	agents, err := client.Agents(ctx, capability)
	if err != nil {
		return nil, err
	}
	rows := make([]DiscoveredAgent, 0, len(agents))
	for _, ag := range agents {
		row := DiscoveredAgent{
			AgentID:      ag.AgentID,
			Endpoint:     ag.Endpoint,
			Capabilities: ag.Capabilities,
			PricingText:  pricingText(ag.Pricing),
			BondText:     coinText(ag.Bond),
			Metadata:     ag.Metadata,
		}
		if card, cardErr := client.FetchCard(ctx, ag); cardErr != nil {
			row.CardError = cardErr.Error()
		} else {
			row.CardVerified = card.Verified
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func pricingText(p registry.Pricing) string {
	if len(p.PerCall) == 0 {
		return ""
	}
	parts := make([]string, 0, len(p.PerCall))
	for _, c := range p.PerCall {
		parts = append(parts, coinText(c))
	}
	out := strings.Join(parts, ", ")
	if p.Unit != "" {
		out += " / " + p.Unit
	}
	return out
}

func coinText(c registry.Coin) string {
	if c.Amount == "" && c.Denom == "" {
		return ""
	}
	return c.Amount + " " + c.Denom
}
