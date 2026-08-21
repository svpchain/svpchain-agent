package chainrpc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/svpchain/svpchain-agent/internal/agent/runlog"
)

func TestURLForChain(t *testing.T) {
	require.Equal(t, TestnetURL, URLForChain("svp-2517-1"))
	require.Equal(t, TestnetURL, URLForChain("svp_2517-1"))
	require.Empty(t, URLForChain("localsvp-1"))
	require.Empty(t, Lookup(""))
}

func TestQueryHash(t *testing.T) {
	h := "71c53e6da77b0ef60d0b053661521a786c5e3ec2a84e90ac50fc3df393dd6ced"
	require.Equal(t, "0x"+h, QueryHash("0x"+h))
	require.Equal(t, "0x"+h, QueryHash(h))
	require.Equal(t, "0x"+h, QueryHash("0x"+strings.ToUpper(h)))
	require.Empty(t, QueryHash(""))
}

func TestLookup_parsesCometTx(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "tx.json"))
	require.NoError(t, err)

	var gotHash string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/tx", r.URL.Path)
		gotHash = r.URL.Query().Get("hash")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	lookup := Lookup(srv.URL)
	require.NotNil(t, lookup)
	tx, err := lookup(context.Background(), "71C53E6DA77B0EF60D0B053661521A786C5E3EC2A84E90AC50FC3DF393DD6CED")
	require.NoError(t, err)
	require.Equal(t, "0x71c53e6da77b0ef60d0b053661521a786c5e3ec2a84e90ac50fc3df393dd6ced", gotHash)
	require.Equal(t, uint32(0), tx.Code)
	require.Equal(t, "6413575", tx.Height)
	require.Equal(t, "svp1qec3nef5x7kkmksml08ucw747dm9h6yldy5y2d", tx.Events[2].Attrs["recipient"])

	ok := true
	intents := runlog.ExtractIntents([]runlog.Step{{
		Kind: "tool", Tool: "build_bank_send", OK: &ok,
		Args: `{"recipient":"svp1qec3nef5x7kkmksml08ucw747dm9h6yldy5y2d"}`,
	}})
	got := runlog.MatchIntents(intents, []runlog.TxCheck{{Status: runlog.TxConfirmed}}, tx.Events, true)
	require.Equal(t, runlog.IntentMatched, got[0].Status)

	wrong := runlog.MatchIntents(intents, []runlog.TxCheck{{Status: runlog.TxConfirmed}}, tx.Events[:1], true)
	require.Equal(t, runlog.IntentMismatch, wrong[0].Status)
}

func TestLookup_notFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":-1,"error":{"code":-32603,"message":"Internal error","data":"Tx (71C5) not found"}}`))
	}))
	t.Cleanup(srv.Close)

	_, err := Lookup(srv.URL)(context.Background(), "0x71c5")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}
