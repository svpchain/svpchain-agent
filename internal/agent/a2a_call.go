package agent

import (
	"context"
	"fmt"

	svpa2a "github.com/svpchain/svpchain-agent/internal/a2a"
)

func isA2ATool(name string) bool {
	return name == "a2a_send_message"
}

func a2aSendFromArgs(ctx context.Context, args map[string]any) (string, error) {
	agentURL, _ := args["agent_url"].(string)
	message, _ := args["message"].(string)
	if agentURL == "" {
		return "", fmt.Errorf("agent_url is required")
	}
	if message == "" {
		return "", fmt.Errorf("message is required")
	}
	return a2aSendMessage(ctx, agentURL, message)
}

// a2aSendMessage is overridden in tests.
var a2aSendMessage = func(ctx context.Context, agentURL, message string) (string, error) {
	return svpa2a.SendToAgentJSON(ctx, agentURL, message)
}
