package manage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/99designs/keyring"

	"github.com/svpchain/svpchain-agent/internal/brand"
	"github.com/svpchain/svpchain-agent/internal/keystore"
	"github.com/svpchain/svpchain-agent/internal/signer"
)

// Entry is one stored signing key: chain id (storage name) and derived Cosmos / EVM addresses.
type Entry struct {
	ChainID string
	Owner   string // SVP Cosmos bech32 address (svp1…)
	EVMAddr string
}

// ImportResult is returned after a successful import.
type ImportResult struct {
	Owner     string
	EVMAddr   string
	Conflicts []string // other chain ids sharing the same key (informational only)
}

// DefaultImportChainIDs are the default Chain ID dropdown options on the import tab.
var DefaultImportChainIDs = []string{"svp-2517-1", "svp-2518-1"}

// GenerateKey returns a fresh random private key as 0x-prefixed hex, suitable for
// pre-filling the import field so users need not paste an existing key.
func GenerateKey() (string, error) {
	return signer.GenPrivKeyHex()
}

// Import validates hexKey, stores it under chainID, and returns the derived owner plus cross-chain reuse warnings.
func Import(chainID, hexKey string) (ImportResult, error) {
	chainID = strings.TrimSpace(chainID)
	ring, err := keystore.Open()
	if err != nil {
		return ImportResult{}, fmt.Errorf("open OS credential store: %w", err)
	}
	owner, evmAddr, conflicts, err := importKey(ring, chainID, hexKey)
	if err != nil {
		return ImportResult{}, err
	}
	return ImportResult{Owner: owner, EVMAddr: evmAddr, Conflicts: conflicts}, nil
}

func importKey(ring keyring.Keyring, name, hexKey string) (owner, evmAddr string, conflicts []string, err error) {
	name = strings.TrimSpace(name)
	hexKey = strings.TrimSpace(hexKey)
	priv, err := signer.ParsePrivKey(hexKey)
	if err != nil {
		return "", "", nil, fmt.Errorf("invalid key: %w", err)
	}
	owner = signer.DeriveAddress(priv)
	evmAddr = signer.DeriveEVMAddress(priv)
	conflicts = findAddressReuse(ring, name, owner)
	if err := keystore.Store(ring, name, hexKey); err != nil {
		return "", "", nil, fmt.Errorf("store key: %w", err)
	}
	return owner, evmAddr, conflicts, nil
}

func findAddressReuse(ring keyring.Keyring, excludeName, owner string) []string {
	names, err := keystore.List(ring)
	if err != nil {
		return nil
	}
	var conflicts []string
	for _, nm := range names {
		if nm == excludeName {
			continue
		}
		hx, err := keystore.Load(ring, nm)
		if err != nil {
			continue
		}
		priv, err := signer.ParsePrivKey(hx)
		if err != nil {
			continue
		}
		if signer.DeriveAddress(priv) == owner {
			conflicts = append(conflicts, nm)
		}
	}
	sort.Strings(conflicts)
	return conflicts
}

// List returns all stored keys and their derived owner addresses.
func List() ([]Entry, error) {
	ring, err := keystore.Open()
	if err != nil {
		return nil, fmt.Errorf("open OS credential store: %w", err)
	}
	names, err := keystore.List(ring)
	if err != nil {
		return nil, fmt.Errorf("list keys: %w", err)
	}
	sort.Strings(names)
	out := make([]Entry, 0, len(names))
	for _, name := range names {
		hx, err := keystore.Load(ring, name)
		if err != nil {
			continue
		}
		priv, err := signer.ParsePrivKey(hx)
		if err != nil {
			continue
		}
		out = append(out, Entry{
			ChainID: name,
			Owner:   signer.DeriveAddress(priv),
			EVMAddr: signer.DeriveEVMAddress(priv),
		})
	}
	return out, nil
}

// Delete removes the key stored under chainID.
func Delete(chainID string) error {
	ring, err := keystore.Open()
	if err != nil {
		return fmt.Errorf("open OS credential store: %w", err)
	}
	if err := keystore.Delete(ring, chainID); err != nil {
		return fmt.Errorf("delete key %q: %w", chainID, err)
	}
	return nil
}

// SelectKey returns the hex key for name: OS credential store first, otherwise envHex (SIGNER_KEY_HEX).
// ring nil means the credential store could not be opened.
func SelectKey(ring keyring.Keyring, name, envHex string) (hexKey, source string, err error) {
	if ring != nil {
		hexKey, loadErr := keystore.Load(ring, name)
		if loadErr == nil {
			return hexKey, "OS credential store", nil
		}
		if !errors.Is(loadErr, keystore.ErrNotFound) {
			return "", "", fmt.Errorf("read key %q from OS credential store: %w", name, loadErr)
		}
	}
	if envHex != "" {
		return envHex, "SIGNER_KEY_HEX env", nil
	}
	return "", "", fmt.Errorf(
		"no signing key for %q: open %s, go to the Keys tab, select Chain ID %q, "+
			"import a private key or use Auto-generate to save it to the OS credential store; "+
			"for headless use, set SIGNER_KEY_HEX",
		name, brand.AppDisplayName, name,
	)
}

// MCPConfig is the JSON shape users paste into MCP client configuration.
type MCPConfig struct {
	MCPServers map[string]mcpServerEntry `json:"mcpServers"`
}

// MCPConfigJSON generates a formatted MCP config snippet from chainID and the absolute svpchain-mcp binary path.
func MCPConfigJSON(chainID, signerBinaryPath string) (string, error) {
	return MCPConfigText(AgentNameCursor, chainID, signerBinaryPath)
}

// GuessSignerBinaryPath looks for svpchain-mcp next to guiBinaryPath (typical release layout)
// and returns an absolute path; returns "" if not found.
func GuessSignerBinaryPath(guiBinaryPath string) string {
	if guiBinaryPath == "" {
		return ""
	}
	dir := filepath.Dir(guiBinaryPath)
	for _, name := range []string{"svpchain-mcp", "svpchain-mcp.exe"} {
		candidate := filepath.Join(dir, name)
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			if abs, err := filepath.Abs(candidate); err == nil {
				return abs
			}
			return candidate
		}
	}
	return ""
}
