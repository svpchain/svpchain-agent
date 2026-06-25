package agent

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/svpchain/svpchain-agent/internal/signer"
)

func samplePaymentRequiredB64(t *testing.T) string {
	t.Helper()
	req := map[string]any{
		"x402Version": 2,
		"accepts": []map[string]any{{
			"scheme":            "exact",
			"network":           "eip155:2517",
			"asset":             "0x013a61E622e6ABFCaB64F52D274C3Fc0aA37f951",
			"amount":            "10000",
			"payTo":             "0xfBd15a89383f82FC869DbAb85480056812722852",
			"maxTimeoutSeconds": 60,
			"extra":             map[string]string{"name": "VanToken", "version": "1"},
		}},
	}
	bz, err := json.Marshal(req)
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(bz)
}

func TestPrepareX402TypedData_GeneratesValidNonce(t *testing.T) {
	from := "0x18cE6b725D5Fa498210bC1788DAcfA5bc14dbadc"
	result, err := PrepareX402TypedData(samplePaymentRequiredB64(t), from)
	require.NoError(t, err)
	require.Equal(t, signer.PrimaryTypeTransferWithAuthorization, result.TypedData.PrimaryType)

	nonce, ok := result.TypedData.Message["nonce"].(string)
	require.True(t, ok)
	require.True(t, strings.HasPrefix(nonce, "0x"))
	require.Len(t, strings.TrimPrefix(nonce, "0x"), 64, "nonce must be 32 bytes hex")
	require.NotEmpty(t, result.ValidBefore)
	require.Equal(t, "exact", result.Accepted.Scheme)
}

func TestPrepareX402TypedData_RejectsEmptyPaymentRequired(t *testing.T) {
	_, err := PrepareX402TypedData("", "0x18cE6b725D5Fa498210bC1788DAcfA5bc14dbadc")
	require.Error(t, err)
}

func TestX402BuildPaymentFromArgs(t *testing.T) {
	raw, err := x402BuildPaymentFromArgs(map[string]any{
		"accepted": map[string]any{
			"scheme":  "exact",
			"network": "eip155:2517",
			"asset":   "0x013a61E622e6ABFCaB64F52D274C3Fc0aA37f951",
			"amount":  "10000",
			"payTo":   "0xfBd15a89383f82FC869DbAb85480056812722852",
		},
		"signature": "0xabc",
		"authorization": map[string]any{
			"from":        "0x18cE6b725D5Fa498210bC1788DAcfA5bc14dbadc",
			"to":          "0xfBd15a89383f82FC869DbAb85480056812722852",
			"value":       "10000",
			"validAfter":  "0",
			"validBefore": "1782371578",
			"nonce":       "0x6824a62c6c7712fdde705cf7fee92673e28a6af59e9db4f6e278dbd15b05d41a",
		},
	})
	require.NoError(t, err)
	require.Contains(t, raw, "payment_b64")
}
