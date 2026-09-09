package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConnectEndpointReusesActiveTaskForSameAgent(t *testing.T) {
	flow := &paidAgentFlow{started: true, endpoint: "https://agent.example"}
	var attached []string
	out, err := flow.ConnectEndpoint(context.Background(), "https://agent.example/", func(_ context.Context, endpoint string) (string, error) {
		attached = append(attached, endpoint)
		return `{"attached":true}`, nil
	})
	require.NoError(t, err)
	require.JSONEq(t, `{"attached":true}`, out)
	require.Equal(t, []string{"https://agent.example"}, attached)
}

func TestConnectEndpointRejectsDifferentAgentAfterFunding(t *testing.T) {
	flow := &paidAgentFlow{started: true, endpoint: "https://agent.example"}
	_, err := flow.ConnectEndpoint(context.Background(), "https://other.example", func(context.Context, string) (string, error) {
		t.Fatal("different endpoint must not attach")
		return "", nil
	})
	require.ErrorContains(t, err, "cannot connect a different endpoint")
}
