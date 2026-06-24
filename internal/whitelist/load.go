package whitelist

import "github.com/svpchain/svpchain-agent/internal/prefs"

// LoadStore reads whitelist entries from prefs.json.
func LoadStore() *Store {
	f := prefs.Read()
	return NewStore(EntriesFromPrefs(f.Whitelist))
}
