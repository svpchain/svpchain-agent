package whitelist

import "fmt"

// Enforced reports whether any whitelist entries exist. When true, outbound
// transfers must target an allowed recipient for the chain and address type.
func (s *Store) Enforced() bool {
	return s != nil && len(s.entries) > 0
}

// Allows reports whether address is whitelisted for chainID and addressType.
func (s *Store) Allows(chainID, addressType, address string) bool {
	if s == nil {
		return false
	}
	normalized, err := ValidateAddress(addressType, address)
	if err != nil {
		return false
	}
	chainID, err = ValidateChainID(chainID)
	if err != nil {
		return false
	}
	want := EntryKey(Entry{
		ChainID:     chainID,
		AddressType: addressType,
		Address:     normalized,
	})
	for _, e := range s.entries {
		if EntryKey(e) == want {
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
