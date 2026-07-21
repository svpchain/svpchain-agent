package whitelist

import "fmt"

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
