package chainmsgs

import (
	"encoding/hex"
	"testing"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/gogoproto/proto"
	"github.com/stretchr/testify/require"
)

// Golden bytes produced once by marshaling the same messages with the chain's
// own generated types (svpagent protocol/x/agentwallet/types, commit current
// as of vendoring). If these tests fail after regenerating this package, the
// wire format diverged from the chain and every locally built transaction
// would be rejected or, worse, mean something else.
const (
	goldenCreateHex = "0a287376703164656c656761746f7278787878787878787878787878787878787878787878787878787812306469643a7376703a7376703164656c656761746f727878787878787878787878787878787878787878787878787878781a6c0a100a057575736463120731303030303030120f0a05757573646312063130303030302a0575757364633211636c6f622e63616e63656c5f6f726465723210636c6f622e706c6163655f6f726465723a12737670636861696e2d657865637574696f6e4a020007500258ac022080e6fe8907"
	goldenPauseHex  = "0a287376703164656c656761746f7278787878787878787878787878787878787878787878787878787812200102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"
)

func TestMsgCreateDelegationMatchesChainWireFormat(t *testing.T) {
	msg := &MsgCreateDelegation{
		Delegator: "svp1delegatorxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		AgentId:   "did:svp:svp1delegatorxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		Limits: Limits{
			SpendLimitTotal:    []sdk.Coin{{Denom: "uusdc", Amount: sdkmath.NewInt(1000000)}},
			SpendLimitDaily:    []sdk.Coin{{Denom: "uusdc", Amount: sdkmath.NewInt(100000)}},
			Denoms:             []string{"uusdc"},
			Actions:            []string{"clob.cancel_order", "clob.place_order"},
			Skills:             []string{"svpchain-execution"},
			Subaccounts:        []uint32{0, 7},
			MaxDepth:           2,
			MaxTokenTtlSeconds: 300,
		},
		ExpiresAt: 1900000000,
	}
	bz, err := proto.Marshal(msg)
	require.NoError(t, err)
	require.Equal(t, goldenCreateHex, hex.EncodeToString(bz))
}

func TestMsgPauseDelegationMatchesChainWireFormat(t *testing.T) {
	rootID := make([]byte, 32)
	for i := range rootID {
		rootID[i] = byte(i + 1)
	}
	msg := &MsgPauseDelegation{
		Delegator: "svp1delegatorxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		RootId:    rootID,
	}
	bz, err := proto.Marshal(msg)
	require.NoError(t, err)
	require.Equal(t, goldenPauseHex, hex.EncodeToString(bz))
}
