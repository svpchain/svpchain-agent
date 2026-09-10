package settlement

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClientCreatesTask(t *testing.T) {
	var got Task
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/internal/v1/tasks", r.URL.Path)
		require.Equal(t, "Bearer token", r.Header.Get("Authorization"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"task_id":"` + testTask + `","state":"assigned","assignment_tx_hash":"` + testTx + `"}`))
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(server.URL, "token")
	require.NoError(t, err)
	assignment, err := client.CreateTask(context.Background(), Task{
		IntentID: testTx, TaskID: testTask, Amount: "1000000", Owner: "0x0000000000000000000000000000000000000004",
	})
	require.NoError(t, err)
	require.Equal(t, testTask, got.TaskID)
	require.Equal(t, "1000000", got.Amount)
	require.Equal(t, "assigned", assignment.State)
}

func TestClientReadsSettlementNetworkConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/settlement/config", r.URL.Path)
		_, _ = w.Write([]byte(`{"settlement_contract":"0x0000000000000000000000000000000000000001","payment_token":"0x0000000000000000000000000000000000000002"}`))
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(server.URL, "")
	require.NoError(t, err)
	config, err := client.NetworkConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, "0x0000000000000000000000000000000000000002", config.PaymentToken)
}

// Assignment waits on assignTask being mined; observed inclusion has exceeded
// fifty seconds, so the deadline must clear that tail rather than sit inside it.
func TestValidatorTimeoutClearsChainInclusion(t *testing.T) {
	client, err := NewClient("https://validator.example", "")
	require.NoError(t, err)
	require.GreaterOrEqual(t, client.httpClient.Timeout, 60*time.Second)
}

// The status line alone cannot distinguish a reverted assignTask from a
// rejected argument, which is what made a real assignment failure opaque.
func TestClientReportsTaskRefusalReason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"assignTask: execution reverted"}`))
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(server.URL, "")
	require.NoError(t, err)
	_, err = client.CreateTask(context.Background(), Task{
		IntentID: testTx, TaskID: testTask, Amount: "1000000", Owner: "0x0000000000000000000000000000000000000004",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "assignTask: execution reverted")
}
