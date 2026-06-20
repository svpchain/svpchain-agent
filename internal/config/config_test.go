package config_test

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"
	"github.com/svpchain/svpchain-agent/internal/config"
	"testing"
)

func TestSetupConfig_SealsConfig(t *testing.T) {
	sdkConfig := sdk.GetConfig()

	// A successful Set means the config is not yet sealed.
	sdkConfig.SetPurpose(0)
	require.Equal(t, uint32(0), sdkConfig.GetPurpose(), "Expected purpose to match set value")

	// Should apply default app values and seal the config.
	config.SetupConfig()

	require.Panicsf(t, func() { sdkConfig.SetPurpose(0) }, "Expected config to be sealed after SetupConfig")
}

func TestSetAddressPrefixes(t *testing.T) {
	sdkConfig := sdk.GetConfig()

	require.Equal(t, "svp", sdkConfig.GetBech32AccountAddrPrefix())
	require.Equal(t, "svppub", sdkConfig.GetBech32AccountPubPrefix())

	require.Equal(t, "svpvaloper", sdkConfig.GetBech32ValidatorAddrPrefix())
	require.Equal(t, "svpvaloperpub", sdkConfig.GetBech32ValidatorPubPrefix())

	require.Equal(t, "svpvalcons", sdkConfig.GetBech32ConsensusAddrPrefix())
	require.Equal(t, "svpvalconspub", sdkConfig.GetBech32ConsensusPubPrefix())
}
