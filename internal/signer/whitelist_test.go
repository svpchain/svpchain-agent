package signer_test

import (
	"os"
	"path/filepath"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	appconfig "github.com/svpchain/svpchain-agent/internal/config"
	"github.com/svpchain/svpchain-agent/internal/payload"
	"github.com/svpchain/svpchain-agent/internal/prefs"
	"github.com/svpchain/svpchain-agent/internal/signer"
)

func TestSign_RejectsNonWhitelistedBankSend(t *testing.T) {
	appconfig.SetAddressPrefixes()
	priv := newRandomPriv(t)
	from := signer.DeriveAddress(priv)
	allowed := sdk.AccAddress([]byte{
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15,
	}).String()
	blocked := sdk.AccAddress([]byte{
		0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28,
		0x29, 0x2a, 0x2b, 0x2c, 0x2d, 0x2e, 0x2f, 0x30,
		0x31, 0x32, 0x33, 0x34, 0x35,
	}).String()

	dir := t.TempDir()
	path := filepath.Join(dir, "prefs.json")
	prefsData := []byte(`{"whitelist":[{"chain_id":"localsvp-1","address_type":"cosmos","address":"` + allowed + `"}]}`)
	require.NoError(t, os.WriteFile(path, prefsData, 0o600))
	t.Cleanup(func() { prefs.SetPathOverride("") })
	prefs.SetPathOverride(path)

	_, err := signer.Sign(priv, payloadWithBody(marshalBody(t, bankSendAny(t, from, blocked)), payload.Summary{
		MsgTypeURL: "/cosmos.bank.v1beta1.MsgSend",
	}))
	require.Error(t, err)
	require.Contains(t, err.Error(), "not on the whitelist")

	signed, err := signer.Sign(priv, payloadWithBody(marshalBody(t, bankSendAny(t, from, allowed)), payload.Summary{
		MsgTypeURL: "/cosmos.bank.v1beta1.MsgSend",
	}))
	require.NoError(t, err)
	require.NotEmpty(t, signed.TxRawBytesB64)
}
