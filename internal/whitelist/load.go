package whitelist

import "github.com/svpchain/svpchain-agent/internal/prefs"

// LoadStore reads the user's saved whitelist entries from prefs.json. This is
// the signer-layer view, where an empty list means "unrestricted" (see
// enforce.go). The GUI assistant instead uses LoadEffectiveStore, which folds in
// the hardcoded DefaultEntries and treats the result as mandatory.
func LoadStore() *Store {
	f := prefs.Read()
	return NewStore(EntriesFromPrefs(f.Whitelist))
}

// LoadEffectiveStore builds the assistant's effective ("virtual") whitelist: the
// hardcoded DefaultEntries merged with the user's saved entries. Defaults come
// first and win on exact-key collisions; a user entry equal to a default is
// dropped. The defaults are never written to prefs.json — they live only in
// code and are merged in here — so the assistant is always locked to at least
// the defaults while the persisted set stays exactly what the user added.
func LoadEffectiveStore() *Store {
	f := prefs.Read()
	merged := append(DefaultEntries(), EntriesFromPrefs(f.Whitelist)...)
	seen := make(map[string]struct{}, len(merged))
	entries := make([]Entry, 0, len(merged))
	for _, e := range merged {
		k := EntryKey(e)
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		entries = append(entries, e)
	}
	return NewStore(entries)
}
