package whitelist

import (
	"fmt"
	"strings"

	"github.com/svpchain/svpchain-agent/internal/evmcall"
)

// Enforced reports whether any whitelist entries exist. When true, outbound
// transfers must target an allowed recipient for the chain and address type.
func (s *Store) Enforced() bool {
	return s != nil && len(s.entries) > 0
}

// Allows reports whether address is whitelisted for chainID. Matching is by
// account, not by encoding: an entry authorizes address when both name the same
// on-chain account, so a recipient given in bech32 (svp1…) form is allowed by an
// EVM (0x…) whitelist entry for the same account and vice versa. addressType
// declares how address itself is encoded so it can be parsed to its raw bytes.
func (s *Store) Allows(chainID, addressType, address string) bool {
	if s == nil {
		return false
	}
	chainID, err := ValidateChainID(chainID)
	if err != nil {
		return false
	}
	want, ok := accountKey(chainID, addressType, address)
	if !ok {
		return false
	}
	for _, e := range s.entries {
		if got, ok := accountKey(e.ChainID, e.AddressType, e.Address); ok && got == want {
			return true
		}
	}
	return false
}

// CheckEVMTx checks every account an EVM transaction hands value or spending
// rights to against s:
//
//  1. the recipient of a native send — to, when value is positive; and
//  2. the recipient / spender / operator named INSIDE standard ERC-20, ERC-721
//     and ERC-1155 call data (see internal/evmcall).
//
// The second check is what makes the whitelist meaningful for token
// transactions. A token transfer or approval carries value 0 and addresses the
// token contract, so it is invisible to check 1 alone: without decoding the
// call data, "transfer my USDC to the attacker" and "approve the attacker for
// an unlimited amount" both pass a to/value-only check unexamined.
//
// Call data whose selector is a known token method but whose arguments will not
// decode is refused, not skipped — otherwise malformed arguments would be a
// bypass.
//
// This method does NOT consult Enforced(): the two enforcement layers disagree
// about what an empty whitelist means (the signer treats it as unrestricted,
// the assistant's gate as refuse-all), so each caller applies that policy itself
// and then calls this for the shared "given this whitelist, is the tx allowed?"
// decision.
func (s *Store) CheckEVMTx(chainID, to string, valuePositive bool, data []byte) error {
	if valuePositive && strings.TrimSpace(to) != "" {
		if !s.Allows(chainID, AddressTypeEVM, to) {
			return fmt.Errorf("recipient %q is not on the whitelist for chain %q (EVM)", to, chainID)
		}
	}

	dest, err := evmcall.Decode(data)
	if err != nil {
		return err
	}
	if dest == nil {
		return nil
	}
	addr := dest.Address.Hex()
	if !s.Allows(chainID, AddressTypeEVM, addr) {
		return fmt.Errorf("%s %q is not on the whitelist for chain %q (EVM %s)",
			dest.Role, addr, chainID, dest.Method)
	}
	return nil
}

// CheckCosmosRecipient returns an error when whitelist enforcement is active
// and recipient is not allowed for chainID.
func CheckCosmosRecipient(chainID, recipient string) error {
	store := LoadStore()
	if !store.Enforced() {
		return nil
	}
	if store.Allows(chainID, AddressTypeCosmos, recipient) {
		return nil
	}
	return fmt.Errorf("recipient %q is not on the whitelist for chain %q (SVP Cosmos)", recipient, chainID)
}

// CheckEVMRecipient returns an error when whitelist enforcement is active and
// recipient is not allowed for chainID.
func CheckEVMRecipient(chainID, recipient string) error {
	store := LoadStore()
	if !store.Enforced() {
		return nil
	}
	if store.Allows(chainID, AddressTypeEVM, recipient) {
		return nil
	}
	return fmt.Errorf("recipient %q is not on the whitelist for chain %q (EVM)", recipient, chainID)
}
