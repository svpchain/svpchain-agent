package config

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	evmhd "github.com/cosmos/evm/crypto/hd"
)

const (
	AccountAddressPrefix = "svp"
	// Bech32MainPrefix is the main Bech32 prefix for account addresses.
	Bech32MainPrefix = AccountAddressPrefix
	// Bech32PrefixAccAddr is the Bech32 prefix for account addresses.
	Bech32PrefixAccAddr = Bech32MainPrefix
	// Bech32PrefixAccPub is the Bech32 prefix for account public keys.
	Bech32PrefixAccPub = Bech32MainPrefix + sdk.PrefixPublic
	// Bech32PrefixValAddr is the Bech32 prefix for validator operator addresses.
	Bech32PrefixValAddr = Bech32MainPrefix + sdk.PrefixValidator + sdk.PrefixOperator
	// Bech32PrefixValPub is the Bech32 prefix for validator operator public keys.
	Bech32PrefixValPub = Bech32MainPrefix + sdk.PrefixValidator + sdk.PrefixOperator + sdk.PrefixPublic
	// Bech32PrefixConsAddr is the Bech32 prefix for consensus node addresses.
	Bech32PrefixConsAddr = Bech32MainPrefix + sdk.PrefixValidator + sdk.PrefixConsensus
	// Bech32PrefixConsPub is the Bech32 prefix for consensus node public keys.
	Bech32PrefixConsPub = Bech32MainPrefix + sdk.PrefixValidator + sdk.PrefixConsensus + sdk.PrefixPublic
)

// SetupConfig configures and seals the global SDK config.
// Importing and calling this function also runs this package's init(), which sets address prefixes.
func SetupConfig() {
	config := sdk.GetConfig()
	config.SetCoinType(evmhd.Bip44CoinType)
	config.SetPurpose(sdk.Purpose)
	config.Seal()
}

func init() {
	// This package's import chain does not include app/config; set svp address prefixes explicitly.
	SetAddressPrefixes()
}

// SetAddressPrefixes sets the global prefixes used when serializing addresses and pubkeys as Bech32 strings.
func SetAddressPrefixes() {
	config := sdk.GetConfig()
	config.SetBech32PrefixForAccount(Bech32PrefixAccAddr, Bech32PrefixAccPub)
	config.SetBech32PrefixForValidator(Bech32PrefixValAddr, Bech32PrefixValPub)
	config.SetBech32PrefixForConsensusNode(Bech32PrefixConsAddr, Bech32PrefixConsPub)
}
