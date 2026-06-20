<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NInput, NSelect, NScrollbar, NTag, NText } from 'naive-ui'
import * as App from '../wailsjs/go/desktop/App'
import { EventsOn } from '../wailsjs/runtime/runtime'

const { t } = useI18n()

type Entry = { ChainID: string; Owner: string; EVMAddr: string }
type ChatLine = { role: 'user' | 'assistant' | 'step'; text: string; kind?: string }

const props = defineProps<{ entries: Entry[] }>()
const emit = defineEmits<{ status: [msg: string] }>()

const chainId = ref('')
const input = ref('')
const running = ref(false)
const runStatus = ref('')
const lines = ref<ChatLine[]>([])
const scrollRef = ref<InstanceType<typeof NScrollbar> | null>(null)

// Update both the in-tab execution-status line and the parent's global status bar.
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

function stepTagType(kind: string) {
  switch (kind) {
    case 'error':
      return 'error'
    case 'answer':
      return 'success'
    case 'tool':
      return 'info'
    default:
      return 'default'
  }
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

async function loadSettings() {
  try {
    const s = (await App.AgentGetSettings()) as { chain_id?: string }
    const id = s.chain_id || ''
    if (id) chainId.value = id
    else if (props.entries.length > 0) chainId.value = props.entries[0].ChainID
  } catch {
    if (props.entries.length > 0) chainId.value = props.entries[0].ChainID
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

  lines.value.push({ role: 'user', text: msg })
  input.value = ''
  running.value = true
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

function onDone(e: { answer?: string }) {
  clearWatchdog()
  running.value = false
  if (e.answer) {
    lines.value.push({ role: 'assistant', text: e.answer })
  }
  report(t('assistant.status.done'))
}

function onError(e: { error?: string }) {
  clearWatchdog()
  running.value = false
  const err = e.error || t('assistant.status.failed')
  lines.value.push({ role: 'step', text: err, kind: 'error' })
  report(err)
}

function cancel() {
  clearWatchdog()
  App.AgentCancel()
  running.value = false
  lines.value.push({ role: 'step', text: t('assistant.status.cancelled'), kind: 'error' })
  report(t('assistant.status.cancelled'))
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    send()
  }
}

onMounted(async () => {
  await loadSettings()
  unsubs = [
    EventsOn('agent:step', onStep),
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
    <n-select
      v-model:value="chainId"
      :placeholder="t('assistant.ph.chainId')"
      :options="entries.map((e) => ({ label: e.ChainID, value: e.ChainID }))"
      size="small"
      class="chain-select"
    />

    <n-scrollbar ref="scrollRef" class="chat-log">
      <div v-for="(line, i) in lines" :key="i" class="chat-line" :class="line.role">
        <template v-if="line.role === 'user'">
          <n-text strong>{{ t('assistant.you') }}</n-text>
          <pre class="chat-text">{{ line.text }}</pre>
        </template>
        <template v-else-if="line.role === 'assistant'">
          <n-text type="success" strong>{{ t('assistant.reply') }}</n-text>
          <pre class="chat-text">{{ line.text }}</pre>
        </template>
        <template v-else>
          <n-tag :type="stepTagType(line.kind || '')" size="small" :bordered="false">
            {{ line.text.split('\n')[0] }}
          </n-tag>
          <pre v-if="line.text.includes('\n')" class="chat-detail">{{ line.text.slice(line.text.indexOf('\n') + 1) }}</pre>
        </template>
      </div>
      <n-text v-if="lines.length === 0" depth="3" class="empty-hint">{{ t('assistant.hint') }}</n-text>
    </n-scrollbar>

    <div class="input-row">
      <n-input
        v-model:value="input"
        type="textarea"
        :autosize="{ minRows: 3, maxRows: 6 }"
        :placeholder="t('assistant.ph.message')"
        :disabled="running"
        class="input-box"
        @keydown="onKeydown"
      />
      <div class="input-buttons">
        <n-button v-if="running" type="warning" @click="cancel">{{ t('assistant.btn.cancel') }}</n-button>
        <n-button type="primary" :loading="running" :disabled="running" @click="send">
          {{ t('assistant.btn.send') }}
        </n-button>
      </div>
    </div>

    <div class="status-line">
      <span class="status-label">{{ t('assistant.status.label') }}:</span>
      <span class="status-text" :class="{ running }">{{ runStatus || t('assistant.status.idle') }}</span>
    </div>
  </div>
</template>

<style scoped>
.assistant-pane {
  display: flex;
  flex-direction: column;
  gap: 10px;
  height: 100%;
  min-height: 0;
}

.chain-select {
  max-width: 280px;
  flex-shrink: 0;
}

.chat-log {
  flex: 1;
  min-height: 0;
  border: 1px solid #ececec;
  border-radius: 12px;
  padding: 10px;
  background: #fafafa;
}

/* Keep scrolling functional but hide the scrollbar in the assistant chat area. */
.chat-log :deep(.n-scrollbar-rail) {
  display: none;
}

.chat-line {
  margin-bottom: 10px;
}

/* Highlight the user's own input as a right-aligned accent bubble so it stands
   out from assistant replies and step logs. */
.chat-line.user {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
}

.chat-line.user .chat-text {
  margin: 4px 0 0;
  align-self: flex-end;
  max-width: 85%;
  background: #18a058;
  color: #fff;
  padding: 8px 12px;
  border-radius: 10px 10px 2px 10px;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: inherit;
  font-size: 13px;
  line-height: 1.5;
}

.chat-line.assistant .chat-text {
  margin: 4px 0 0;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: inherit;
  font-size: 13px;
  line-height: 1.5;
}

.chat-detail {
  margin: 4px 0 0;
  padding: 6px 8px;
  background: #f0f0f0;
  border-radius: 4px;
  font-size: 11px;
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 120px;
  overflow: auto;
}

.empty-hint {
  font-size: 12px;
}

.input-row {
  display: flex;
  align-items: flex-end;
  gap: 10px;
  flex-shrink: 0;
}

.input-box {
  flex: 1;
}

.input-buttons {
  display: flex;
  flex-direction: column;
  gap: 8px;
  flex-shrink: 0;
}

.status-line {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: #888;
  border-top: 1px solid #ececec;
  padding-top: 6px;
}

.status-label {
  color: #aaa;
  flex-shrink: 0;
}

.status-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.status-text.running {
  color: #18a058;
}
</style>
