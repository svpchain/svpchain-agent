#!/usr/bin/env bash
# Idempotently add macOS microphone + speech-recognition usage strings to
# Info.plist files (Wails' generated bundle does not include them).
set -euo pipefail

if [[ $# -lt 1 ]]; then
	echo "usage: $0 <Info.plist> [Info.plist...]" >&2
	exit 1
fi

python3 - "$@" <<'PY'
import plistlib
import sys
from pathlib import Path

mic = (
    "Voice input transcribes your command into the assistant text box. "
    "Recognition uses the system speech service; SVPChain does not upload the recording."
)
speech = (
    "Voice input uses Speech Recognition to transcribe your command into the "
    "assistant text box. Recognition uses the system speech service; SVPChain "
    "does not upload the recording."
)

for raw in sys.argv[1:]:
    path = Path(raw)
    if not path.is_file():
        continue
    with path.open("rb") as f:
        data = plistlib.load(f)
    data["NSMicrophoneUsageDescription"] = mic
    data["NSSpeechRecognitionUsageDescription"] = speech
    with path.open("wb") as f:
        plistlib.dump(data, f, sort_keys=False)
    print(f"patched {path}")
PY
