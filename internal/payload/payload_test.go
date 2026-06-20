package payload_test

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/svpchain/svpchain-agent/internal/payload"
)

func TestTxPayload_JSONRoundTrip(t *testing.T) {
	orig := payload.TxPayload{
		Version:          payload.CurrentVersion,
		ClientID:         "5f9d8c4f-3a2b-4e11-8c2a-1234567890ab",
		ChainID:          "localsvp-1",
		SignerAddress:    "svp1exampleaddress",
		AccountNumber:    "42",
		Sequence:         "17",
		IsShortTermCLOB:  true,
		TxBodyBytesB64:   base64.StdEncoding.EncodeToString([]byte{0x01, 0x02, 0x03}),
		AuthInfoBytesB64: base64.StdEncoding.EncodeToString([]byte{0x04, 0x05}),
		SignBytesB64:     base64.StdEncoding.EncodeToString([]byte{0x06, 0x07, 0x08, 0x09}),
		Fee: payload.Fee{
			GasLimit: "1000000",
			Amount:   []payload.Coin{},
		},
		Summary: payload.Summary{
			ToolName:      "build_place_limit_order",
			MsgTypeURL:    "/dydxprotocol.clob.MsgPlaceOrder",
			Subaccount:    payload.SubaccountRef{Owner: "svp1exampleaddress", Number: 0},
			Ticker:        "BTC-USD",
			Side:          "BUY",
			SizeHuman:     "0.05",
			PriceHuman:    "65000.00",
			NotionalUSD:   "3250.00",
			GoodTil:       "block:1234500",
			OrderClientID: 1,
		},
		ExpiresAt: time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC),
	}

	bz, err := json.Marshal(orig)
	require.NoError(t, err)

	var got payload.TxPayload
	require.NoError(t, json.Unmarshal(bz, &got))
	require.Equal(t, orig, got, "round-trip must preserve every field")
}

func TestDirectSignBytes_Deterministic(t *testing.T) {
	body := []byte("body-bytes")
	auth := []byte("auth-info-bytes")
	chainID := "localsvp-1"
	accNum := uint64(42)

	a, err := payload.DirectSignBytes(body, auth, chainID, accNum)
	require.NoError(t, err)
	b, err := payload.DirectSignBytes(body, auth, chainID, accNum)
	require.NoError(t, err)
	require.Equal(t, a, b, "same inputs must produce identical sign bytes")
}

func TestDirectSignBytes_SensitiveToEachField(t *testing.T) {
	base, err := payload.DirectSignBytes([]byte("body"), []byte("auth"), "chain-a", 1)
	require.NoError(t, err)
	bodyDiff, err := payload.DirectSignBytes([]byte("body2"), []byte("auth"), "chain-a", 1)
	require.NoError(t, err)
	authDiff, err := payload.DirectSignBytes([]byte("body"), []byte("auth2"), "chain-a", 1)
	require.NoError(t, err)
	chainDiff, err := payload.DirectSignBytes([]byte("body"), []byte("auth"), "chain-b", 1)
	require.NoError(t, err)
	accDiff, err := payload.DirectSignBytes([]byte("body"), []byte("auth"), "chain-a", 2)
	require.NoError(t, err)

	require.NotEqual(t, base, bodyDiff)
	require.NotEqual(t, base, authDiff)
	require.NotEqual(t, base, chainDiff)
	require.NotEqual(t, base, accDiff)
}
