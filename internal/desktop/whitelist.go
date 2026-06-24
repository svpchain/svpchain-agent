package desktop

import "github.com/svpchain/svpchain-agent/internal/whitelist"

// WhitelistEntry is one whitelisted address for the Security tab.
// Field names are PascalCase for Wails JSON bindings (same as manage.Entry).
type WhitelistEntry struct {
	ChainID     string
	AddressType string
	Address     string
}

func toWhitelistEntry(e whitelist.Entry) WhitelistEntry {
	return WhitelistEntry{
		ChainID:     e.ChainID,
		AddressType: e.AddressType,
		Address:     e.Address,
	}
}

// ListWhitelist returns saved whitelist entries.
func (a *App) ListWhitelist() []WhitelistEntry {
	entries := a.prefs.listWhitelist()
	out := make([]WhitelistEntry, len(entries))
	for i, e := range entries {
		out[i] = toWhitelistEntry(e)
	}
	return out
}

// AddWhitelist validates and saves a whitelist entry.
func (a *App) AddWhitelist(chainID, addressType, address string) (WhitelistEntry, error) {
	entry, err := a.prefs.addWhitelist(chainID, addressType, address)
	if err != nil {
		return WhitelistEntry{}, err
	}
	return toWhitelistEntry(entry), nil
}

// DeleteWhitelist removes a whitelist entry.
func (a *App) DeleteWhitelist(chainID, addressType, address string) error {
	return a.prefs.deleteWhitelist(chainID, addressType, address)
}
