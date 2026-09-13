package settlement

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	testTask = "0x2000000000000000000000000000000000000000000000000000000000000002"
	testTx   = "0x3000000000000000000000000000000000000000000000000000000000000003"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "svpchain-settlement-test-*")
	if err != nil {
		panic(err)
	}
	SetCallbackOutboxPathOverride(filepath.Join(dir, "callbacks.json"))
	code := m.Run()
	SetCallbackOutboxPathOverride("")
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func TestReporterPostsExplicitTaskAndDelegatedSource(t *testing.T) {
	var got struct {
		TaskID string `json:"task_id"`
		TxHash string `json:"tx_hash"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/internal/v1/executions", r.URL.Path)
		require.Equal(t, "Bearer token", r.Header.Get("Authorization"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(server.Close)

	reporter, err := New(Config{TaskID: testTask, Owner: "0x0000000000000000000000000000000000000004", ValidatorURL: server.URL, CallbackToken: "token", Source: SourceCosmosDelegatedEVM})
	require.NoError(t, err)
	reporter.RecordTool("execute_delegated_evm", "")(true, `{"tx_hash":"`+testTx+`"}`, "")
	require.NoError(t, reporter.Report(context.Background()))
	require.Equal(t, testTask, got.TaskID)
	require.Equal(t, testTx, got.TxHash)
}

func TestReporterCallbackMatchesValidatorStrictRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var raw map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&raw))
		require.Equal(t, map[string]any{"task_id": testTask, "tx_hash": testTx}, raw)
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(server.Close)

	reporter, err := New(Config{TaskID: testTask, Owner: "0x0000000000000000000000000000000000000004", ValidatorURL: server.URL, Source: SourceEVM})
	require.NoError(t, err)
	reporter.RecordTool("broadcast_evm_tx", "")(true, `{"tx_hash":"`+testTx+`"}`, "")
	require.NoError(t, reporter.Report(context.Background()))
	taskID, txHash, ok := reporter.Callback()
	require.True(t, ok)
	require.Equal(t, testTask, taskID)
	require.Equal(t, testTx, txHash)
}

func TestReporterCallbackIsAbsentWithoutExecution(t *testing.T) {
	reporter, err := New(Config{TaskID: testTask, ValidatorURL: "http://validator.example"})
	require.NoError(t, err)
	_, _, ok := reporter.Callback()
	require.False(t, ok)
}

func TestReporterQueuesCallbackWhenBroadcastIsObserved(t *testing.T) {
	defer os.Remove(callbackOutboxPath())
	reporter, err := New(Config{TaskID: testTask, ValidatorURL: "https://validator.example"})
	require.NoError(t, err)
	reporter.RecordTool("broadcast_evm_tx", "")(true, `{"tx_hash":"`+testTx+`"}`, "")

	pending, err := PendingCallbacks()
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Equal(t, testTask, pending[0].TaskID)
	require.Equal(t, testTx, pending[0].TxHash)
}

func TestReporterPostsFinalExecutionOfWorkflow(t *testing.T) {
	var got struct {
		TaskID string `json:"task_id"`
		TxHash string `json:"tx_hash"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(server.Close)

	reporter, err := New(Config{TaskID: testTask, ValidatorURL: server.URL})
	require.NoError(t, err)
	reporter.RecordTool("broadcast_evm_tx", "")(true, `{"tx_hash":"0x3000000000000000000000000000000000000000000000000000000000000003"}`, "")
	reporter.RecordTool("broadcast_evm_tx", "")(true, `{"tx_hash":"0x4000000000000000000000000000000000000000000000000000000000000004"}`, "")
	reporter.RecordTool("broadcast_evm_tx", "")(true, `{"tx_hash":"`+testTx+`"}`, "")

	require.NoError(t, reporter.Report(context.Background()))
	require.Equal(t, testTask, got.TaskID)
	require.Equal(t, testTx, got.TxHash)
	taskID, txHash, ok := reporter.Callback()
	require.True(t, ok)
	require.Equal(t, testTask, taskID)
	require.Equal(t, testTx, txHash)
}

func TestReporterIgnoresApprovalBroadcastBeforeSwap(t *testing.T) {
	var got struct {
		TaskID string `json:"task_id"`
		TxHash string `json:"tx_hash"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(server.Close)

	reporter, err := New(Config{TaskID: testTask, ValidatorURL: server.URL})
	require.NoError(t, err)
	reporter.RecordTool("build_token_approval", "")(true, `{"payload":{"client_id":"approval-client"}}`, "")
	reporter.RecordTool("broadcast_evm_tx", `{"client_id":"approval-client"}`)(true, `{"tx_hash":"0x4000000000000000000000000000000000000000000000000000000000000004"}`, "")
	reporter.RecordTool("broadcast_evm_tx", `{"client_id":"swap-client"}`)(true, `{"tx_hash":"`+testTx+`"}`, "")

	require.NoError(t, reporter.Report(context.Background()))
	require.Equal(t, testTask, got.TaskID)
	require.Equal(t, testTx, got.TxHash)
}

func TestReporterIgnoresSettlementFundingHashesReturnedByConnect(t *testing.T) {
	t.Cleanup(func() { _ = os.Remove(callbackOutboxPath()) })
	reporter, err := New(Config{TaskID: testTask, ValidatorURL: "http://validator.example"})
	require.NoError(t, err)
	reporter.RecordTool("a2a_connect_agent", "")(true, `{
        "approve_tx_hash":"0x3000000000000000000000000000000000000000000000000000000000000003",
        "deposit_tx_hash":"0x4000000000000000000000000000000000000000000000000000000000000004"
    }`, "")
	reporter.RecordTool("sign_evm_transaction", "")(true, `{"signed_tx":{"tx_hash":"0x5000000000000000000000000000000000000000000000000000000000000005"}}`, "")
	reporter.RecordTool("broadcast_evm_tx", "")(true, `{"tx_hash":"`+testTx+`"}`, "")

	reporter.mu.Lock()
	defer reporter.mu.Unlock()
	require.Equal(t, testTx, reporter.finalHash)
}

func TestNilReporterIsNoopObserver(t *testing.T) {
	var reporter *Reporter
	require.NotPanics(t, func() {
		reporter.RecordTool("sign_evm_transaction", "{}")(true, `{"signed_tx":{"tx_hash":"`+testTx+`"}}`, "")
	})
}

func TestDeferredReporterIgnoresPreFundingBroadcasts(t *testing.T) {
	reporter := NewDeferred()
	reporter.RecordTool("broadcast_evm_tx", "")(true, `{"tx_hash":"`+testTx+`"}`, "")
	require.NoError(t, reporter.Configure(Config{TaskID: testTask, ValidatorURL: "http://validator.example"}))
	require.NoError(t, reporter.Report(context.Background()))
}

func TestReporterPersistsFailedCallbackForStartupRetry(t *testing.T) {
	accept := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if !accept {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(server.Close)

	reporter, err := New(Config{TaskID: testTask, ValidatorURL: server.URL})
	require.NoError(t, err)
	reporter.RecordTool("broadcast_evm_tx", "")(true, `{"tx_hash":"`+testTx+`"}`, "")
	require.Error(t, reporter.Report(context.Background()))

	pending, err := PendingCallbacks()
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Equal(t, testTask, pending[0].TaskID)
	require.Equal(t, testTx, pending[0].TxHash)
	require.Equal(t, 1, pending[0].Attempts)

	accept = true
	require.NoError(t, RetryPendingCallbacks(context.Background()))
	pending, err = PendingCallbacks()
	require.NoError(t, err)
	require.Empty(t, pending)
}
