package desktop

import (
	"github.com/svpchain/svpchain-agent/internal/prefs"
	"github.com/svpchain/svpchain-agent/internal/whitelist"
)

// WhitelistEntry is one whitelisted address for the Security tab.
// Field names are PascalCase for Wails JSON bindings (same as manage.Entry).
type WhitelistEntry struct {
	ChainID     string
	AddressType string
	Address     string
	Alias       string
}

func toWhitelistEntry(e prefs.WhitelistEntry) WhitelistEntry {
	return WhitelistEntry{
		ChainID:     e.ChainID,
		AddressType: e.AddressType,
		Address:     e.Address,
		Alias:       e.Alias,
	}
}

// ListWhitelist returns saved whitelist entries.
func (a *App) ListWhitelist() []WhitelistEntry {
	entries := a.store.ListWhitelist()
	out := make([]WhitelistEntry, len(entries))
	for i, e := range entries {
		out[i] = toWhitelistEntry(e)
	}
	return out
}

// AddWhitelist validates and saves a whitelist entry.
func (a *App) AddWhitelist(chainID, addressType, address, alias string) (WhitelistEntry, error) {
	var added prefs.WhitelistEntry
	err := a.store.UpdateErr(func(f *prefs.File) error {
		store := whitelist.NewStore(whitelist.EntriesFromPrefs(f.Whitelist))
		entry, err := store.Add(chainID, addressType, address, alias)
		if err != nil {
			return err
		}
		f.Whitelist = whitelist.EntriesToPrefs(store.List())
		added = prefs.WhitelistEntry{
			ChainID:     entry.ChainID,
			AddressType: entry.AddressType,
			Address:     entry.Address,
			Alias:       entry.Alias,
		}
		return nil
	})
	if err != nil {
		return WhitelistEntry{}, localized(err)
	}
	return toWhitelistEntry(added), nil
}

// DeleteWhitelist removes a whitelist entry.
func (a *App) DeleteWhitelist(chainID, addressType, address string) error {
	err := a.store.UpdateErr(func(f *prefs.File) error {
		store := whitelist.NewStore(whitelist.EntriesFromPrefs(f.Whitelist))
		if err := store.Delete(chainID, addressType, address); err != nil {
			return err
		}
		f.Whitelist = whitelist.EntriesToPrefs(store.List())
		return nil
	})
	return localized(err)
}
