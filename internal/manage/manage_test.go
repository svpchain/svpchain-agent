package manage

import (
	"errors"
	"strings"
	"testing"

	"github.com/99designs/keyring"
	"github.com/stretchr/testify/require"

	"github.com/svpchain/svpchain-mcp/internal/keystore"
)

const (
	keyA   = "bdac2376052bca8d4ae7c2012578f00ef5c2ce40c06bb0a06f986cd1e016c570"
	keyB   = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	ownerA = "svp153z9kqtxy0aawktsyyw996a8ezt7a5wpkgl377"
)

func newRing() keyring.Keyring { return keyring.NewArrayKeyring(nil) }

func TestImportKey_HappyPath(t *testing.T) {
	ring := newRing()
	owner, _, conflicts, err := importKey(ring, "localsvp-1", keyA)
	require.NoError(t, err)
	require.Equal(t, ownerA, owner)
	require.Empty(t, conflicts)

	stored, err := keystore.Load(ring, "localsvp-1")
	require.NoError(t, err)
	require.Equal(t, keyA, stored)
}

func TestImportKey_TrimsBeforeStore(t *testing.T) {
	ring := newRing()
	_, _, _, err := importKey(ring, "localsvp-1", "  "+keyA+"\n")
	require.NoError(t, err)
	stored, err := keystore.Load(ring, "localsvp-1")
	require.NoError(t, err)
	require.Equal(t, keyA, stored)
}

func TestImportKey_TrimsChainIDName(t *testing.T) {
	ring := newRing()
	_, _, _, err := importKey(ring, "  localsvp-1  ", keyA)
	require.NoError(t, err)
	_, err = keystore.Load(ring, "localsvp-1")
	require.NoError(t, err)
}

func TestImportKey_RejectsInvalid(t *testing.T) {
	ring := newRing()

	_, _, _, err := importKey(ring, "localsvp-1", "not-hex")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid key")

	_, _, _, err = importKey(ring, "localsvp-1", "dead")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid key")

	_, err = keystore.Load(ring, "localsvp-1")
	require.ErrorIs(t, err, keystore.ErrNotFound)
}

func TestImportKey_OverwriteSameNameNotConflict(t *testing.T) {
	ring := newRing()
	_, _, _, err := importKey(ring, "localsvp-1", keyA)
	require.NoError(t, err)

	_, _, conflicts, err := importKey(ring, "localsvp-1", keyA)
	require.NoError(t, err)
	require.Empty(t, conflicts)
}

func TestImportKey_DetectsCrossChainReuse(t *testing.T) {
	ring := newRing()
	_, _, _, err := importKey(ring, "testnet-1", keyA)
	require.NoError(t, err)

	owner, _, conflicts, err := importKey(ring, "mainnet-1", keyA)
	require.NoError(t, err)
	require.Equal(t, ownerA, owner)
	require.Equal(t, []string{"testnet-1"}, conflicts)
}

func TestImportKey_DifferentKeysNoConflict(t *testing.T) {
	ring := newRing()
	_, _, _, err := importKey(ring, "testnet-1", keyA)
	require.NoError(t, err)

	_, _, conflicts, err := importKey(ring, "mainnet-1", keyB)
	require.NoError(t, err)
	require.Empty(t, conflicts)
}

func TestSelectKey_KeyringHit(t *testing.T) {
	ring := newRing()
	require.NoError(t, keystore.Store(ring, "localsvp-1", keyA))

	hex, source, err := SelectKey(ring, "localsvp-1", "")
	require.NoError(t, err)
	require.Equal(t, keyA, hex)
	require.Equal(t, "OS credential store", source)
}

func TestSelectKey_EnvFallback(t *testing.T) {
	hex, source, err := SelectKey(newRing(), "localsvp-1", keyA)
	require.NoError(t, err)
	require.Equal(t, keyA, hex)
	require.Equal(t, "SIGNER_KEY_HEX env", source)

	ring := newRing()
	require.NoError(t, keystore.Store(ring, "localsvp-1", keyA))
	_, source, err = SelectKey(ring, "localsvp-1", keyB)
	require.NoError(t, err)
	require.Equal(t, "OS credential store", source)
}

func TestSelectKey_NilRingUsesEnv(t *testing.T) {
	hex, source, err := SelectKey(nil, "localsvp-1", keyA)
	require.NoError(t, err)
	require.Equal(t, keyA, hex)
	require.Equal(t, "SIGNER_KEY_HEX env", source)
}

func TestSelectKey_NoneError(t *testing.T) {
	_, _, err := SelectKey(newRing(), "localsvp-1", "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no signing key")
	require.Contains(t, err.Error(), "import")
	require.Contains(t, err.Error(), "SIGNER_KEY_HEX")
}

type errRing struct{ keyring.Keyring }

func (errRing) Get(string) (keyring.Item, error) {
	return keyring.Item{}, errors.New("access denied")
}

func TestSelectKey_SurfacesLoadError(t *testing.T) {
	_, _, err := SelectKey(errRing{newRing()}, "localsvp-1", keyA)
	require.Error(t, err)
	require.Contains(t, err.Error(), "access denied")
	require.NotContains(t, err.Error(), "no signing key")
}

func TestGuessSignerBinaryPath(t *testing.T) {
	got := GuessSignerBinaryPath("/Applications/svpchain agent.app/Contents/MacOS/svpchain-gui")
	require.True(t, strings.HasSuffix(got, "svpchain-mcp"))
}
