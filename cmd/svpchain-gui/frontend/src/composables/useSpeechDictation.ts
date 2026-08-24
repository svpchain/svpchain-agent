import {onMounted, onUnmounted, ref, type Ref, unref} from 'vue'
import * as App from '../../wailsjs/go/desktop/App'
import {EventsOn} from '../../wailsjs/runtime/runtime'

type SpeechRecognitionCtor = new () => SpeechRecognitionLike

type SpeechRecognitionLike = {
  lang: string
  continuous: boolean
  interimResults: boolean
  onresult: ((ev: SpeechRecognitionEventLike) => void) | null
  onerror: ((ev: { error: string }) => void) | null
  onend: (() => void) | null
  start: () => void
  stop: () => void
  abort: () => void
}

type SpeechRecognitionEventLike = {
  resultIndex: number
  results: ArrayLike<{
    isFinal: boolean
    0: {transcript: string}
  }>
}

function speechCtor(): SpeechRecognitionCtor | null {
  const w = window as unknown as {
    SpeechRecognition?: SpeechRecognitionCtor
    webkitSpeechRecognition?: SpeechRecognitionCtor
  }
  return w.SpeechRecognition || w.webkitSpeechRecognition || null
}

function joinParts(base: string, extra: string): string {
  const a = base.trimEnd()
  const b = extra.replace(/\s+/g, ' ').trim()
  if (!b) return a
  if (!a) return b
  return a.endsWith('\n') ? a + b : a + ' ' + b
}

function likelyMac(): boolean {
  const nav = navigator as Navigator & {userAgentData?: {platform?: string}}
  const plat = nav.userAgentData?.platform || navigator.platform || ''
  return /Mac/i.test(plat) || /Mac/i.test(navigator.userAgent)
}

function eventText(data: unknown, key: 'text' | 'error'): string {
  if (data && typeof data === 'object' && key in data) {
    return String((data as Record<string, unknown>)[key] || '')
  }
  if (typeof data === 'string') return data
  return ''
}

export function speechRecognitionSupported(): boolean {
  return speechCtor() !== null || likelyMac()
}

type VoiceCopy = {
  unsupported: string
  denied: string
  unavailable: string
  listening: string
  filled: string
  empty: string
}

export function useSpeechDictation(
    input: Ref<string>,
    lang: Ref<string> | (() => string),
    onStatus: (msg: string) => void,
    copy: () => VoiceCopy,
) {
  const listening = ref(false)
  const Ctor = speechCtor()
  const supported = ref(Ctor !== null || likelyMac())

  let native = likelyMac()
  let rec: SpeechRecognitionLike | null = null
  let wantListen = false
  let base = ''
  let heard = false
  const unsubs: Array<() => void> = []

  function currentLang(): string {
    const v = typeof lang === 'function' ? lang() : unref(lang)
    return v.startsWith('zh') ? 'zh-CN' : 'en-US'
  }

  function applyWebTranscript(finalChunk: string, interim: string) {
    if (finalChunk) {
      heard = true
      base = joinParts(base, finalChunk)
    }
    input.value = joinParts(base, interim)
  }

  function applyNative(full: string, isFinal: boolean) {
    const text = full.replace(/\s+/g, ' ').trim()
    if (!text) return
    heard = true
    if (isFinal) {
      base = joinParts(base, text)
      input.value = base
      return
    }
    input.value = joinParts(base, text)
  }

  function handleResult(ev: SpeechRecognitionEventLike) {
    let finals = ''
    let interim = ''
    for (let i = ev.resultIndex; i < ev.results.length; i++) {
      const row = ev.results[i]
      const text = row[0]?.transcript || ''
      if (row.isFinal) finals += text
      else interim += text
    }
    applyWebTranscript(finals, interim)
  }

  function startRec() {
    if (!Ctor) return
    rec = new Ctor()
    rec.continuous = true
    rec.interimResults = true
    rec.lang = currentLang()
    rec.onresult = handleResult
    rec.onerror = (ev) => {
      const err = ev.error
      if (err === 'aborted' || err === 'no-speech') return
      wantListen = false
      listening.value = false
      if (err === 'not-allowed' || err === 'service-not-allowed') {
        onStatus(copy().denied)
        return
      }
      onStatus(copy().unavailable)
    }
    rec.onend = () => {
      if (wantListen) {
        try {
          rec?.start()
        } catch {
          listening.value = false
          wantListen = false
        }
        return
      }
      listening.value = false
    }
    rec.start()
  }

  async function startNative() {
    wantListen = true
    listening.value = true
    onStatus(copy().listening)
    try {
      await App.VoiceDictationStart(currentLang())
    } catch {
      wantListen = false
      listening.value = false
      onStatus(copy().unavailable)
    }
  }

  async function start() {
    if (!supported.value) {
      onStatus(copy().unsupported)
      return
    }
    if (listening.value) return
    heard = false
    base = input.value
    if (native) {
      await startNative()
      return
    }
    wantListen = true
    listening.value = true
    onStatus(copy().listening)
    try {
      startRec()
    } catch {
      wantListen = false
      listening.value = false
      onStatus(copy().unavailable)
    }
  }

  function stop(silent = false) {
    const wasListening = listening.value || wantListen
    wantListen = false
    listening.value = false
    if (native) {
      void App.VoiceDictationStop()
    } else {
      try {
        rec?.stop()
      } catch {
        /* already stopped */
      }
      rec = null
    }
    if (silent || !wasListening) return
    if (heard || input.value.trim()) onStatus(copy().filled)
    else onStatus(copy().empty)
  }

  function onNativePartial(data: unknown) {
    if (!wantListen && !listening.value) return
    applyNative(eventText(data, 'text'), false)
  }

  function onNativeFinal(data: unknown) {
    if (!wantListen && !listening.value) return
    applyNative(eventText(data, 'text'), true)
  }

  function onNativeError(data: unknown) {
    const msg = eventText(data, 'error')
    const was = wantListen || listening.value
    wantListen = false
    listening.value = false
    void App.VoiceDictationStop()
    if (!was) return
    if (msg === 'denied' || /denied|not authorized|authorization|permission/i.test(msg)) {
      onStatus(copy().denied)
      return
    }
    onStatus(copy().unavailable)
  }

  async function toggle() {
    if (listening.value) stop()
    else await start()
  }

  onMounted(async () => {
    try {
      native = await App.VoiceDictationAvailable()
    } catch {
      native = false
    }
    if (native || Ctor) supported.value = true
    else supported.value = false
    unsubs.push(
        EventsOn('voice:partial', onNativePartial),
        EventsOn('voice:final', onNativeFinal),
        EventsOn('voice:error', onNativeError),
    )
  })

  onUnmounted(() => {
    stop(true)
    unsubs.forEach((u) => u())
    unsubs.length = 0
  })

  return {supported, listening, start, stop, toggle}
}
