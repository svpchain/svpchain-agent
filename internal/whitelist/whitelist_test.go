package whitelist

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	appconfig "github.com/svpchain/svpchain-agent/internal/config"
)

func TestValidateCosmosAddress(t *testing.T) {
	appconfig.SetAddressPrefixes()
	valid := sdk.AccAddress([]byte{
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15,
	}).String()
	got, err := ValidateAddress(AddressTypeCosmos, valid)
	require.NoError(t, err)
	require.Equal(t, valid, got)

	_, err = ValidateAddress(AddressTypeCosmos, "cosmos1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq0fr2sh")
	require.Error(t, err)

	_, err = ValidateAddress(AddressTypeCosmos, "not-an-address")
	require.Error(t, err)
}

func TestValidateEVMAddress(t *testing.T) {
	got, err := ValidateAddress(AddressTypeEVM, "0x0000000000000000000000000000000000000001")
	require.NoError(t, err)
	require.Equal(t, "0x0000000000000000000000000000000000000001", got)

	_, err = ValidateAddress(AddressTypeEVM, "0x1234")
	require.Error(t, err)

	_, err = ValidateAddress(AddressTypeEVM, "svp1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqmva6er")
	require.Error(t, err)
}

func TestStoreAddDelete(t *testing.T) {
	s := NewStore(nil)
	_, err := s.Add("svp-2517-1", AddressTypeEVM, "0x0000000000000000000000000000000000000001")
	require.NoError(t, err)

	_, err = s.Add("svp-2517-1", AddressTypeEVM, "0x0000000000000000000000000000000000000001")
	require.Error(t, err)

	list := s.List()
	require.Len(t, list, 1)

	require.NoError(t, s.Delete("svp-2517-1", AddressTypeEVM, "0x0000000000000000000000000000000000000001"))
	require.Empty(t, s.List())

	require.Error(t, s.Delete("svp-2517-1", AddressTypeEVM, "0x0000000000000000000000000000000000000001"))
}
