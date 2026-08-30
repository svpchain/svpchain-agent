#!/usr/bin/env bash
# Idempotently add macOS microphone + speech-recognition usage strings to
# Info.plist files (Wails' generated bundle does not include them).
set -euo pipefail

if [[ $# -lt 1 ]]; then
	echo "usage: $0 <Info.plist> [Info.plist...]" >&2
	exit 1
fi

mic='Voice input transcribes your command into the assistant text box. Recognition uses the system speech service; SVPChain does not upload the recording.'
speech='Voice input uses Speech Recognition to transcribe your command into the assistant text box. Recognition uses the system speech service; SVPChain does not upload the recording.'

for path in "$@"; do
	[[ -f "$path" ]] || continue
	# Wails' build/darwin/Info.plist is a Go template; only patch plist files
	# after Wails has expanded that template into the app bundle.
	if ! plutil -lint "$path" >/dev/null 2>&1; then
		echo "skipped non-plist template $path"
		continue
	fi
	/usr/libexec/PlistBuddy -c "Add :NSMicrophoneUsageDescription string $mic" "$path" 2>/dev/null \
		|| /usr/libexec/PlistBuddy -c "Set :NSMicrophoneUsageDescription $mic" "$path"
	/usr/libexec/PlistBuddy -c "Add :NSSpeechRecognitionUsageDescription string $speech" "$path" 2>/dev/null \
		|| /usr/libexec/PlistBuddy -c "Set :NSSpeechRecognitionUsageDescription $speech" "$path"
	echo "patched $path"
done
