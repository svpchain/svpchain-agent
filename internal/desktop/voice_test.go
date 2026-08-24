package desktop

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVoiceDictationAvailable(t *testing.T) {
	a := &App{}
	if runtime.GOOS == "darwin" {
		require.True(t, a.VoiceDictationAvailable())
		return
	}
	require.False(t, a.VoiceDictationAvailable())
}

func TestVoiceDictationStartRequiresReadyApp(t *testing.T) {
	a := &App{}
	err := a.VoiceDictationStart("en-US")
	require.Error(t, err)
}

func TestVoiceDictationStopIdle(t *testing.T) {
	a := &App{}
	require.NotPanics(t, a.VoiceDictationStop)
}
