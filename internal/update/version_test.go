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
	sums := []byte("abc123  svpchain-agent-1.0.2-macos.zip\n")
	got, err := expectedHashFromSums(sums, "svpchain-agent-1.0.2-macos.zip")
	require.NoError(t, err)
	require.Equal(t, "abc123", got)
}

func TestVerifyZipChecksum(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "svpchain-agent-1.0.0-macos.zip")
	require.NoError(t, os.WriteFile(zipPath, []byte("payload"), 0o644))

	got, err := hashFile(zipPath)
	require.NoError(t, err)
	sums := []byte(got + "  svpchain-agent-1.0.0-macos.zip\n")
	require.NoError(t, verifyZipChecksum(zipPath, "svpchain-agent-1.0.0-macos.zip", sums))
}

func TestCheck_skipsWhenCurrentIsNewer(t *testing.T) {
	// No network: dev version skips before HTTP.
	info, err := Check(t.Context(), "0.1.0-dev", "", nil)
	require.NoError(t, err)
	require.Nil(t, info)
}
