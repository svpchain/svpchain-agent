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
//
// The set covers the protocol contracts an assistant-driven agent spends to when
// it uses the svpchain-mcp build_* tools: the pre-flight gate checks the spender
// of build_erc20_approve and the recipient of transfers/bridge deposits, so any
// contract the agent must approve or send to has to be here or the run is
// refused before signing. Concretely: the SVPBridge (bridge deposits approve and
// send to it), the UniswapV2 router (swaps approve it as spender), the Lendora
// cToken markets (supply/repay approve the cToken as spender), and Permit2 (x402
// / gasless approvals). Read-only tools (quotes, balances, order book) and
// output-to-self swaps are not gated and need no entry.
func DefaultEntries() []Entry {
	return []Entry{
		// SVPBridge (svpchain) — build_bridge_deposit sends/approves here.
		{ChainID: "svp-2517-1", AddressType: AddressTypeEVM, Address: "0x78Aca10afd5b28E838ECf0De20c5621CE39D9F4a", Alias: "Bridge01"},

		// Foreign SVPBridge contracts — build_bridge_deposit_inbound approves these
		// as spender on the source chain when bridging an ERC-20 INTO svpchain. The
		// gate keys every call to the app chain id and matches by raw address bytes,
		// so they live under svp-2517-1 even though they are deployed on the foreign
		// chains (Sepolia / Arbitrum Sepolia).
		{ChainID: "svp-2517-1", AddressType: AddressTypeEVM, Address: "0xb9a9937006E886F0Ec145a19634426300dD20a64", Alias: "BridgeSepolia"},
		{ChainID: "svp-2517-1", AddressType: AddressTypeEVM, Address: "0xB6c74A758E3fA7bf57c22037821f7cA974d0CdfD", Alias: "BridgeArbSepolia"},

		// UniswapV2 router — build_swap approves this as the spender of the input
		// ERC-20 before swapping.
		{ChainID: "svp-2517-1", AddressType: AddressTypeEVM, Address: "0xFe7bf2DFd5CB268C6779f1F614638a436Cb701e4", Alias: "SwapRouter01"},

		// Permit2 — canonical spender for x402 / gasless (EIP-2612/Permit2) approvals.
		{ChainID: "svp-2517-1", AddressType: AddressTypeEVM, Address: "0x000000000022D473030F116dDEE9F6B43aC78BA3", Alias: "Permit2"},

		// Lendora cToken markets — lendora supply/repay approve the cToken (not the
		// comptroller) as the spender of the underlying.
		{ChainID: "svp-2517-1", AddressType: AddressTypeEVM, Address: "0xc67a1F0B635522E5dAdBBFAFd5aA24a3176EDF3e", Alias: "LendoraCSVP"},
		{ChainID: "svp-2517-1", AddressType: AddressTypeEVM, Address: "0x4a2ac95D82414237A1062EA4B816dAe46950Cf56", Alias: "LendoraCWETH"},
		{ChainID: "svp-2517-1", AddressType: AddressTypeEVM, Address: "0xC647A36ea112109E6B341399f665F10cEaEecEC3", Alias: "LendoraCUSDC"},
		{ChainID: "svp-2517-1", AddressType: AddressTypeEVM, Address: "0x6653b238548927c15A5dd2046af15C88018BF1aa", Alias: "LendoraCWBTC"},
		{ChainID: "svp-2517-1", AddressType: AddressTypeEVM, Address: "0x668cC3523050cd8ef48e0fd1210F32013E630612", Alias: "LendoraCWBNB"},
	}
}
