package settlement

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	appconfig "github.com/svpchain/svpchain-agent/internal/config"
)

func TestNewTaskIDs(t *testing.T) {
	payer := common.HexToAddress("0x1111111111111111111111111111111111111111")
	intentID, taskID, err := NewTaskIDs(payer)
	require.NoError(t, err)
	require.Len(t, intentID, 66)
	require.Len(t, taskID, 66)
	require.NotEqual(t, common.Hash{}, common.HexToHash(intentID))
	require.NotEqual(t, common.Hash{}, common.HexToHash(taskID))
	require.NotEqual(t, intentID, taskID)
}

func TestEVMOwner(t *testing.T) {
	appconfig.SetAddressPrefixes()
	want := common.HexToAddress("0x1111111111111111111111111111111111111111")
	got, err := EVMOwner(want.Hex())
	require.NoError(t, err)
	require.Equal(t, want, got)

	cosmosOwner := sdk.AccAddress(want.Bytes()).String()
	got, err = EVMOwner(cosmosOwner)
	require.NoError(t, err)
	require.Equal(t, want, got)
}
