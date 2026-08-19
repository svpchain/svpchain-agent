package signer_test

import (
	"os"
	"path/filepath"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
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

// evmWhitelistChainID is the chain id the EVM whitelist tests below key entries
// on; it is the Cosmos chain id string, not the numeric EVM one.
const evmWhitelistChainID = "localsvp-1"

// writeEVMWhitelist points prefs at a temp file whitelisting exactly allowed.
func writeEVMWhitelist(t *testing.T, allowed string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "prefs.json")
	data := []byte(`{"whitelist":[{"chain_id":"` + evmWhitelistChainID +
		`","address_type":"evm","address":"` + allowed + `"}]}`)
	require.NoError(t, os.WriteFile(path, data, 0o600))
	t.Cleanup(func() { prefs.SetPathOverride("") })
	prefs.SetPathOverride(path)
}

// erc20Call encodes selector + left-padded 32-byte words as 0x hex call data.
func erc20Call(t *testing.T, sig string, words ...[]byte) string {
	t.Helper()
	out := ethcrypto.Keccak256([]byte(sig))[:4]
	for _, w := range words {
		padded := make([]byte, 32)
		copy(padded[32-len(w):], w)
		out = append(out, padded...)
	}
	return hexutil.Encode(out)
}

// A token transfer addresses the TOKEN contract and carries value 0, so the
// beneficiary exists only in the call data. Without decoding it the signer would
// happily sign a transfer of the whole balance to an attacker.
func TestSignEvm_RejectsNonWhitelistedERC20Transfer(t *testing.T) {
	priv := newRandomPriv(t)
	addr := signer.DeriveEvmAddress(priv)

	allowedRecipient := common.HexToAddress("0x1111111111111111111111111111111111111111")
	attackerAddr := common.HexToAddress("0x2222222222222222222222222222222222222222")
	tokenContract := "0x3333333333333333333333333333333333333333"

	writeEVMWhitelist(t, allowedRecipient.Hex())

	newTokenTx := func(recipient common.Address) *payload.EvmTxPayload {
		p := newEvmPayload(addr)
		p.To = tokenContract // the token, not the beneficiary
		p.Value = "0"        // a token transfer moves no native value
		p.Data = erc20Call(t, "transfer(address,uint256)", recipient.Bytes(), []byte{0x05})
		return p
	}

	_, err := signer.SignEvm(priv, newTokenTx(attackerAddr), evmWhitelistChainID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not on the whitelist")
	require.Contains(t, err.Error(), attackerAddr.Hex())

	signed, err := signer.SignEvm(priv, newTokenTx(allowedRecipient), evmWhitelistChainID)
	require.NoError(t, err)
	require.NotEmpty(t, signed.RawTxHex)
}

// The infinite-approval case: approve(attacker, 2^256-1) hands over the whole
// balance for later, and must be refused for the same reason as a transfer.
func TestSignEvm_RejectsNonWhitelistedERC20Approve(t *testing.T) {
	priv := newRandomPriv(t)
	addr := signer.DeriveEvmAddress(priv)

	allowedSpender := common.HexToAddress("0x1111111111111111111111111111111111111111")
	attackerAddr := common.HexToAddress("0x2222222222222222222222222222222222222222")

	writeEVMWhitelist(t, allowedSpender.Hex())

	maxUint := make([]byte, 32)
	for i := range maxUint {
		maxUint[i] = 0xff
	}

	newApproval := func(spender common.Address) *payload.EvmTxPayload {
		p := newEvmPayload(addr)
		p.To = "0x3333333333333333333333333333333333333333"
		p.Value = "0"
		p.Data = erc20Call(t, "approve(address,uint256)", spender.Bytes(), maxUint)
		return p
	}

	_, err := signer.SignEvm(priv, newApproval(attackerAddr), evmWhitelistChainID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not on the whitelist")
	require.Contains(t, err.Error(), "spender")

	_, err = signer.SignEvm(priv, newApproval(allowedSpender), evmWhitelistChainID)
	require.NoError(t, err)
}

// Revoking an approval must stay possible even for a spender that is not (or is
// no longer) whitelisted, or the whitelist could trap an allowance.
func TestSignEvm_AllowsApprovalRevocation(t *testing.T) {
	priv := newRandomPriv(t)
	addr := signer.DeriveEvmAddress(priv)
	writeEVMWhitelist(t, common.HexToAddress("0x1111111111111111111111111111111111111111").Hex())

	untrusted := common.HexToAddress("0x2222222222222222222222222222222222222222")
	p := newEvmPayload(addr)
	p.To = "0x3333333333333333333333333333333333333333"
	p.Value = "0"
	p.Data = erc20Call(t, "approve(address,uint256)", untrusted.Bytes(), nil)

	_, err := signer.SignEvm(priv, p, evmWhitelistChainID)
	require.NoError(t, err)
}

// Call data using a known token selector but undecodable arguments must be
// refused rather than passed through as "nothing to check".
func TestSignEvm_RejectsMalformedTokenCallData(t *testing.T) {
	priv := newRandomPriv(t)
	addr := signer.DeriveEvmAddress(priv)
	writeEVMWhitelist(t, common.HexToAddress("0x1111111111111111111111111111111111111111").Hex())

	p := newEvmPayload(addr)
	p.To = "0x3333333333333333333333333333333333333333"
	p.Value = "0"
	// transfer selector with the amount word missing.
	p.Data = erc20Call(t, "transfer(address,uint256)",
		common.HexToAddress("0x2222222222222222222222222222222222222222").Bytes())

	_, err := signer.SignEvm(priv, p, evmWhitelistChainID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "truncated")
}

// Signer-layer policy is unchanged: with no whitelist configured the signer
// stays unrestricted (the assistant's gate is the layer that refuses outright).
func TestSignEvm_EmptyWhitelistStaysUnrestricted(t *testing.T) {
	priv := newRandomPriv(t)
	addr := signer.DeriveEvmAddress(priv)

	path := filepath.Join(t.TempDir(), "prefs.json")
	require.NoError(t, os.WriteFile(path, []byte(`{}`), 0o600))
	t.Cleanup(func() { prefs.SetPathOverride("") })
	prefs.SetPathOverride(path)

	p := newEvmPayload(addr)
	p.To = "0x3333333333333333333333333333333333333333"
	p.Value = "0"
	p.Data = erc20Call(t, "transfer(address,uint256)",
		common.HexToAddress("0x2222222222222222222222222222222222222222").Bytes(), []byte{0x05})

	signed, err := signer.SignEvm(priv, p, evmWhitelistChainID)
	require.NoError(t, err)
	require.NotEmpty(t, signed.RawTxHex)
}

// Contract calls the decoder does not model must keep working when they move no
// native value — gating them would break every swap, order and lending flow.
func TestSignEvm_UnknownSelectorNotGated(t *testing.T) {
	priv := newRandomPriv(t)
	addr := signer.DeriveEvmAddress(priv)
	writeEVMWhitelist(t, common.HexToAddress("0x1111111111111111111111111111111111111111").Hex())

	p := newEvmPayload(addr)
	p.To = "0x3333333333333333333333333333333333333333" // not whitelisted
	p.Value = "0"
	p.Data = erc20Call(t, "mint(uint256)", []byte{0x01})

	_, err := signer.SignEvm(priv, p, evmWhitelistChainID)
	require.NoError(t, err)
}
