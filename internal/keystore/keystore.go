package keystore

import (
	"errors"
	"fmt"

	"github.com/99designs/keyring"

	"github.com/svpchain/svpchain-agent/internal/brand"
)

// ServiceName is the service/collection name for signing keys in the OS credential store.
// On macOS Keychain it is the entry service; on Windows Credential Manager a prefix;
// on Linux Secret Service an attribute name.
const ServiceName = "svpchain-agent"

// ErrNotFound is returned by Load when no key exists under the given name.
// Callers (especially the serve path) use it to decide whether to fall back to SIGNER_KEY_HEX.
var ErrNotFound = errors.New("no key stored in the OS credential store")

// Open opens the OS credential store, limited to each platform's Keychain/Credential Manager/Secret Service backend.
// file, pass, kwallet, keyctl, and similar backends are excluded — keys must stay in OS-protected storage,
// not a project-specific on-disk format.
//
// The macOS Keychain backend requires a CGO build; release binaries are compiled with CGO_ENABLED=1.
// On headless Linux without Secret Service, Open (or a later Get) fails and callers fall back to SIGNER_KEY_HEX.
func Open() (keyring.Keyring, error) {
	return keyring.Open(keyring.Config{
		ServiceName: ServiceName,
		AllowedBackends: []keyring.BackendType{
			keyring.KeychainBackend,
			keyring.WinCredBackend,
			keyring.SecretServiceBackend,
		},
		// After the first authorization, later launches should not prompt on every read.
		KeychainTrustApplication:       true,
		KeychainAccessibleWhenUnlocked: true,
		// On Linux, store in the user's default login collection.
		LibSecretCollectionName: "login",
		WinCredPrefix:           ServiceName,
	})
}

// Store writes hexKey under name, overwriting any existing value (key rotation = import again).
// hexKey is a 32-byte private key as a hex string; format validation is the caller's responsibility.
func Store(ring keyring.Keyring, name, hexKey string) error {
	return ring.Set(keyring.Item{
		Key:         name,
		Data:        []byte(hexKey),
		Label:       fmt.Sprintf("%s (%s)", brand.AppDisplayName, name),
		Description: brand.AppDisplayName + " private key (hex)",
	})
}

// Load returns the hex key stored under name; returns ErrNotFound if missing.
func Load(ring keyring.Keyring, name string) (string, error) {
	item, err := ring.Get(name)
	if errors.Is(err, keyring.ErrKeyNotFound) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return string(item.Data), nil
}

// Delete removes the key under name; returns ErrNotFound if the entry does not exist.
func Delete(ring keyring.Keyring, name string) error {
	err := ring.Remove(name)
	if errors.Is(err, keyring.ErrKeyNotFound) {
		return ErrNotFound
	}
	return err
}

// List returns all key names (keyring entry keys) in the credential store.
func List(ring keyring.Keyring) ([]string, error) {
	return ring.Keys()
}
