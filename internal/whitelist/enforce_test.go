package whitelist_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	appconfig "github.com/svpchain/svpchain-agent/internal/config"
	"github.com/svpchain/svpchain-agent/internal/prefs"
	"github.com/svpchain/svpchain-agent/internal/whitelist"
)

func TestCheckCosmosRecipient_enforced(t *testing.T) {
	appconfig.SetAddressPrefixes()
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
	writePrefs(t, path, []whitelist.Entry{{
		ChainID:     "svp-2517-1",
		AddressType: whitelist.AddressTypeCosmos,
		Address:     allowed,
	}})
	t.Cleanup(func() { prefs.SetPathOverride("") })
	prefs.SetPathOverride(path)

	require.NoError(t, whitelist.CheckCosmosRecipient("svp-2517-1", allowed))
	err := whitelist.CheckCosmosRecipient("svp-2517-1", blocked)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not on the whitelist")
}

// TestCheckCosmosRecipient_emptyWhitelistAllows locks in the signer-layer
// semantics: with no persisted entries the signer path is unrestricted. The
// hardcoded DefaultEntries belong to the assistant's LoadEffectiveStore, not to
// this signer path, so they must NOT make an empty user whitelist enforce here.
func TestCheckCosmosRecipient_emptyWhitelistAllows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prefs.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"whitelist":[]}`), 0o600))
	t.Cleanup(func() { prefs.SetPathOverride("") })
	prefs.SetPathOverride(path)

	require.NoError(t, whitelist.CheckCosmosRecipient("svp-2517-1", "svp1anything"))
}

// TestLoadEffectiveStore_defaultsWithoutUserEntries verifies the assistant's
// effective whitelist folds in the hardcoded defaults even when the user has
// saved none, and that this happens without writing the defaults to prefs.json.
func TestLoadEffectiveStore_defaultsWithoutUserEntries(t *testing.T) {
	appconfig.SetAddressPrefixes()
	dir := t.TempDir()
	path := filepath.Join(dir, "prefs.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"whitelist":[]}`), 0o600))
	t.Cleanup(func() { prefs.SetPathOverride("") })
	prefs.SetPathOverride(path)

	def := whitelist.DefaultEntries()[0]
	store := whitelist.LoadEffectiveStore()
	require.True(t, store.Enforced())
	require.True(t, store.Allows(def.ChainID, whitelist.AddressTypeEVM, def.Address))

	// The signer-layer view stays empty/unrestricted: defaults are not persisted.
	require.False(t, whitelist.LoadStore().Enforced())
	onDisk := prefs.Read()
	require.Empty(t, onDisk.Whitelist, "defaults must never be written to prefs.json")
}

// TestCheckRecipient_crossEncoding verifies that whitelisting an account in one
// encoding authorizes it in the other: a bech32 (svp1…) address and its 0x EVM
// form are the same on-chain account, so enforcement must match across the two.
func TestCheckRecipient_crossEncoding(t *testing.T) {
	appconfig.SetAddressPrefixes()
	raw := []byte{
		0x41, 0x42, 0x43, 0x44, 0x45, 0x46, 0x47, 0x48,
		0x49, 0x4a, 0x4b, 0x4c, 0x4d, 0x4e, 0x4f, 0x50,
		0x51, 0x52, 0x53, 0x54,
	}
	cosmosAddr := sdk.AccAddress(raw).String()
	evmAddr := common.BytesToAddress(raw).Hex()

	dir := t.TempDir()
	path := filepath.Join(dir, "prefs.json")
	// Whitelist ONLY the EVM form of the account.
	writePrefs(t, path, []whitelist.Entry{{
		ChainID:     "svp-2517-1",
		AddressType: whitelist.AddressTypeEVM,
		Address:     evmAddr,
	}})
	t.Cleanup(func() { prefs.SetPathOverride("") })
	prefs.SetPathOverride(path)

	// A bank send naming the same account in bech32 form must be allowed.
	require.NoError(t, whitelist.CheckCosmosRecipient("svp-2517-1", cosmosAddr))
	// The EVM form is still allowed directly.
	require.NoError(t, whitelist.CheckEVMRecipient("svp-2517-1", evmAddr))

	// A different account remains blocked in either encoding.
	other := sdk.AccAddress([]byte{
		0x61, 0x62, 0x63, 0x64, 0x65, 0x66, 0x67, 0x68,
		0x69, 0x6a, 0x6b, 0x6c, 0x6d, 0x6e, 0x6f, 0x70,
		0x71, 0x72, 0x73, 0x74,
	}).String()
	require.Error(t, whitelist.CheckCosmosRecipient("svp-2517-1", other))
}

func writePrefs(t *testing.T, path string, entries []whitelist.Entry) {
	t.Helper()
	data, err := json.Marshal(map[string]any{"whitelist": entries})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o600))
}
