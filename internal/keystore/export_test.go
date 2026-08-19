package keystore

import "github.com/99designs/keyring"

// SelectRingForTest exposes selectRing to tests in package keystore_test.
func SelectRingForTest(primary keyring.Keyring, primaryErr error, legacy keyring.Keyring, legacyErr error) (keyring.Keyring, error) {
	return selectRing(primary, primaryErr, legacy, legacyErr)
}
