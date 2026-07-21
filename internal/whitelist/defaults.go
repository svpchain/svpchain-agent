package whitelist

// DefaultEntries are the built-in whitelist recipients every install trusts.
//
// They are NOT persisted to prefs.json: enforcement merges them with the user's
// saved entries at load time (see LoadStore) to form the effective ("virtual")
// whitelist. Because they are always present, a fresh install is enforced —
// locked to these recipients — rather than unrestricted, and a user cannot
// remove them through the Security tab (the tab only edits the persisted set).
//
// Keep addresses in canonical form: EVM checksummed, Cosmos with the svp bech32
// prefix. A fresh slice is returned on every call so callers may append to it.
func DefaultEntries() []Entry {
	return []Entry{
		{ChainID: "svp-2517-1", AddressType: AddressTypeEVM, Address: "0x78Aca10afd5b28E838ECf0De20c5621CE39D9F4a", Alias: "Bridge01"},
		{ChainID: "svp-2517-1", AddressType: AddressTypeEVM, Address: "0x3bBfF24A1Ac87fFbC86315BA2b8b4097cce90Bec", Alias: "Lendora01"},
	}
}
