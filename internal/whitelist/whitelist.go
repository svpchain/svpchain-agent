package whitelist

import (
	"fmt"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"

	appconfig "github.com/svpchain/svpchain-agent/internal/config"
)

const (
	AddressTypeCosmos = "cosmos"
	AddressTypeEVM    = "evm"
)

// Entry is one whitelisted address scoped to a chain id.
type Entry struct {
	ChainID     string `json:"chain_id"`
	AddressType string `json:"address_type"`
	Address     string `json:"address"`
}

func init() {
	appconfig.SetAddressPrefixes()
}

// ValidateAddress checks address against addressType and returns a normalized form.
func ValidateAddress(addressType, address string) (string, error) {
	switch strings.TrimSpace(addressType) {
	case AddressTypeCosmos:
		return normalizeCosmosAddress(address)
	case AddressTypeEVM:
		return normalizeEVMAddress(address)
	default:
		return "", fmt.Errorf("unsupported address type %q", addressType)
	}
}

func normalizeCosmosAddress(address string) (string, error) {
	s := strings.TrimSpace(address)
	if s == "" {
		return "", fmt.Errorf("address is required")
	}
	acc, err := sdk.AccAddressFromBech32(s)
	if err != nil {
		return "", fmt.Errorf("invalid SVP Cosmos address: %w", err)
	}
	normalized := acc.String()
	if !strings.HasPrefix(normalized, appconfig.Bech32PrefixAccAddr) {
		return "", fmt.Errorf("address must use the %s bech32 prefix", appconfig.Bech32PrefixAccAddr)
	}
	return normalized, nil
}

func normalizeEVMAddress(address string) (string, error) {
	s := strings.TrimSpace(address)
	if s == "" {
		return "", fmt.Errorf("address is required")
	}
	if !common.IsHexAddress(s) {
		return "", fmt.Errorf("invalid EVM address: must be a 0x-prefixed 20-byte hex string")
	}
	return common.HexToAddress(s).Hex(), nil
}

// ValidateChainID requires a non-empty chain id.
func ValidateChainID(chainID string) (string, error) {
	chainID = strings.TrimSpace(chainID)
	if chainID == "" {
		return "", fmt.Errorf("chain id is required")
	}
	return chainID, nil
}

// EntryKey returns a stable comparison key for an entry.
func EntryKey(e Entry) string {
	return strings.Join([]string{e.ChainID, e.AddressType, e.Address}, "\x00")
}
