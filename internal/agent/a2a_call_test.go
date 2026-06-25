package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestA2ASendFromArgs(t *testing.T) {
	t.Parallel()
	prev := a2aSendMessage
	t.Cleanup(func() { a2aSendMessage = prev })

	a2aSendMessage = func(ctx context.Context, agentURL, message string) (string, error) {
		require.Equal(t, "http://localhost:9001", agentURL)
		require.Equal(t, "ping", message)
		return `{"response":"pong"}`, nil
	}

	out, err := a2aSendFromArgs(context.Background(), map[string]any{
		"agent_url": "http://localhost:9001",
		"message":   "ping",
	})
	require.NoError(t, err)
	require.Contains(t, out, "pong")
}

func TestA2ASendFromArgsValidation(t *testing.T) {
	t.Parallel()
	_, err := a2aSendFromArgs(context.Background(), map[string]any{"message": "hi"})
	require.Error(t, err)
	_, err = a2aSendFromArgs(context.Background(), map[string]any{"agent_url": "http://x"})
	require.Error(t, err)
}

func TestLocalToolDefsIncludesA2A(t *testing.T) {
	t.Parallel()
	var found bool
	for _, tool := range LocalToolDefs() {
		if tool.Function.Name == "a2a_send_message" {
			found = true
			break
		}
	}
	require.True(t, found)
}
