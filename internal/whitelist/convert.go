package whitelist

import "github.com/svpchain/svpchain-agent/internal/prefs"

// EntriesFromPrefs converts persisted whitelist rows to domain entries.
func EntriesFromPrefs(in []prefs.WhitelistEntry) []Entry {
	if len(in) == 0 {
		return nil
	}
	out := make([]Entry, len(in))
	for i, e := range in {
		out[i] = Entry{
			ChainID:     e.ChainID,
			AddressType: e.AddressType,
			Address:     e.Address,
		}
	}
	return out
}

// EntriesToPrefs converts domain entries for persistence.
func EntriesToPrefs(in []Entry) []prefs.WhitelistEntry {
	if len(in) == 0 {
		return nil
	}
	out := make([]prefs.WhitelistEntry, len(in))
	for i, e := range in {
		out[i] = prefs.WhitelistEntry{
			ChainID:     e.ChainID,
			AddressType: e.AddressType,
			Address:     e.Address,
		}
	}
	return out
}
