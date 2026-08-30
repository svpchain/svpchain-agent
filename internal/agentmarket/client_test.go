package agentmarket

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/agents/did:svp:evm", r.URL.Path)
		_, _ = w.Write([]byte(`{"agent_id":"did:svp:evm","owner":"svp1owner","endpoint":"http://agent","status":"AGENT_STATUS_ACTIVE","pricing":{"amount":"1000000","unit":"call"}}`))
	}))
	t.Cleanup(server.Close)

	hit, err := New(server.URL).Get(context.Background(), "did:svp:evm")
	require.NoError(t, err)
	require.Equal(t, "svp1owner", hit.Owner)
	require.Equal(t, "1000000", hit.Pricing.Amount)
}

func TestFindActiveByEndpointScansPagesAndNormalizesTrailingSlash(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/agents", r.URL.Path)
		switch r.URL.Query().Get("cursor") {
		case "0":
			_, _ = w.Write([]byte(`{"agents":[{"agent_id":"did:svp:first","endpoint":"https://first.example"}],"cursor":0,"next_cursor":1}`))
		case "1":
			_, _ = w.Write([]byte(`{"agents":[{"agent_id":"did:svp:target","endpoint":"https://target.example/a2a/","status":"AGENT_STATUS_ACTIVE"}],"cursor":1,"next_cursor":2}`))
		default:
			t.Fatalf("unexpected cursor %q", r.URL.Query().Get("cursor"))
		}
	}))
	t.Cleanup(server.Close)

	hit, found, err := New(server.URL).FindActiveByEndpoint(context.Background(), " https://target.example/a2a ")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "did:svp:target", hit.AgentID)
	require.Equal(t, "https://target.example/a2a", hit.Endpoint)
}
