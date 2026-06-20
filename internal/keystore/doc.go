// Package keystore stores and retrieves signing private keys in the OS credential store via 99designs/keyring —
// macOS Keychain, Windows Credential Manager, or Linux Secret Service (libsecret), and only those native backends.
//
// Keys are never written to a project-specific on-disk format: Open excludes file/pass/kwallet/keyctl backends.
// Stored values are 32-byte private keys as hex strings, named by the caller (default chain id) so multiple
// chain keys can coexist and be inspected/rotated with OS tools.
//
// Store/Load/Delete/List operate on keyring.Keyring; tests can use keyring.NewArrayKeyring in memory
// without touching a real OS credential store.
package keystore
