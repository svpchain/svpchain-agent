package runlog

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractIntents_fromBuildArgsAndSummary(t *testing.T) {
	ok := true
	steps := []Step{
		{Kind: "tool", Tool: "whoami", OK: &ok, Result: `{"owner":"svp1x"}`},
		{Kind: "tool", Tool: "build_bank_send", OK: &ok, Args: `{"recipient":"svp1abc","amount":"12","denom":"asvp"}`},
		{Kind: "tool", Tool: "build_place_limit_order", OK: &ok, Result: `{"summary":{"ticker":"BTC-USD","side":"BUY","size_human":"0.05"}}`},
		{Kind: "tool", Tool: "build_erc20_transfer", OK: &ok, Args: `{"to":"0xabc","amount":"1"}`},
	}
	got := ExtractIntents(steps)
	require.Len(t, got, 3)
	require.Equal(t, "bank_send", got[0].Kind)
	require.Equal(t, "svp1abc", got[0].Expect["recipient"])
	require.Equal(t, "place_order", got[1].Kind)
	require.Equal(t, "BTC-USD", got[1].Expect["ticker"])
	require.Equal(t, "0.05", got[1].Expect["size"])
	require.Equal(t, "erc20_transfer", got[2].Kind)
}

func TestMatchIntents_bankSend(t *testing.T) {
	ok := true
	intents := ExtractIntents([]Step{{
		Kind: "tool", Tool: "build_bank_send", OK: &ok,
		Args: `{"recipient":"svp1abc","amount":"12"}`,
	}})
	events := []ChainEvent{{
		Type: "transfer",
		Attrs: map[string]string{
			"recipient": "svp1abc",
			"amount":    "12000000asvp",
		},
	}}
	checks := []TxCheck{{Status: TxConfirmed}}

	matched := MatchIntents(intents, checks, events, true)
	require.Equal(t, IntentMatched, matched[0].Status)

	wrong := MatchIntents(intents, checks, []ChainEvent{{
		Type:  "transfer",
		Attrs: map[string]string{"recipient": "svp1other"},
	}}, true)
	require.Equal(t, IntentMismatch, wrong[0].Status)

	pending := MatchIntents(intents, []TxCheck{{Status: TxPending}}, nil, true)
	require.Equal(t, IntentUnobserved, pending[0].Status)

	skipped := MatchIntents(intents, nil, nil, false)
	require.Equal(t, IntentSkipped, skipped[0].Status)
}

func TestMatchIntents_evmIncluded(t *testing.T) {
	ok := true
	intents := ExtractIntents([]Step{{
		Kind: "tool", Tool: "build_erc20_transfer", OK: &ok,
		Args: `{"to":"0xabc"}`,
	}})
	got := MatchIntents(intents, []TxCheck{{Status: TxConfirmed}}, nil, true)
	require.Equal(t, IntentIncluded, got[0].Status)
}

func TestMatchIntents_placeOrder(t *testing.T) {
	ok := true
	intents := ExtractIntents([]Step{{
		Kind: "tool", Tool: "build_place_limit_order", OK: &ok,
		Args: `{"ticker":"BTC-USD","side":"BUY"}`,
	}})
	events := []ChainEvent{{
		Type:  "place_order",
		Attrs: map[string]string{"ticker": "BTC-USD", "side": "BUY"},
	}}
	got := MatchIntents(intents, []TxCheck{{Status: TxConfirmed}}, events, true)
	require.Equal(t, IntentMatched, got[0].Status)

	wrongSide := MatchIntents(intents, []TxCheck{{Status: TxConfirmed}}, []ChainEvent{{
		Type:  "place_order",
		Attrs: map[string]string{"ticker": "BTC-USD", "side": "SELL"},
	}}, true)
	require.Equal(t, IntentMismatch, wrongSide[0].Status)
}
