<script setup lang="ts">
import {computed, onMounted, ref, watch} from 'vue'
import {useI18n} from 'vue-i18n'
import {NButton, NEmpty, NSelect, NSpin, NTag, NText} from 'naive-ui'
import * as App from '../../wailsjs/go/desktop/App'
import type {AgentRun, AgentRunOutcome, AgentRunStep} from '../types'

const props = defineProps<{active?: boolean}>()
const emit = defineEmits<{status: [msg: string]}>()

const {t, locale} = useI18n()

const loading = ref(false)
const logPath = ref('')
const loggingOn = ref(true)
const runs = ref<AgentRun[]>([])
const selectedId = ref('')
const outcomeFilter = ref<string>('all')
const expanded = ref<Record<string, boolean>>({})

const outcomes: AgentRunOutcome[] = ['success', 'failed', 'stopped', 'rejected', 'cancelled']

const outcomeOptions = computed(() => [
  {label: t('runs.filter.all'), value: 'all'},
  ...outcomes.map((o) => ({label: t(`runs.outcome.${o}`), value: o})),
])

const filtered = computed(() => {
  if (outcomeFilter.value === 'all') return runs.value
  return runs.value.filter((r) => r.outcome === outcomeFilter.value)
})

const selected = computed(() => filtered.value.find((r) => r.run_id === selectedId.value) || null)

const timeline = computed(() => {
  const run = selected.value
  if (!run) return []
  return (run.steps || []).filter((s) => !(s.kind === 'tool' && !s.tool))
})

function setStatus(msg: string) {
  emit('status', msg)
}

function outcomeType(outcome: string): 'success' | 'error' | 'warning' | 'info' | 'default' {
  if (outcome === 'success') return 'success'
  if (outcome === 'failed') return 'error'
  if (outcome === 'cancelled') return 'default'
  if (outcome === 'rejected' || outcome === 'stopped') return 'warning'
  return 'info'
}

function formatTime(iso?: string) {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toLocaleString(locale.value === 'zh' ? 'zh-CN' : 'en-US', {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}

function durationMs(run: AgentRun) {
  if (!run.started_at || !run.finished_at) return 0
  const a = new Date(run.started_at).getTime()
  const b = new Date(run.finished_at).getTime()
  if (Number.isNaN(a) || Number.isNaN(b) || b < a) return 0
  return b - a
}

function formatDuration(ms: number) {
  if (ms <= 0) return '—'
  if (ms < 1000) return `${ms} ms`
  return `${(ms / 1000).toFixed(1)} s`
}

function preview(text: string, n = 72) {
  const s = (text || '').replace(/\s+/g, ' ').trim()
  if (s.length <= n) return s
  return s.slice(0, n) + '…'
}

function stepKey(step: AgentRunStep, idx: number) {
  return `${step.at}-${step.kind}-${step.tool || ''}-${idx}`
}

function toggleExpand(key: string) {
  expanded.value = {...expanded.value, [key]: !expanded.value[key]}
}

function copy(text: string, statusKey: string) {
  if (!text) return
  App.CopyText(text)
  setStatus(t(statusKey))
}

function selectRun(id: string) {
  selectedId.value = id
  expanded.value = {}
}

function stepTitle(step: AgentRunStep) {
  if (step.tool) return step.tool
  if (step.detail) return preview(step.detail.split('\n')[0], 80)
  return step.kind || '—'
}

function outcomeLabel(outcome: string) {
  const key = `runs.outcome.${outcome}`
  const translated = t(key)
  return translated === key ? outcome : translated
}

function stepKindLabel(kind: string) {
  const key = `runs.step.${kind}`
  const translated = t(key)
  return translated === key ? kind : translated
}

watch(filtered, (list) => {
  if (!list.length) {
    selectedId.value = ''
    return
  }
  if (!list.some((r) => r.run_id === selectedId.value)) {
    selectedId.value = list[0].run_id
  }
})

async function refresh() {
  loading.value = true
  setStatus(t('runs.status.loading'))
  try {
    logPath.value = (await App.AgentRunLogPath()) || ''
    try {
      const s = (await App.AgentGetSettings()) as {agent_run_log_disabled?: boolean}
      loggingOn.value = !s.agent_run_log_disabled
    } catch {
      loggingOn.value = true
    }
    const list = ((await App.AgentRecentRuns(100)) as unknown as AgentRun[]) || []
    runs.value = list
    if (!selectedId.value && list.length) selectedId.value = list[0].run_id
    setStatus(t('runs.status.count', {n: list.length}))
  } catch (err) {
    runs.value = []
    setStatus(t('runs.status.loadFailed', {err: String(err)}))
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  if (props.active) refresh()
})

watch(
    () => props.active,
    (on) => {
      if (on) refresh()
    },
)

defineExpose({refresh})
</script>

<template>
  <div class="runs-pane">
    <div class="runs-toolbar">
      <n-select
          v-model:value="outcomeFilter"
          class="filter-select"
          size="small"
          :options="outcomeOptions"
          :consistent-menu-width="false"
      />
      <n-button size="small" :loading="loading" @click="refresh">{{ t('btn.refresh') }}</n-button>
      <n-button
          v-if="logPath"
          size="small"
          quaternary
          :title="logPath"
          @click="copy(logPath, 'runs.status.pathCopied')"
      >
        {{ t('runs.copyPath') }}
      </n-button>
    </div>
    <n-text v-if="!loggingOn" depth="3" class="logging-hint">{{ t('runs.loggingOff') }}</n-text>

    <n-spin :show="loading" class="runs-spin">
      <div v-if="filtered.length" class="runs-split">
        <div class="run-list" role="list">
          <button
              v-for="run in filtered"
              :key="run.run_id"
              type="button"
              class="run-row"
              :class="{'run-row--active': run.run_id === selectedId}"
              @click="selectRun(run.run_id)"
          >
            <div class="run-row-top">
              <n-tag
                  size="small"
                  round
                  :bordered="false"
                  :type="outcomeType(run.outcome)"
              >
                {{ outcomeLabel(run.outcome) }}
              </n-tag>
              <span class="run-time">{{ formatTime(run.started_at) }}</span>
            </div>
            <div class="run-msg">{{ preview(run.user_message) || t('runs.untitled') }}</div>
            <div class="run-meta">
              <span>{{ run.model || '—' }}</span>
              <span>{{ t('runs.rounds', {n: run.round_count || 0}) }}</span>
              <span>{{ formatDuration(durationMs(run)) }}</span>
            </div>
          </button>
        </div>

        <div v-if="selected" class="run-detail">
          <div class="detail-head">
            <n-tag size="small" round :bordered="false" :type="outcomeType(selected.outcome)">
              {{ outcomeLabel(selected.outcome) }}
            </n-tag>
            <span class="detail-id mono" :title="selected.run_id">{{ selected.run_id.slice(0, 8) }}</span>
            <n-button size="tiny" quaternary @click="copy(selected.run_id, 'runs.status.idCopied')">
              {{ t('btn.copyShort') }}
            </n-button>
          </div>

          <dl class="kv">
            <div>
              <dt>{{ t('runs.when') }}</dt>
              <dd>{{ formatTime(selected.started_at) }} · {{ formatDuration(durationMs(selected)) }}</dd>
            </div>
            <div>
              <dt>{{ t('col.chainId') }}</dt>
              <dd class="mono">{{ selected.chain_id || '—' }}</dd>
            </div>
            <div>
              <dt>{{ t('runs.model') }}</dt>
              <dd>{{ [selected.provider, selected.model].filter(Boolean).join(' / ') || '—' }}</dd>
            </div>
            <div>
              <dt>{{ t('runs.tokens') }}</dt>
              <dd>
                {{
                  selected.usage?.total_tokens
                      ? t('runs.tokenBreakdown', {
                        prompt: selected.usage.prompt_tokens || 0,
                        completion: selected.usage.completion_tokens || 0,
                        total: selected.usage.total_tokens,
                      })
                      : '—'
                }}
              </dd>
            </div>
          </dl>

          <section class="block">
            <h3>{{ t('runs.userMessage') }}</h3>
            <p class="block-text">{{ selected.user_message || '—' }}</p>
          </section>

          <section v-if="selected.error" class="block">
            <h3>{{ t('runs.error') }}</h3>
            <p class="block-text block-text--error">{{ selected.error }}</p>
          </section>

          <section v-if="selected.answer" class="block">
            <h3>{{ t('runs.answer') }}</h3>
            <p class="block-text">{{ selected.answer }}</p>
          </section>

          <section v-if="selected.tx_hashes?.length" class="block">
            <h3>{{ t('runs.txHashes') }}</h3>
            <div class="hash-list">
              <div v-for="hash in selected.tx_hashes" :key="hash" class="hash-row">
                <code class="mono">{{ hash }}</code>
                <n-button size="tiny" quaternary @click="copy(hash, 'runs.status.hashCopied')">
                  {{ t('btn.copyShort') }}
                </n-button>
              </div>
            </div>
          </section>

          <section v-if="selected.llm_rounds?.length" class="block">
            <h3>{{ t('runs.llmRounds') }}</h3>
            <table class="rounds-table">
              <thead>
              <tr>
                <th>{{ t('runs.col.round') }}</th>
                <th>{{ t('runs.col.latency') }}</th>
                <th>{{ t('runs.col.tokens') }}</th>
              </tr>
              </thead>
              <tbody>
              <tr v-for="round in selected.llm_rounds" :key="round.round">
                <td>{{ round.round }}</td>
                <td>{{ formatDuration(round.latency_ms) }}</td>
                <td>
                  {{
                    round.total_tokens
                        ? `${round.prompt_tokens || 0} + ${round.completion_tokens || 0} → ${round.total_tokens}`
                        : '—'
                  }}
                </td>
              </tr>
              </tbody>
            </table>
          </section>

          <section class="block">
            <h3>{{ t('runs.timeline') }}</h3>
            <p v-if="!timeline.length" class="block-text muted">{{ t('runs.noSteps') }}</p>
            <ol v-else class="timeline">
              <li v-for="(step, idx) in timeline" :key="stepKey(step, idx)" class="tl-item">
                <div class="tl-mark" :class="`tl-mark--${step.kind}`"/>
                <div class="tl-body">
                  <div class="tl-head">
                    <span class="tl-kind">{{ step.tool ? t('runs.step.tool') : stepKindLabel(step.kind) }}</span>
                    <span class="tl-title mono">{{ stepTitle(step) }}</span>
                    <n-tag
                        v-if="step.ok === true"
                        size="small"
                        round
                        :bordered="false"
                        type="success"
                    >ok
                    </n-tag>
                    <n-tag
                        v-else-if="step.ok === false"
                        size="small"
                        round
                        :bordered="false"
                        type="error"
                    >fail
                    </n-tag>
                    <span v-if="step.elapsed_ms" class="tl-ms">{{ formatDuration(step.elapsed_ms) }}</span>
                    <span v-if="step.round" class="tl-round">R{{ step.round }}</span>
                  </div>
                  <p v-if="step.detail && !step.tool" class="tl-detail">{{ step.detail }}</p>
                  <button
                      v-if="step.args || step.result || (step.tool && step.detail)"
                      type="button"
                      class="tl-toggle"
                      @click="toggleExpand(stepKey(step, idx))"
                  >
                    {{ expanded[stepKey(step, idx)] ? t('runs.hidePayload') : t('runs.showPayload') }}
                  </button>
                  <pre
                      v-if="expanded[stepKey(step, idx)] && (step.args || step.result || step.detail)"
                      class="tl-payload"
                  >{{
                      [
                        step.args ? `args:\n${step.args}` : '',
                        step.result ? `result:\n${step.result}` : '',
                        !step.args && !step.result && step.detail ? step.detail : '',
                      ].filter(Boolean).join('\n\n')
                    }}</pre>
                </div>
              </li>
            </ol>
          </section>
        </div>
      </div>

      <n-empty
          v-else-if="!loading"
          class="runs-empty"
          :description="runs.length ? t('runs.emptyFilter') : t('runs.empty')"
      />
    </n-spin>
  </div>
</template>

<style scoped>
.runs-pane {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
}

.runs-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
  margin-bottom: 8px;
}

.filter-select {
  width: 160px;
}

.logging-hint {
  display: block;
  margin-bottom: 8px;
  font-size: 12px;
}

.runs-spin {
  flex: 1;
  min-height: 0;
}

.runs-spin :deep(.n-spin-container),
.runs-spin :deep(.n-spin-content) {
  height: 100%;
  min-height: 0;
}

.runs-split {
  display: grid;
  grid-template-columns: minmax(240px, 320px) minmax(0, 1fr);
  gap: 12px;
  height: 100%;
  min-height: 0;
}

.run-list {
  overflow-y: auto;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding-right: 2px;
}

.run-row {
  display: flex;
  flex-direction: column;
  gap: 4px;
  width: 100%;
  padding: 10px 12px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  background: var(--bg-elevated);
  color: inherit;
  font-family: inherit;
  text-align: left;
  cursor: pointer;
}

.run-row:hover {
  background: var(--bg-hover);
}

.run-row--active {
  border-color: var(--accent);
  box-shadow: inset 0 0 0 1px var(--accent);
}

.run-row-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.run-time,
.run-meta {
  font-size: 11px;
  color: var(--text-muted);
}

.run-msg {
  font-size: 13px;
  font-weight: 500;
  color: var(--text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.run-meta {
  display: flex;
  gap: 8px;
  min-width: 0;
}

.run-meta span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.run-detail {
  min-width: 0;
  min-height: 0;
  overflow-y: auto;
  padding: 4px 4px 24px 8px;
}

.detail-head {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}

.detail-id {
  font-size: 12px;
  color: var(--text-muted);
}

.mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
}

.kv {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px 16px;
  margin: 0 0 16px;
}

.kv dt {
  font-size: 11px;
  color: var(--text-muted);
}

.kv dd {
  margin: 2px 0 0;
  font-size: 13px;
  color: var(--text-primary);
  word-break: break-all;
}

.block {
  margin-bottom: 16px;
}

.block h3 {
  margin: 0 0 6px;
  font-size: 12px;
  font-weight: 600;
  color: var(--text-secondary);
}

.block-text {
  margin: 0;
  font-size: 13px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-word;
  color: var(--text-primary);
}

.block-text--error {
  color: #d03050;
}

.muted {
  color: var(--text-muted);
}

.hash-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.hash-row {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.hash-row code {
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.rounds-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
}

.rounds-table th,
.rounds-table td {
  padding: 6px 8px;
  text-align: left;
  border-bottom: 1px solid var(--border-subtle);
}

.rounds-table th {
  color: var(--text-muted);
  font-weight: 500;
}

.timeline {
  margin: 0;
  padding: 0;
  list-style: none;
}

.tl-item {
  display: grid;
  grid-template-columns: 12px 1fr;
  gap: 10px;
  padding: 8px 0;
}

.tl-mark {
  width: 8px;
  height: 8px;
  margin-top: 6px;
  border-radius: 50%;
  background: var(--text-muted);
}

.tl-mark--tool {
  background: var(--accent);
}

.tl-mark--error {
  background: #d03050;
}

.tl-mark--confirm {
  background: #f0a020;
}

.tl-head {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  flex-wrap: wrap;
}

.tl-kind {
  font-size: 11px;
  font-weight: 600;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.02em;
}

.tl-title {
  font-size: 13px;
  color: var(--text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tl-ms,
.tl-round {
  font-size: 11px;
  color: var(--text-muted);
  margin-left: auto;
}

.tl-round {
  margin-left: 0;
}

.tl-detail {
  margin: 4px 0 0;
  font-size: 12px;
  color: var(--text-secondary);
  white-space: pre-wrap;
  word-break: break-word;
}

.tl-toggle {
  margin-top: 4px;
  padding: 0;
  border: none;
  background: none;
  color: var(--accent);
  font: inherit;
  font-size: 12px;
  cursor: pointer;
}

.tl-payload {
  margin: 6px 0 0;
  padding: 10px;
  max-height: 240px;
  overflow: auto;
  border-radius: var(--radius-sm);
  background: var(--bg-surface);
  font-size: 11px;
  line-height: 1.45;
  white-space: pre-wrap;
  word-break: break-word;
}

.runs-empty {
  padding-top: 48px;
}

@media (max-width: 720px) {
  .runs-split {
    grid-template-columns: 1fr;
    grid-template-rows: 180px 1fr;
  }

  .kv {
    grid-template-columns: 1fr;
  }
}
</style>
