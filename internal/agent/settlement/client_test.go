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
		IntentID: testTx, TaskID: testTask, Amount: "1000000", Owner: "0x0000000000000000000000000000000000000004", AgentIndex: "7",
	})
	require.NoError(t, err)
	require.Equal(t, testTask, got.TaskID)
	require.Equal(t, "1000000", got.Amount)
	require.Equal(t, "7", got.AgentIndex)
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

func TestClientReadsExecutionStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/executions/"+testTask, r.URL.Path)
		_, _ = w.Write([]byte(`{"task_id":"` + testTask + `","tx_hash":"` + testTx + `","chain":"evm","state":"received","attempts":3,"last_error":"bindExecution: execution reverted"}`))
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(server.URL, "")
	require.NoError(t, err)
	execution, found, err := client.Execution(context.Background(), testTask)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "received", execution.State)
	require.Equal(t, 3, execution.Attempts)
	require.Contains(t, execution.LastError, "reverted")
}

func TestClientExecutionNotFoundIsNotAnError(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(server.Close)

	client, err := NewClient(server.URL, "")
	require.NoError(t, err)
	_, found, err := client.Execution(context.Background(), testTask)
	require.NoError(t, err)
	require.False(t, found)
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
		IntentID: testTx, TaskID: testTask, Amount: "1000000", Owner: "0x0000000000000000000000000000000000000004", AgentIndex: "1",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "assignTask: execution reverted")
}

func TestAgentIndexFromID(t *testing.T) {
	for name, test := range map[string]struct {
		agentID string
		want    string
		wantErr string
	}{
		"indexed DID":    {agentID: "did:svp:svp14dme9z6nayv00g77lvxh88d42mfmxqzxwqve5s:42", want: "42"},
		"legacy DID":     {agentID: "did:svp:svp14dme9z6nayv00g77lvxh88d42mfmxqzxwqve5s", want: "0"},
		"missing prefix": {agentID: "svp1owner:1", wantErr: "must start"},
		"zero index":     {agentID: "did:svp:svp1owner:0", wantErr: "positive"},
		"leading zero":   {agentID: "did:svp:svp1owner:01", wantErr: "canonical"},
		"non decimal":    {agentID: "did:svp:svp1owner:one", wantErr: "non-decimal"},
		"extra suffix":   {agentID: "did:svp:svp1owner:1:2", wantErr: "invalid DID suffix"},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := AgentIndexFromID(test.agentID)
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
}
