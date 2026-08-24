//go:build !darwin

package desktop

import (
	"context"
	"fmt"
)

func installWebViewMicGrant() {}

func setVoiceCtx(context.Context) {}

// VoiceDictationAvailable is false off macOS; the UI uses the Web Speech API.
func (a *App) VoiceDictationAvailable() bool { return false }

// VoiceDictationStart is a stub on non-macOS platforms.
func (a *App) VoiceDictationStart(string) error {
	return fmt.Errorf("native dictation is only available on macOS")
}

// VoiceDictationStop is a stub on non-macOS platforms.
func (a *App) VoiceDictationStop() {}
