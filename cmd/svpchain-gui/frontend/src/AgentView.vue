<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NInput, NSelect, NScrollbar } from 'naive-ui'
import * as App from '../wailsjs/go/desktop/App'
import { EventsOn } from '../wailsjs/runtime/runtime'

const { t, tm } = useI18n()

import type { Entry } from './types'

type ChatLine = { role: 'user' | 'assistant' | 'step'; text: string; kind?: string }

const props = defineProps<{ entries: Entry[] }>()
const emit = defineEmits<{ status: [msg: string] }>()

const chainId = ref('')
const input = ref('')
const running = ref(false)
const runStatus = ref('')
const showToolSteps = ref(false)
const lines = ref<ChatLine[]>([])
const scrollRef = ref<InstanceType<typeof NScrollbar> | null>(null)
const imeComposing = ref(false)

type SessionOption = { id: string; title: string; chain_id: string; messages: number }
const sessions = ref<SessionOption[]>([])
const currentSessionId = ref('')

const promptChips = computed(() => {
  const raw = tm('assistant.chips')
  return Array.isArray(raw) ? (raw as string[]) : []
})

const isMultilineInput = computed(() => input.value.includes('\n'))

function applyChip(text: string) {
  if (running.value) return
  input.value = text
}

function report(msg: string) {
  runStatus.value = msg
  emit('status', msg)
}

function scrollToBottom() {
  nextTick(() => {
    scrollRef.value?.scrollTo({ top: 9_999_999 })
  })
}

watch(
  () => lines.value.length,
  () => scrollToBottom(),
)

let unsubs: Array<() => void> = []
let watchdog: ReturnType<typeof setTimeout> | null = null
// Index of the assistant bubble currently being streamed into, or -1 if none open.
// Any step event closes it, so each round's text lands in its own bubble.
let streamingIdx = -1

const stepKinds = new Set(['auth', 'tool', 'think', 'answer', 'error'])

function stepKindClass(kind: string) {
  return 'kind-' + (stepKinds.has(kind) ? kind : 'default')
}

function normalizeStep(raw: Record<string, unknown>): { kind: string; title: string; detail: string } {
  const title = String(raw.title ?? raw.Title ?? '')
  const detail = String(raw.detail ?? raw.Detail ?? '')
  const kind = String(raw.kind ?? raw.Kind ?? '')
  return { kind, title, detail }
}

function pushStep(raw: Record<string, unknown>) {
  const { kind, title, detail } = normalizeStep(raw)
  if (!title && !detail) return
  // A step interrupts streaming: close the current bubble so the next delta
  // (e.g. the answer after a tool call) opens a fresh one.
  streamingIdx = -1
  if (!showToolSteps.value && kind !== 'error') {
    return
  }
  const text = detail ? `${title}\n${detail}` : title
  lines.value.push({ role: 'step', text, kind })
}

function clearWatchdog() {
  if (watchdog) {
    clearTimeout(watchdog)
    watchdog = null
  }
}

function armWatchdog() {
  clearWatchdog()
  watchdog = setTimeout(() => {
    if (!running.value) return
    running.value = false
    const msg = t('assistant.status.timeout')
    lines.value.push({ role: 'step', text: msg, kind: 'error' })
    report(msg)
    App.AgentCancel()
  }, 180_000)
}

function availableChainIDs(): Set<string> {
  return new Set(props.entries.map((e) => e.ChainID).filter(Boolean))
}

/** Pick a chain the imported keys still support; never stick to a deleted key. */
function resolveChainID(preferred: string): string {
  const available = availableChainIDs()
  const pick = preferred.trim()
  if (pick && available.has(pick)) return pick
  return props.entries[0]?.ChainID || ''
}

async function loadSettings() {
  try {
    const s = (await App.AgentGetSettings()) as { chain_id?: string; show_tool_steps?: boolean }
    showToolSteps.value = !!s.show_tool_steps
    // Keep the user's current selection when it still has a key. Only fall back
    // to prefs (or the first imported key) when the UI has no valid chain yet —
    // calling this from send() must NOT reset a deliberate chain switch.
    chainId.value = resolveChainID(chainId.value || s.chain_id || '')
  } catch {
    chainId.value = resolveChainID(chainId.value)
  }
}

async function persistChainID(id: string) {
  const next = resolveChainID(id)
  chainId.value = next
  if (!next) return
  try {
    const s = (await App.AgentGetSettings()) as Record<string, unknown>
    await App.AgentSetSettings({ ...s, chain_id: next })
  } catch {
    /* best-effort sync with Settings → Basic default chain */
  }
}

watch(
  () => props.entries.map((e) => e.ChainID).join('\0'),
  () => {
    chainId.value = resolveChainID(chainId.value)
  },
)

async function refreshSessions() {
  try {
    const rows = ((await App.AgentSessions()) || []) as SessionOption[]
    sessions.value = rows
    currentSessionId.value = (await App.AgentCurrentSessionID()) || ''
  } catch {
    sessions.value = []
  }
}

async function loadTranscript(id: string) {
  try {
    const rows = ((await App.AgentTranscript(id)) || []) as Array<{ role: string; text: string }>
    lines.value = rows
      .filter((r) => r.role === 'user' || r.role === 'assistant')
      .map((r) => ({ role: r.role as 'user' | 'assistant', text: r.text }))
    streamingIdx = -1
    scrollToBottom()
  } catch {
    /* history bindings unavailable */
  }
}

async function switchSession(id: string) {
  if (running.value || !id || id === currentSessionId.value) return
  try {
    await App.AgentSwitchSession(id)
    currentSessionId.value = id
    const sess = sessions.value.find((s) => s.id === id)
    if (sess?.chain_id) {
      await persistChainID(sess.chain_id)
    }
    await loadTranscript(id)
  } catch (err) {
    report(String(err))
  }
}

async function newSession() {
  if (running.value) return
  try {
    await App.AgentNewSession(chainId.value)
    lines.value = []
    streamingIdx = -1
    await refreshSessions()
  } catch (err) {
    report(String(err))
  }
}

async function deleteSession() {
  if (running.value || !currentSessionId.value) return
  try {
    await App.AgentDeleteSession(currentSessionId.value)
    lines.value = []
    streamingIdx = -1
    await refreshSessions()
  } catch (err) {
    report(String(err))
  }
}

async function send() {
  const msg = input.value.trim()
  if (!msg) {
    report(t('assistant.status.enterMessage'))
    return
  }
  if (!chainId.value) {
    report(t('assistant.status.selectChain'))
    return
  }
  if (running.value) return

  await loadSettings()

  lines.value.push({ role: 'user', text: msg })
  input.value = ''
  running.value = true
  streamingIdx = -1
  armWatchdog()
  report(t('assistant.status.running'))

  try {
    await App.AgentSend(chainId.value, msg)
  } catch (err) {
    clearWatchdog()
    running.value = false
    const text = String(err)
    lines.value.push({ role: 'step', text, kind: 'error' })
    report(text)
  }
}

function onStep(raw: Record<string, unknown>) {
  pushStep(raw)
  const { title } = normalizeStep(raw)
  if (title) runStatus.value = title
}

function onDelta(e: { text?: string }) {
  const text = e?.text || ''
  if (!text) return
  if (streamingIdx < 0) {
    lines.value.push({ role: 'assistant', text: '' })
    streamingIdx = lines.value.length - 1
  }
  lines.value[streamingIdx].text += text
  scrollToBottom()
}

function onDone(e: { answer?: string }) {
  clearWatchdog()
  running.value = false
  if (streamingIdx >= 0) {
    // Finalize the streamed bubble with the authoritative answer.
    if (e.answer) lines.value[streamingIdx].text = e.answer
    streamingIdx = -1
  } else if (e.answer) {
    lines.value.push({ role: 'assistant', text: e.answer })
  }
  report(t('assistant.status.done'))
  refreshSessions()
}

function onError(e: { error?: string }) {
  clearWatchdog()
  running.value = false
  streamingIdx = -1
  const err = e.error || t('assistant.status.failed')
  lines.value.push({ role: 'step', text: err, kind: 'error' })
  report(err)
  refreshSessions()
}

function cancel() {
  clearWatchdog()
  App.AgentCancel()
  running.value = false
  streamingIdx = -1
  lines.value.push({ role: 'step', text: t('assistant.status.cancelled'), kind: 'error' })
  report(t('assistant.status.cancelled'))
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey) {
    if (e.isComposing || imeComposing.value || e.keyCode === 229) {
      return
    }
    e.preventDefault()
    send()
  }
}

onMounted(async () => {
  await loadSettings()
  await refreshSessions()
  if (currentSessionId.value) {
    await loadTranscript(currentSessionId.value)
  }
  unsubs = [
    EventsOn('agent:step', onStep),
    EventsOn('agent:delta', onDelta),
    EventsOn('agent:done', onDone),
    EventsOn('agent:error', onError),
  ]
})

onUnmounted(() => {
  clearWatchdog()
  unsubs.forEach((u) => u())
  unsubs = []
})
</script>

<template>
  <div class="assistant-pane">
    <header class="chat-header">
      <n-select
        :value="chainId || null"
        data-tour="assistant-chain"
        :placeholder="t('assistant.ph.chainId')"
        :options="entries.map((e) => ({ label: e.ChainID, value: e.ChainID }))"
        size="small"
        class="chain-select"
        :disabled="running"
        @update:value="persistChainID"
      />
      <div class="session-controls">
        <n-select
          v-if="sessions.length > 0"
          :value="currentSessionId || null"
          :placeholder="t('assistant.session.placeholder')"
          :options="sessions.map((s) => ({ label: s.title || t('assistant.session.untitled'), value: s.id }))"
          size="small"
          class="session-select"
          :disabled="running"
          @update:value="switchSession"
        />
        <n-button size="small" quaternary :disabled="running" @click="newSession">
          {{ t('assistant.btn.newChat') }}
        </n-button>
        <n-button
          v-if="currentSessionId"
          size="small"
          quaternary
          :disabled="running"
          :aria-label="t('assistant.btn.deleteChat')"
          :title="t('assistant.btn.deleteChat')"
          @click="deleteSession"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" class="trash-icon">
            <path d="M4 7h16M10 11v6M14 11v6M6 7l1 13h10l1-13M9 7V4h6v3" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
        </n-button>
      </div>
      <span v-if="running" class="running-badge">
        <span class="running-pulse" />
        {{ runStatus || t('assistant.status.running') }}
      </span>
    </header>

    <n-scrollbar ref="scrollRef" class="chat-log">
      <div class="chat-inner">
        <div v-if="lines.length === 0" class="welcome">
          <div class="welcome-icon">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
              <path d="M12 3c5.523 0 10 3.582 10 8 0 2.4-1.2 4.56-3.12 6.08L21 21l-4.28-1.42C15.56 20.18 13.82 20.5 12 20.5 6.477 20.5 2 16.918 2 11.5S6.477 3 12 3z" stroke-linejoin="round" />
            </svg>
          </div>
          <h2 class="welcome-title">SVPChain Agent</h2>
          <p class="welcome-hint">{{ t('assistant.hint') }}</p>
        </div>

        <div v-for="(line, i) in lines" :key="i" class="chat-line" :class="line.role">
          <template v-if="line.role === 'user'">
            <div class="bubble user-bubble">{{ line.text }}</div>
          </template>
          <template v-else-if="line.role === 'assistant'">
            <div class="assistant-block">
              <div class="assistant-avatar" aria-hidden="true">S</div>
              <div class="bubble assistant-bubble">{{ line.text }}</div>
            </div>
          </template>
          <template v-else>
            <div class="step-bubble" :class="stepKindClass(line.kind || '')">
              <span class="step-title">{{ line.text.split('\n')[0] }}</span>
              <pre v-if="line.text.includes('\n')" class="chat-detail">{{ line.text.slice(line.text.indexOf('\n') + 1) }}</pre>
            </div>
          </template>
        </div>
      </div>
    </n-scrollbar>

    <footer class="composer">
      <div v-if="lines.length === 0 && promptChips.length" class="composer-chips">
        <button
          v-for="(chip, idx) in promptChips"
          :key="'c-' + idx"
          type="button"
          class="prompt-chip"
          :disabled="running"
          @click="applyChip(chip)"
        >
          {{ chip }}
        </button>
      </div>
      <div
        class="composer-box"
        :class="{ 'composer-box--running': running, 'composer-box--multiline': isMultilineInput }"
      >
        <n-input
          v-model:value="input"
          type="textarea"
          :autosize="{ minRows: 1, maxRows: 6 }"
          :placeholder="t('assistant.ph.message')"
          :disabled="running"
          class="composer-input"
          :bordered="false"
          @compositionstart="imeComposing = true"
          @compositionend="imeComposing = false"
          @keydown="onKeydown"
        />
        <div class="composer-actions">
          <n-button
            v-if="running"
            size="small"
            quaternary
            class="cancel-btn"
            @click="cancel"
          >
            {{ t('assistant.btn.cancel') }}
          </n-button>
          <button
            type="button"
            class="send-btn"
            :disabled="running || !input.trim()"
            :aria-label="t('assistant.btn.send')"
            @click="send"
          >
            <svg viewBox="0 0 24 24" fill="currentColor">
              <path d="M3.4 20.4 22 12 3.4 3.6 3 10.8 17.6 12 3 13.2 3.4 20.4z" />
            </svg>
          </button>
        </div>
      </div>
    </footer>
  </div>
</template>

<style scoped>
.assistant-pane {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  background: var(--bg-base);
}

.chat-header {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 24px;
  border-bottom: 1px solid var(--border-subtle);
  background: var(--bg-base);
}

.chain-select {
  max-width: 200px;
  flex-shrink: 0;
}

.session-controls {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: 1;
  min-width: 0;
  justify-content: flex-end;
}

.session-select {
  max-width: 220px;
  min-width: 120px;
}

.trash-icon {
  width: 15px;
  height: 15px;
}

.running-badge {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  color: var(--accent);
  max-width: 50%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.running-pulse {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--accent);
  animation: pulse 1.4s ease-in-out infinite;
  flex-shrink: 0;
}

@keyframes pulse {
  0%,
  100% {
    opacity: 1;
    transform: scale(1);
  }
  50% {
    opacity: 0.4;
    transform: scale(0.85);
  }
}

.chat-log {
  flex: 1;
  min-height: 0;
}

.chat-log :deep(.n-scrollbar-rail) {
  display: none;
}

.chat-inner {
  max-width: 768px;
  margin: 0 auto;
  padding: 24px 24px 16px;
  min-height: 100%;
  box-sizing: border-box;
}

.welcome {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  text-align: center;
  min-height: 280px;
  padding: 40px 20px;
}

.welcome-icon {
  width: 48px;
  height: 48px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 14px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  color: var(--accent);
  margin-bottom: 16px;
}

.welcome-icon svg {
  width: 24px;
  height: 24px;
}

.welcome-title {
  margin: 0 0 8px;
  font-size: 22px;
  font-weight: 600;
  letter-spacing: -0.02em;
  color: var(--text-primary);
}

.welcome-hint {
  margin: 0;
  max-width: 420px;
  font-size: 14px;
  line-height: 1.6;
  color: var(--text-secondary);
}

.chat-line {
  margin-bottom: 20px;
}

.chat-line.user {
  display: flex;
  justify-content: flex-end;
}

.user-bubble {
  max-width: 85%;
  padding: 12px 16px;
  border-radius: 20px 20px 4px 20px;
  background: var(--bg-user-bubble);
  color: var(--text-primary);
  font-size: 14px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-word;
  border: 1px solid var(--border-subtle);
}

.assistant-block {
  display: flex;
  gap: 12px;
  align-items: flex-start;
}

.assistant-avatar {
  flex-shrink: 0;
  width: 28px;
  height: 28px;
  border-radius: 8px;
  background: linear-gradient(135deg, var(--accent) 0%, #0d8c6d 100%);
  color: #fff;
  font-size: 12px;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
}

.assistant-bubble {
  flex: 1;
  min-width: 0;
  font-size: 14px;
  line-height: 1.7;
  color: var(--text-primary);
  white-space: pre-wrap;
  word-break: break-word;
}

.step-bubble {
  margin-left: 40px;
  padding: 8px 12px;
  border-left: 2px solid transparent;
  border-radius: 0 var(--radius-sm) var(--radius-sm) 0;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-left-width: 2px;
}

.step-title {
  font-size: 12px;
  font-weight: 500;
  line-height: 1.5;
  color: var(--text-secondary);
}

.step-bubble.kind-auth {
  border-left-color: #9b7bff;
  background: rgba(155, 123, 255, 0.08);
}

.step-bubble.kind-tool {
  border-left-color: #5b9cf5;
  background: rgba(91, 156, 245, 0.08);
}

.step-bubble.kind-think {
  border-left-color: #d4a24a;
  background: rgba(212, 162, 74, 0.08);
}

.step-bubble.kind-answer {
  border-left-color: var(--accent);
  background: var(--accent-muted);
}

.step-bubble.kind-error {
  border-left-color: #e55353;
  background: rgba(229, 83, 83, 0.1);
}

.step-bubble.kind-default {
  border-left-color: var(--text-muted);
}

.chat-detail {
  margin: 6px 0 0;
  padding: 8px 10px;
  background: var(--bg-base);
  border-radius: var(--radius-sm);
  font-size: 11px;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  color: var(--text-secondary);
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 120px;
  overflow: auto;
}

.composer-chips {
  max-width: 768px;
  margin: 0 auto 10px;
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 8px;
}

.prompt-chip {
  padding: 8px 14px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-full);
  background: var(--bg-chip);
  color: var(--text-secondary);
  font-family: inherit;
  font-size: 12px;
  line-height: 1.4;
  cursor: pointer;
  transition: background 0.15s ease, border-color 0.15s ease, color 0.15s ease;
}

.prompt-chip:hover:not(:disabled) {
  background: var(--bg-chip-hover);
  border-color: var(--border-default);
  color: var(--text-primary);
}

.prompt-chip:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.composer {
  flex-shrink: 0;
  padding: 12px 24px 16px;
  background: linear-gradient(to top, var(--bg-base) 70%, transparent);
}

.composer-box {
  max-width: 768px;
  margin: 0 auto;
  display: flex;
  align-items: center;
  gap: 4px;
  min-height: 44px;
  padding: 4px 6px 4px 14px;
  background: var(--bg-input);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-xl);
  box-shadow: none;
  transition: border-color 0.15s ease;
}

.composer-box:focus-within {
  border-color: var(--accent);
  box-shadow: none;
}

.composer-box--running {
  opacity: 0.85;
}

.composer-box--multiline {
  align-items: flex-end;
  padding-top: 8px;
  padding-bottom: 8px;
}

.composer-box--multiline .composer-input :deep(.n-input__textarea-el) {
  line-height: 1.5;
  min-height: unset !important;
}

.composer-box--multiline .composer-input :deep(.n-input__placeholder) {
  line-height: 1.5;
  min-height: unset;
  align-items: flex-start;
}

.composer-input {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
}

.composer-input :deep(.n-input) {
  background: transparent !important;
  width: 100%;
}

.composer-input :deep(.n-input-wrapper) {
  background: transparent !important;
  box-shadow: none !important;
  padding: 0 !important;
  align-items: center !important;
}

.composer-input :deep(.n-input__border),
.composer-input :deep(.n-input__state-border) {
  display: none !important;
}

.composer-input :deep(.n-input__textarea-el) {
  font-size: 14px;
  line-height: 32px;
  min-height: 32px !important;
  text-align: left;
  padding: 0 !important;
  margin: 0;
  background: transparent !important;
  box-shadow: none !important;
  resize: none;
  vertical-align: middle;
}

.composer-input :deep(.n-input__placeholder) {
  text-align: left;
  line-height: 32px;
  padding: 0 !important;
  top: 0 !important;
  transform: none !important;
  display: flex;
  align-items: center;
  min-height: 32px;
}

.composer-actions {
  display: flex;
  align-items: center;
  gap: 2px;
  flex-shrink: 0;
  margin-left: 2px;
}

.cancel-btn {
  font-size: 12px;
  padding: 0 6px !important;
}

.send-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  margin: 0;
  border: none;
  border-radius: 50%;
  background: var(--accent);
  color: #fff;
  cursor: pointer;
  transition: background 0.15s ease, opacity 0.15s ease, transform 0.1s ease;
  flex-shrink: 0;
  box-shadow: none;
}

.send-btn svg {
  width: 16px;
  height: 16px;
}

.send-btn:hover:not(:disabled) {
  background: var(--accent-hover);
}

.send-btn:active:not(:disabled) {
  transform: scale(0.94);
}

.send-btn:disabled {
  opacity: 0.35;
  cursor: not-allowed;
}
</style>
