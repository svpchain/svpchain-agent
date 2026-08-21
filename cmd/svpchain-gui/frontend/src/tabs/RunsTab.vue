<script setup lang="ts">
import {computed, onMounted, ref, watch} from 'vue'
import {useI18n} from 'vue-i18n'
import {NButton, NEmpty, NSelect, NSpin, NTag, NText, useDialog, useMessage} from 'naive-ui'
import * as App from '../../wailsjs/go/desktop/App'
import type {AgentRun, AgentRunOutcome, AgentRunStep, AgentTxCheck} from '../types'

const props = defineProps<{active?: boolean}>()
const emit = defineEmits<{status: [msg: string]; 'open-session': [id: string]}>()

const {t, locale} = useI18n()
const dialog = useDialog()
const message = useMessage()

const loading = ref(false)
const logPath = ref('')
const loggingOn = ref(true)
const runs = ref<AgentRun[]>([])
const selectedId = ref('')
const outcomeFilter = ref<string>('all')
const expanded = ref<Record<string, boolean>>({})
const deleting = ref(false)
const checking = ref(false)
const sessionTitles = ref<Record<string, string>>({})

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

function sessionLabel(run: AgentRun) {
  const id = run.session_id
  if (!id) return ''
  return sessionTitles.value[id] || run.session_title || id.slice(0, 8)
}

function sessionStillExists(id?: string) {
  return !!id && Object.prototype.hasOwnProperty.call(sessionTitles.value, id)
}

function txRows(run: AgentRun): {hash: string; check?: AgentTxCheck}[] {
  const checks = run.tx_checks || []
  const byHash = new Map(checks.map((c) => [c.hash, c]))
  const hashes = run.tx_hashes?.length ? run.tx_hashes : checks.map((c) => c.hash)
  return hashes.map((hash) => ({hash, check: byHash.get(hash)}))
}

function txStatusType(status?: string): 'success' | 'error' | 'warning' | 'default' {
  if (status === 'confirmed') return 'success'
  if (status === 'failed' || status === 'error') return 'error'
  if (status === 'pending') return 'warning'
  return 'default'
}

function txStatusLabel(status?: string) {
  if (!status) return ''
  const key = `runs.txStatus.${status}`
  const translated = t(key)
  return translated === key ? status : translated
}

function intentStatusType(status?: string): 'success' | 'error' | 'warning' | 'info' | 'default' {
  if (status === 'matched') return 'success'
  if (status === 'mismatch') return 'error'
  if (status === 'unobserved' || status === 'skipped') return 'warning'
  if (status === 'included') return 'info'
  return 'default'
}

function intentStatusLabel(status?: string) {
  if (!status) return ''
  const key = `runs.intentStatus.${status}`
  const translated = t(key)
  return translated === key ? status : translated
}

function expectLine(expect?: Record<string, string>) {
  if (!expect) return ''
  return Object.entries(expect)
      .filter(([, v]) => v)
      .map(([k, v]) => `${k}=${v}`)
      .join(' · ')
}

function openSelectedSession() {
  const id = selected.value?.session_id
  if (!id) return
  if (!sessionStillExists(id)) {
    setStatus(t('runs.sessionGone'))
    return
  }
  emit('open-session', id)
}

async function recheckSelected() {
  const run = selected.value
  if (!run || checking.value) return
  checking.value = true
  try {
    const updated = (await App.AgentRecheckRunTxs(run.run_id)) as unknown as AgentRun
    runs.value = runs.value.map((r) => (r.run_id === updated.run_id ? updated : r))
    setStatus(t('runs.status.rechecked'))
  } catch (err) {
    setStatus(t('runs.status.recheckFailed', {err: String(err)}))
  } finally {
    checking.value = false
  }
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
    try {
      const sessions = ((await App.AgentSessions()) || []) as Array<{id: string; title: string}>
      const titles: Record<string, string> = {}
      for (const s of sessions) {
        if (s.id) titles[s.id] = s.title || ''
      }
      sessionTitles.value = titles
    } catch {
      sessionTitles.value = {}
    }
    if (!selectedId.value && list.length) selectedId.value = list[0].run_id
    setStatus(t('runs.status.count', {n: list.length}))
  } catch (err) {
    runs.value = []
    setStatus(t('runs.status.loadFailed', {err: String(err)}))
  } finally {
    loading.value = false
  }
}

function confirmDeleteSelected() {
  const run = selected.value
  if (!run) return
  dialog.warning({
    title: t('runs.dialog.deleteTitle'),
    content: t('runs.dialog.deleteBody'),
    positiveText: t('dialog.confirm'),
    negativeText: t('dialog.cancel'),
    onPositiveClick: () => deleteRun(run.run_id),
  })
}

function confirmClearAll() {
  if (!runs.value.length) return
  dialog.warning({
    title: t('runs.dialog.clearTitle'),
    content: t('runs.dialog.clearBody'),
    positiveText: t('dialog.confirm'),
    negativeText: t('dialog.cancel'),
    onPositiveClick: clearAll,
  })
}

async function deleteRun(id: string) {
  deleting.value = true
  try {
    await App.AgentDeleteRun(id)
    if (selectedId.value === id) selectedId.value = ''
    await refresh()
    setStatus(t('runs.status.deleted'))
  } catch (err) {
    message.error(t('runs.status.deleteFailed', {err: String(err)}))
  } finally {
    deleting.value = false
  }
}

async function clearAll() {
  deleting.value = true
  try {
    await App.AgentClearRuns()
    selectedId.value = ''
    await refresh()
    setStatus(t('runs.status.cleared'))
  } catch (err) {
    message.error(t('runs.status.clearFailed', {err: String(err)}))
  } finally {
    deleting.value = false
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
      <n-button
          class="clear-btn"
          size="small"
          quaternary
          :disabled="!runs.length || deleting"
          :loading="deleting"
          @click="confirmClearAll"
      >
        {{ t('runs.btn.clearAll') }}
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
              <span v-if="sessionLabel(run)">{{ sessionLabel(run) }}</span>
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
            <n-button
                class="delete-run"
                size="tiny"
                quaternary
                type="error"
                :disabled="deleting"
                :loading="deleting"
                @click="confirmDeleteSelected"
            >
              {{ t('runs.btn.delete') }}
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
            <div v-if="selected.session_id">
              <dt>{{ t('runs.session') }}</dt>
              <dd class="hash-row">
                <span>{{ sessionLabel(selected) }}</span>
                <n-button size="tiny" quaternary @click="openSelectedSession">
                  {{ t('runs.openSession') }}
                </n-button>
              </dd>
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
            <div v-if="selected.prompt_sha256">
              <dt>{{ t('runs.promptHash') }}</dt>
              <dd class="hash-row">
                <code class="mono" :title="selected.prompt_sha256">{{ selected.prompt_sha256.slice(0, 16) }}…</code>
                <n-button size="tiny" quaternary @click="copy(selected.prompt_sha256, 'runs.status.promptHashCopied')">
                  {{ t('btn.copyShort') }}
                </n-button>
              </dd>
            </div>
          </dl>

          <section v-if="selected.skills?.length" class="block">
            <h3>{{ t('runs.skills') }}</h3>
            <div class="skill-tags">
              <n-tag v-for="name in selected.skills" :key="name" size="small" round :bordered="false">
                {{ name }}
              </n-tag>
            </div>
          </section>

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

          <section v-if="txRows(selected).length" class="block">
            <h3>
              {{ t('runs.txHashes') }}
              <n-button
                  size="tiny"
                  quaternary
                  :disabled="checking"
                  :loading="checking"
                  @click="recheckSelected"
              >
                {{ t('runs.txRecheck') }}
              </n-button>
            </h3>
            <div class="hash-list">
              <div v-for="row in txRows(selected)" :key="row.hash" class="hash-row">
                <code class="mono">{{ row.hash }}</code>
                <n-tag
                    v-if="row.check?.status"
                    size="small"
                    round
                    :bordered="false"
                    :type="txStatusType(row.check.status)"
                >
                  {{ txStatusLabel(row.check.status) }}
                  <template v-if="row.check.height"> · {{ row.check.height }}</template>
                </n-tag>
                <n-button size="tiny" quaternary @click="copy(row.hash, 'runs.status.hashCopied')">
                  {{ t('btn.copyShort') }}
                </n-button>
              </div>
            </div>
          </section>

          <section v-if="selected.intent_checks?.length" class="block">
            <h3>{{ t('runs.intents') }}</h3>
            <ul class="intent-list">
              <li v-for="(item, idx) in selected.intent_checks" :key="`${item.tool}-${idx}`" class="intent-row">
                <div class="hash-row">
                  <span class="mono">{{ item.tool }}</span>
                  <n-tag
                      size="small"
                      round
                      :bordered="false"
                      :type="intentStatusType(item.status)"
                  >
                    {{ intentStatusLabel(item.status) }}
                  </n-tag>
                </div>
                <p v-if="expectLine(item.expect)" class="block-text muted">{{ expectLine(item.expect) }}</p>
                <p v-if="item.detail" class="block-text muted">{{ item.detail }}</p>
              </li>
            </ul>
          </section>

          <section v-if="selected.llm_rounds?.length" class="block">
            <h3>{{ t('runs.llmRounds') }}</h3>
            <div class="round-list">
              <article v-for="round in selected.llm_rounds" :key="round.round" class="round-card">
                <div class="round-head">
                  <span class="round-num">R{{ round.round }}</span>
                  <span>{{ formatDuration(round.latency_ms) }}</span>
                  <span>
                    {{
                      round.total_tokens
                          ? `${round.prompt_tokens || 0} + ${round.completion_tokens || 0} → ${round.total_tokens}`
                          : '—'
                    }}
                  </span>
                </div>
                <p v-if="round.reply" class="block-text">{{ round.reply }}</p>
                <ul v-if="round.tool_calls?.length" class="call-list">
                  <li v-for="(call, cidx) in round.tool_calls" :key="call.id || `${call.name}-${cidx}`">
                    <div class="call-head">
                      <span class="tl-kind">{{ t('runs.toolCalls') }}</span>
                      <span class="mono">{{ call.name }}</span>
                    </div>
                    <pre v-if="call.args" class="tl-payload">{{ call.args }}</pre>
                  </li>
                </ul>
              </article>
            </div>
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

.clear-btn {
  margin-left: auto;
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

.delete-run {
  margin-left: auto;
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
  display: flex;
  align-items: center;
  gap: 8px;
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

.intent-list {
  margin: 0;
  padding: 0;
  list-style: none;
}

.intent-row + .intent-row {
  border-top: 1px solid var(--border-subtle);
}

.skill-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.round-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.round-card {
  padding: 10px 12px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  background: var(--bg-surface);
}

.round-head {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 6px;
  font-size: 12px;
  color: var(--text-muted);
}

.round-num {
  font-weight: 600;
  color: var(--text-secondary);
}

.call-list {
  margin: 8px 0 0;
  padding: 0;
  list-style: none;
}

.call-head {
  display: flex;
  align-items: center;
  gap: 8px;
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
