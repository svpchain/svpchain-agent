package a2acall

import (
	"context"
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"

	"github.com/svpchain/svpchain-agent/internal/agent/local"
	"github.com/svpchain/svpchain-agent/internal/registry"
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

	out, err := SendFromArgs(context.Background(), map[string]any{
		"agent_url": "http://localhost:9001",
		"message":   "ping",
	})
	require.NoError(t, err)
	require.Contains(t, out, "pong")
}

func TestLendingComptrollerFromVerifiedCard(t *testing.T) {
	card := registry.Card{Verified: true, Raw: []byte(`{
  "skills": [
    {"id":"svpchain-lendora","description":"Tools: lendora_build_collateral_tx."},
    {"id":"svpchain-execution-lendora","description":"Runtime Lendora deployment: Comptroller 0x000000000000000000000000000000000000c0de; enabled ABI method signatures: enterMarkets(address[])."}
  ]
}`)}
	got, err := lendingComptrollerFromCard(card)
	require.NoError(t, err)
	require.Equal(t, "0x000000000000000000000000000000000000c0de", got)

	card.Verified = false
	_, err = lendingComptrollerFromCard(card)
	require.ErrorContains(t, err, "hash")
}

func TestCollateralPayloadFromResponseChecksIdentityTargetAndSelector(t *testing.T) {
	const owner = "0x0000000000000000000000000000000000000abc"
	const comptroller = "0x000000000000000000000000000000000000c0de"
	selector := crypto.Keccak256([]byte("enterMarkets(address[])"))[:4]
	response := fmt.Sprintf(`{
  "skill":"svpchain-lendora",
  "tool":"lendora_build_collateral_tx",
  "ok":true,
  "result":{"payload":{
    "evm_chain_id":"2517",
    "signer_address":%q,
    "to":%q,
    "value":"0",
    "nonce":"1",
    "gas":"21000",
    "gas_price":"1",
    "data":"0x%x",
    "summary":{"tool_name":"lendora_build_collateral_tx"}
  }}
}`, owner, comptroller, selector)
	p, err := collateralPayloadFromResponse(response, "enable", owner, "2517", comptroller)
	require.NoError(t, err)
	require.Equal(t, comptroller, p.To)

	_, err = collateralPayloadFromResponse(response, "disable", owner, "2517", comptroller)
	require.ErrorContains(t, err, "exitMarket")
}

func TestA2ASendFromArgsValidation(t *testing.T) {
	t.Parallel()
	_, err := SendFromArgs(context.Background(), map[string]any{"message": "hi"})
	require.Error(t, err)
	_, err = SendFromArgs(context.Background(), map[string]any{"agent_url": "http://x"})
	require.Error(t, err)
}

func TestLocalToolDefsIncludesA2A(t *testing.T) {
	t.Parallel()
	var found bool
	for _, tool := range local.ToolDefs() {
		if tool.Function.Name == "a2a_send_message" {
			found = true
			break
		}
	}
	require.True(t, found)
}
