package update

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsNewer(t *testing.T) {
	require.True(t, IsNewer("1.0.2", "1.0.1"))
	require.False(t, IsNewer("1.0.1", "1.0.1"))
	require.False(t, IsNewer("1.0.0", "1.0.1"))
	require.True(t, IsNewer("v2.0.0", "1.9.9"))
}

func TestIsDevVersion(t *testing.T) {
	require.True(t, IsDevVersion("0.1.0-dev"))
	require.True(t, IsDevVersion(""))
	require.False(t, IsDevVersion("1.0.0"))
}

func TestExpectedHashFromSums(t *testing.T) {
	sums := []byte("abc123  svpchain-agent-1.0.2-macos.dmg\n")
	got, err := expectedHashFromSums(sums, "svpchain-agent-1.0.2-macos.dmg")
	require.NoError(t, err)
	require.Equal(t, "abc123", got)
}

func TestVerifyZipChecksum(t *testing.T) {
	dir := t.TempDir()
	dmgPath := filepath.Join(dir, "svpchain-agent-1.0.0-macos.dmg")
	require.NoError(t, os.WriteFile(dmgPath, []byte("payload"), 0o644))

	got, err := hashFile(dmgPath)
	require.NoError(t, err)
	sums := []byte(got + "  svpchain-agent-1.0.0-macos.dmg\n")
	require.NoError(t, verifyReleaseChecksum(dmgPath, "svpchain-agent-1.0.0-macos.dmg", sums))
}

func TestCheck_skipsWhenCurrentIsNewer(t *testing.T) {
	// No network: dev version skips before HTTP.
	info, err := Check(t.Context(), "0.1.0-dev", "", nil)
	require.NoError(t, err)
	require.Nil(t, info)
}
