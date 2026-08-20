package hitl

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSignRequest_cosmosSummary(t *testing.T) {
	req := SignRequest("sign_transaction", map[string]any{
		"payload": map[string]any{
			"chain_id":       "svp_2517-1",
			"signer_address": "svp1abc",
			"summary": map[string]any{
				"tool_name":       "build_bank_send",
				"amount_human":    "1.5 svp",
				"recipient_owner": "svp1dest",
			},
			"fee": map[string]any{
				"amount": []map[string]any{{"denom": "asvp", "amount": "2000"}},
			},
		},
	})
	require.Equal(t, KindSignTransaction, req.Kind)
	require.Equal(t, "Sign Cosmos transaction", req.Title)
	require.Contains(t, req.Lines, "Chain: svp_2517-1")
	require.Contains(t, req.Lines, "Recipient: svp1dest")
	require.Contains(t, req.Lines, "Amount: 1.5 svp")
	require.Contains(t, req.Lines, "Fee: 2000 asvp")
	for _, line := range req.Lines {
		require.NotContains(t, line, "sign_bytes")
		require.NotContains(t, line, "tx_body_bytes")
	}
}

func TestSignRequest_evmNativeAndToken(t *testing.T) {
	native := SignRequest("sign_evm_transaction", map[string]any{
		"payload": map[string]any{
			"evm_chain_id":   "2517",
			"signer_address": "0xabc",
			"to":             "0x1111111111111111111111111111111111111111",
			"value":          "1000",
			"summary":        map[string]any{"tool_name": "build_swap"},
		},
	})
	require.Equal(t, KindSignEVM, native.Kind)
	require.Contains(t, native.Lines, "To: 0x1111111111111111111111111111111111111111")
	require.Contains(t, native.Lines, "Value (wei): 1000")

	// ERC-20 transfer(address,uint256) selector 0xa9059cbb + padded address + amount.
	data := "0xa9059cbb0000000000000000000000002222222222222222222222222222222222222222000000000000000000000000000000000000000000000000000000000000000a"
	token := SignRequest("sign_evm_transaction", map[string]any{
		"payload": map[string]any{
			"evm_chain_id": "2517",
			"to":           "0x3333333333333333333333333333333333333333",
			"value":        "0",
			"data":         data,
		},
	})
	joined := ""
	for _, line := range token.Lines {
		joined += line
	}
	require.Contains(t, joined, "recipient")
	require.Contains(t, joined, "0x2222222222222222222222222222222222222222")
}

func TestSignRequest_typedData(t *testing.T) {
	req := SignRequest("sign_typed_data", map[string]any{
		"typed_data": map[string]any{
			"primaryType": "TransferWithAuthorization",
			"domain": map[string]any{
				"name":              "USD Coin",
				"verifyingContract": "0xcontract",
				"chainId":           "2517",
			},
			"message": map[string]any{
				"from":  "0xfrom",
				"to":    "0xto",
				"value": "50",
			},
		},
	})
	require.Equal(t, KindSignTypedData, req.Kind)
	require.Contains(t, req.Lines, "Type: TransferWithAuthorization")
	require.Contains(t, req.Lines, "to: 0xto")
	require.Contains(t, req.Lines, "value: 50")
}

func TestSignRequest_missingPayloadStillAsks(t *testing.T) {
	req := SignRequest("sign_transaction", nil)
	require.Equal(t, KindSignTransaction, req.Kind)
	require.NotEmpty(t, req.Lines)
	require.Contains(t, req.Lines[0], "Could not parse")
}
