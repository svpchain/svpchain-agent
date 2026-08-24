//go:build darwin

package desktop

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework Speech -framework AVFoundation -Wl,-sectcreate,__TEXT,__info_plist,${SRCDIR}/macos_privacy.plist
#include "voice_darwin.h"
#include <stdlib.h>
*/
import "C"

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"unsafe"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

var (
	voiceMu  sync.Mutex
	voiceCtx context.Context
)

func installWebViewMicGrant() {
	C.SVPInstallWebViewMicGrant()
}

func setVoiceCtx(ctx context.Context) {
	voiceMu.Lock()
	voiceCtx = ctx
	voiceMu.Unlock()
}

// VoiceDictationAvailable reports that macOS uses native Speech.framework
// instead of the WebView Speech API (WKWebView rejects getUserMedia).
func (a *App) VoiceDictationAvailable() bool { return true }

// VoiceDictationStart begins system speech recognition (bypasses the WebView).
func (a *App) VoiceDictationStart(lang string) error {
	if a.ctx == nil {
		return fmt.Errorf("app not ready")
	}
	lang = strings.TrimSpace(lang)
	if lang == "" {
		lang = "en-US"
	}
	cLang := C.CString(lang)
	defer C.free(unsafe.Pointer(cLang))
	C.SVPInstallWebViewMicGrant()
	C.SVPVoiceStart(cLang)
	return nil
}

// VoiceDictationStop ends the native recognition session.
func (a *App) VoiceDictationStop() {
	C.SVPVoiceStop()
}

func emitVoice(event, key, value string) {
	voiceMu.Lock()
	ctx := voiceCtx
	voiceMu.Unlock()
	if ctx == nil {
		return
	}
	wruntime.EventsEmit(ctx, event, map[string]string{key: value})
}

func goEmitVoice(event, key, value string) {
	// Copy is already a Go string; never call Wails from inside a cgo
	// callback on the AppKit main thread.
	go emitVoice(event, key, value)
}

//export goVoicePartial
func goVoicePartial(text *C.char) {
	goEmitVoice("voice:partial", "text", C.GoString(text))
}

//export goVoiceFinal
func goVoiceFinal(text *C.char) {
	goEmitVoice("voice:final", "text", C.GoString(text))
}

//export goVoiceError
func goVoiceError(text *C.char) {
	msg := C.GoString(text)
	if msg == "" {
		msg = "denied"
	}
	goEmitVoice("voice:error", "error", msg)
}
