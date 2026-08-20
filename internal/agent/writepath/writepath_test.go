package writepath

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

const cosmosBuild = `{
  "chain_id": "svp-2517-1",
  "signer_address": "svp1abc",
  "tx_body_bytes_b64": "AAEC",
  "summary": {"tool_name": "build_bank_send"}
}`

const evmBuild = `{
  "evm_chain_id": "2517",
  "to": "0x1111111111111111111111111111111111111111",
  "value": "1",
  "nonce": "0"
}`

func TestClassify(t *testing.T) {
	k, f := Classify("build_bank_send")
	require.Equal(t, KindBuild, k)
	require.Equal(t, Family(""), f)

	k, f = Classify("lendora_build_supply_tx")
	require.Equal(t, KindBuild, k)

	k, f = Classify("x402_build_payment")
	require.Equal(t, KindNone, k)

	k, f = Classify("sign_transaction")
	require.Equal(t, KindSign, k)
	require.Equal(t, FamilyCosmos, f)

	k, f = Classify("sign_evm_transaction")
	require.Equal(t, KindSign, k)
	require.Equal(t, FamilyEVM, f)

	k, f = Classify("sign_typed_data")
	require.Equal(t, KindNone, k)

	k, f = Classify("broadcast_signed_tx")
	require.Equal(t, KindBroadcast, k)
	require.Equal(t, FamilyCosmos, f)

	k, f = Classify("whoami")
	require.Equal(t, KindNone, k)
}

func TestHappyPathCosmos(t *testing.T) {
	tr := New()
	require.NoError(t, tr.Before("build_bank_send", nil))
	require.NoError(t, tr.After("build_bank_send", nil, cosmosBuild))

	payload := map[string]any{
		"summary":           map[string]any{"tool_name": "build_bank_send"},
		"chain_id":          "svp-2517-1",
		"tx_body_bytes_b64": "AAEC",
		"signer_address":    "svp1abc",
	}
	require.NoError(t, tr.Before("sign_transaction", map[string]any{"payload": payload}))
	require.NoError(t, tr.After("sign_transaction", nil, `{"signed_tx":{"tx_raw_bytes_b64":"raw","signature_b64":"sig","pub_key_b64":"pk"}}`))

	require.NoError(t, tr.Before("broadcast_signed_tx", map[string]any{
		"signed_tx": map[string]any{"signature_b64": "sig", "pub_key_b64": "pk", "tx_raw_bytes_b64": "raw"},
	}))
	require.NoError(t, tr.After("broadcast_signed_tx", nil, `{"tx_hash":"abc"}`))

	// Lane is clear: a new sign needs a new build.
	err := tr.Before("sign_transaction", map[string]any{"payload": payload})
	require.ErrorAs(t, err, new(*Violation))
}

func TestSignWithoutBuild(t *testing.T) {
	tr := New()
	err := tr.Before("sign_evm_transaction", map[string]any{
		"payload": map[string]any{"evm_chain_id": "2517", "to": "0x1", "value": "1"},
	})
	var v *Violation
	require.ErrorAs(t, err, &v)
	require.Contains(t, v.StopMessage(), "Write path rejected")
	require.Contains(t, v.Reason, "matching build_*")
}

func TestSignMutatedPayload(t *testing.T) {
	tr := New()
	require.NoError(t, tr.After("build_bank_send", nil, cosmosBuild))
	err := tr.Before("sign_transaction", map[string]any{
		"payload": map[string]any{
			"chain_id":          "svp-2517-1",
			"signer_address":    "svp1abc",
			"tx_body_bytes_b64": "MUTATED",
		},
	})
	require.ErrorAs(t, err, new(*Violation))
	require.Contains(t, err.Error(), "verbatim")
}

func TestBroadcastWithoutSign(t *testing.T) {
	tr := New()
	require.NoError(t, tr.After("build_bank_send", nil, cosmosBuild))
	err := tr.Before("broadcast_signed_tx", map[string]any{
		"signed_tx": map[string]any{"tx_raw_bytes_b64": "raw"},
	})
	require.ErrorAs(t, err, new(*Violation))
	require.Contains(t, err.Error(), "requires the signed_tx")
}

func TestBroadcastMutatedSignedTx(t *testing.T) {
	tr := New()
	require.NoError(t, tr.After("build_swap", nil, evmBuild))
	require.NoError(t, tr.Before("sign_evm_transaction", map[string]any{
		"payload": map[string]any{"evm_chain_id": "2517", "to": "0x1111111111111111111111111111111111111111", "value": "1", "nonce": "0"},
	}))
	require.NoError(t, tr.After("sign_evm_transaction", nil, `{"signed_tx":{"raw_tx_hex":"0xabc","tx_hash":"0x1"}}`))

	err := tr.Before("broadcast_evm_tx", map[string]any{
		"signed_tx": map[string]any{"raw_tx_hex": "0xEVIL", "tx_hash": "0x1"},
	})
	require.ErrorAs(t, err, new(*Violation))
	require.Contains(t, err.Error(), "altered")
}

func TestApprovalRequiredDoesNotRecord(t *testing.T) {
	tr := New()
	require.NoError(t, tr.After("lendora_build_supply_tx", nil, `{"approval_required":{"tool":"build_erc20_approve"}}`))
	err := tr.Before("sign_evm_transaction", map[string]any{
		"payload": map[string]any{"evm_chain_id": "2517"},
	})
	require.ErrorAs(t, err, new(*Violation))
}

func TestNestedPayloadAndIndependentLanes(t *testing.T) {
	tr := New()
	require.NoError(t, tr.After("lendora_build_supply_tx", nil, `{"simulation":{},"payload":`+evmBuild+`}`))
	require.NoError(t, tr.After("build_bank_send", nil, cosmosBuild))

	require.NoError(t, tr.Before("sign_evm_transaction", map[string]any{
		"payload": map[string]any{"nonce": "0", "evm_chain_id": "2517", "value": "1", "to": "0x1111111111111111111111111111111111111111"},
	}))
	require.NoError(t, tr.Before("sign_transaction", map[string]any{
		"payload": map[string]any{
			"tx_body_bytes_b64": "AAEC",
			"chain_id":          "svp-2517-1",
			"signer_address":    "svp1abc",
			"summary":           map[string]any{"tool_name": "build_bank_send"},
		},
	}))
}

func TestRebuildReplacesPending(t *testing.T) {
	tr := New()
	require.NoError(t, tr.After("build_bank_send", nil, cosmosBuild))
	require.NoError(t, tr.After("sign_transaction", nil, `{"signed_tx":{"tx_raw_bytes_b64":"old"}}`))

	other := `{
	  "chain_id": "svp-2517-1",
	  "signer_address": "svp1abc",
	  "tx_body_bytes_b64": "NEW"
	}`
	require.NoError(t, tr.After("build_bank_send", nil, other))
	err := tr.Before("broadcast_signed_tx", map[string]any{
		"signed_tx": map[string]any{"tx_raw_bytes_b64": "old"},
	})
	require.ErrorAs(t, err, new(*Violation))

	require.NoError(t, tr.Before("sign_transaction", map[string]any{
		"payload": map[string]any{
			"chain_id":          "svp-2517-1",
			"signer_address":    "svp1abc",
			"tx_body_bytes_b64": "NEW",
		},
	}))
}

func TestNilTrackerIsNoop(t *testing.T) {
	var tr *Tracker
	require.NoError(t, tr.Before("sign_transaction", nil))
	require.NoError(t, tr.After("build_bank_send", nil, cosmosBuild))
}

func TestViolationAsError(t *testing.T) {
	err := (&Violation{Reason: "payload does not match"}).StopMessage()
	require.True(t, errors.As(&Violation{}, new(*Violation)))
	require.Contains(t, err, "Write path rejected")
}
