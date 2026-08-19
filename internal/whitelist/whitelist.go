package whitelist

import (
	"encoding/hex"
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
	// Alias is optional human-friendly metadata. It is NOT part of the entry's
	// identity (see EntryKey) and does not affect dedup, sorting, or matching.
	Alias string `json:"alias,omitempty"`
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

// EntryKey returns a stable comparison key for an entry. It keys on the exact
// (chain, type, address string) triple, so it is the identity used for storage
// dedup, sorting, and Delete — NOT for match/enforcement, where the same
// account whitelisted in either encoding must be treated as one (see accountKey).
func EntryKey(e Entry) string {
	return strings.Join([]string{e.ChainID, e.AddressType, e.Address}, "\x00")
}

// accountBytes returns the raw account bytes an address encodes, independent of
// its textual form. On svpchain a bech32 (svp1…) address and a 0x EVM address
// can be two encodings of the SAME 20-byte account, so enforcement keys on these
// bytes rather than the exact string: an account authorized in either form is
// authorized however a transaction happens to name it. The second result is
// false when address cannot be parsed for its declared type.
func accountBytes(addressType, address string) ([]byte, bool) {
	s := strings.TrimSpace(address)
	switch strings.TrimSpace(addressType) {
	case AddressTypeCosmos:
		acc, err := sdk.AccAddressFromBech32(s)
		if err != nil {
			return nil, false
		}
		return acc.Bytes(), true
	case AddressTypeEVM:
		if !common.IsHexAddress(s) {
			return nil, false
		}
		return common.HexToAddress(s).Bytes(), true
	default:
		return nil, false
	}
}

// accountKey reduces an address to a chain-scoped, encoding-independent match
// key over its raw account bytes. Two addresses share a key iff they name the
// same account on the same chain, regardless of bech32-vs-EVM encoding.
func accountKey(chainID, addressType, address string) (string, bool) {
	b, ok := accountBytes(addressType, address)
	if !ok {
		return "", false
	}
	return chainID + "\x00" + hex.EncodeToString(b), true
}
