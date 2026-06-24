package whitelist_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	appconfig "github.com/svpchain/svpchain-agent/internal/config"
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
	t.Cleanup(func() { whitelist.SetPrefsPathOverride("") })
	whitelist.SetPrefsPathOverride(path)

	require.NoError(t, whitelist.CheckCosmosRecipient("svp-2517-1", allowed))
	err := whitelist.CheckCosmosRecipient("svp-2517-1", blocked)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not on the whitelist")
}

func TestCheckCosmosRecipient_emptyWhitelistAllows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prefs.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"whitelist":[]}`), 0o600))
	t.Cleanup(func() { whitelist.SetPrefsPathOverride("") })
	whitelist.SetPrefsPathOverride(path)

	require.NoError(t, whitelist.CheckCosmosRecipient("svp-2517-1", "svp1anything"))
}

func writePrefs(t *testing.T, path string, entries []whitelist.Entry) {
	t.Helper()
	data, err := json.Marshal(map[string]any{"whitelist": entries})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o600))
}
