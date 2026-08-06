package settlement

import (
	"encoding/hex"
	"testing"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/gogoproto/proto"
	"github.com/stretchr/testify/require"
)

// Golden bytes produced once by marshaling the same messages with the chain's
// own generated types (svpagent protocol/x/settlement/types, commit current
// as of vendoring). If these tests fail after regenerating this package, the
// wire format diverged from the chain and every locally built transaction
// would be rejected or, worse, mean something else.
const (
	goldenOpenHex   = "0a28737670316f70656e657278787878787878787878787878787878787878787878787878787878787812100a0575757364631207313030303030301a0b7461736b5f676f6c64656e"
	goldenSettleHex = "0a28737670316f70656e657278787878787878787878787878787878787878787878787878787878787812200102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"
	goldenRefundHex = "0a28737670316f70656e657278787878787878787878787878787878787878787878787878787878787812200102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"
)

const goldenOpener = "svp1openerxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"

func goldenID() []byte {
	id := make([]byte, 32)
	for i := range id {
		id[i] = byte(i + 1)
	}
	return id
}

func TestMsgOpenSettlementMatchesChainWireFormat(t *testing.T) {
	bz, err := proto.Marshal(&MsgOpenSettlement{
		Opener: goldenOpener,
		Cap:    sdk.NewCoin("uusdc", sdkmath.NewInt(1000000)),
		Memo:   "task_golden",
	})
	require.NoError(t, err)
	require.Equal(t, goldenOpenHex, hex.EncodeToString(bz))
}

func TestMsgSettleMatchesChainWireFormat(t *testing.T) {
	bz, err := proto.Marshal(&MsgSettle{Opener: goldenOpener, SettlementId: goldenID()})
	require.NoError(t, err)
	require.Equal(t, goldenSettleHex, hex.EncodeToString(bz))
}

func TestMsgRefundSettlementMatchesChainWireFormat(t *testing.T) {
	bz, err := proto.Marshal(&MsgRefundSettlement{Opener: goldenOpener, SettlementId: goldenID()})
	require.NoError(t, err)
	require.Equal(t, goldenRefundHex, hex.EncodeToString(bz))
}
