package keystore_test

import (
	"errors"
	"sort"
	"testing"

	"github.com/99designs/keyring"
	"github.com/stretchr/testify/require"

	"github.com/svpchain/svpchain-agent/internal/keystore"
)

const sampleHex = "bdac2376052bca8d4ae7c2012578f00ef5c2ce40c06bb0a06f986cd1e016c570"

// newRing returns an in-memory keyring for testing Store/Load/Delete/List without touching the real OS store.
func newRing() keyring.Keyring {
	return keyring.NewArrayKeyring(nil)
}

func TestStore_LoadRoundTrip(t *testing.T) {
	ring := newRing()
	require.NoError(t, keystore.Store(ring, "localsvp-1", sampleHex))

	got, err := keystore.Load(ring, "localsvp-1")
	require.NoError(t, err)
	require.Equal(t, sampleHex, got)
}

func TestLoad_NotFound(t *testing.T) {
	ring := newRing()
	_, err := keystore.Load(ring, "missing")
	require.ErrorIs(t, err, keystore.ErrNotFound)
}

func TestStore_Overwrites(t *testing.T) {
	// Key rotation is import again: Store under the same name overwrites the previous value.
	ring := newRing()
	require.NoError(t, keystore.Store(ring, "localsvp-1", sampleHex))
	const other = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	require.NoError(t, keystore.Store(ring, "localsvp-1", other))

	got, err := keystore.Load(ring, "localsvp-1")
	require.NoError(t, err)
	require.Equal(t, other, got)
}

func TestDelete_RemovesKey(t *testing.T) {
	ring := newRing()
	require.NoError(t, keystore.Store(ring, "localsvp-1", sampleHex))
	require.NoError(t, keystore.Delete(ring, "localsvp-1"))

	_, err := keystore.Load(ring, "localsvp-1")
	require.ErrorIs(t, err, keystore.ErrNotFound)
}

func TestList_ReturnsNames(t *testing.T) {
	ring := newRing()
	require.NoError(t, keystore.Store(ring, "localsvp-1", sampleHex))
	require.NoError(t, keystore.Store(ring, "localsvp-2", sampleHex))

	names, err := keystore.List(ring)
	require.NoError(t, err)
	sort.Strings(names)
	require.Equal(t, []string{"localsvp-1", "localsvp-2"}, names)
}

func TestErrNotFound_IsSentinel(t *testing.T) {
	// Preserve the public sentinel so callers can rely on errors.Is for SIGNER_KEY_HEX fallback.
	require.True(t, errors.Is(keystore.ErrNotFound, keystore.ErrNotFound))
}
