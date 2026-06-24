package whitelist

import (
	"fmt"
	"strings"
)

// Store holds whitelist entries in memory with basic CRUD helpers.
type Store struct {
	entries []Entry
}

// NewStore returns an empty store.
func NewStore(entries []Entry) *Store {
	if entries == nil {
		entries = []Entry{}
	}
	return &Store{entries: entries}
}

// List returns a copy of all entries sorted by chain id, type, then address.
func (s *Store) List() []Entry {
	out := make([]Entry, len(s.entries))
	copy(out, s.entries)
	sortEntries(out)
	return out
}

// Add validates and inserts an entry. Duplicate chain/type/address pairs are rejected.
func (s *Store) Add(chainID, addressType, address string) (Entry, error) {
	chainID, err := ValidateChainID(chainID)
	if err != nil {
		return Entry{}, err
	}
	address, err = ValidateAddress(addressType, address)
	if err != nil {
		return Entry{}, err
	}
	entry := Entry{
		ChainID:     chainID,
		AddressType: strings.TrimSpace(addressType),
		Address:     address,
	}
	key := EntryKey(entry)
	for _, existing := range s.entries {
		if EntryKey(existing) == key {
			return Entry{}, fmt.Errorf("whitelist entry already exists")
		}
	}
	s.entries = append(s.entries, entry)
	sortEntries(s.entries)
	return entry, nil
}

// Delete removes the matching entry. Returns an error if not found.
func (s *Store) Delete(chainID, addressType, address string) error {
	chainID = strings.TrimSpace(chainID)
	addressType = strings.TrimSpace(addressType)
	address = strings.TrimSpace(address)
	target := EntryKey(Entry{ChainID: chainID, AddressType: addressType, Address: address})
	for i, existing := range s.entries {
		if EntryKey(existing) == target {
			s.entries = append(s.entries[:i], s.entries[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("whitelist entry not found")
}

func sortEntries(entries []Entry) {
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			if entryLess(entries[j], entries[i]) {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}
}

func entryLess(a, b Entry) bool {
	if a.ChainID != b.ChainID {
		return a.ChainID < b.ChainID
	}
	if a.AddressType != b.AddressType {
		return a.AddressType < b.AddressType
	}
	return a.Address < b.Address
}
