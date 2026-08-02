<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NInput, NSpin, NTag, NText } from 'naive-ui'
import * as App from '../../wailsjs/go/desktop/App'
import type { DiscoveredAgent } from '../types'

const emit = defineEmits<{ status: [msg: string] }>()

const { t } = useI18n()

const capability = ref('')
const agents = ref<DiscoveredAgent[]>([])
const loading = ref(false)
const errorMsg = ref('')
const searched = ref(false)

function setStatus(msg: string) {
  emit('status', msg)
}

function truncateMiddle(s: string, head = 12, tail = 8) {
  if (!s || s.length <= head + tail + 1) return s
  return `${s.slice(0, head)}…${s.slice(-tail)}`
}

function copyText(text: string) {
  if (!text) return
  App.CopyText(text)
  setStatus(t('status.addressCopied'))
}

function badgeKind(a: DiscoveredAgent): 'verified' | 'unreachable' | 'mismatch' {
  if (a.card_verified) return 'verified'
  if (a.card_error) return 'unreachable'
  return 'mismatch'
}

async function discover() {
  loading.value = true
  errorMsg.value = ''
  setStatus(t('agents.discovering'))
  try {
    const list = (await App.AgentsDiscover(capability.value.trim())) as unknown as DiscoveredAgent[]
    agents.value = list || []
    searched.value = true
    setStatus(t('agents.status.found', { n: agents.value.length }))
  } catch (err) {
    agents.value = []
    searched.value = true
    errorMsg.value = String(err)
    setStatus(t('agents.status.failed', { err: String(err) }))
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="pane-body">
    <div class="discover-bar">
      <n-input
        v-model:value="capability"
        class="capability-input"
        :placeholder="t('agents.ph.capability')"
        :disabled="loading"
        @keyup.enter="discover"
      />
      <n-button type="primary" :loading="loading" @click="discover">
        {{ t('agents.btn.discover') }}
      </n-button>
    </div>

    <n-text v-if="errorMsg" type="error" class="hint error-text">{{ errorMsg }}</n-text>

    <n-spin :show="loading">
      <div v-if="agents.length" class="agent-list">
        <div v-for="agent in agents" :key="agent.agent_id" class="agent-card">
          <div class="agent-card-head">
            <span class="agent-id" :title="agent.agent_id">{{ truncateMiddle(agent.agent_id) }}</span>
            <n-button size="tiny" quaternary @click="copyText(agent.agent_id)">
              {{ t('btn.copyShort') }}
            </n-button>
            <span
              class="card-badge"
              :class="`card-badge--${badgeKind(agent)}`"
              :title="agent.card_error || ''"
            >
              <svg v-if="badgeKind(agent) === 'verified'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                <path d="M5 13l4 4L19 7" stroke-linecap="round" stroke-linejoin="round" />
              </svg>
              <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M12 9v4M12 16.5h.01" stroke-linecap="round" />
                <path d="M10.3 4.2 2.9 17a2 2 0 0 0 1.7 3h14.8a2 2 0 0 0 1.7-3L13.7 4.2a2 2 0 0 0-3.4 0z" stroke-linejoin="round" />
              </svg>
              {{
                badgeKind(agent) === 'verified'
                  ? t('agents.cardVerified')
                  : badgeKind(agent) === 'unreachable'
                    ? t('agents.cardUnreachable')
                    : t('agents.cardMismatch')
              }}
            </span>
          </div>
          <div class="agent-row">
            <span class="agent-label">{{ t('agents.endpoint') }}</span>
            <span class="agent-value" :title="agent.endpoint">{{ agent.endpoint }}</span>
          </div>
          <div class="agent-row">
            <span class="agent-label">{{ t('agents.capabilities') }}</span>
            <span class="agent-tags">
              <n-tag
                v-for="cap in agent.capabilities || []"
                :key="cap"
                size="small"
                round
                :bordered="false"
              >{{ cap }}</n-tag>
            </span>
          </div>
          <div v-if="agent.pricing_text" class="agent-row">
            <span class="agent-label">{{ t('agents.pricing') }}</span>
            <span class="agent-value">{{ agent.pricing_text }}</span>
          </div>
          <div v-if="agent.bond_text" class="agent-row">
            <span class="agent-label">{{ t('agents.bond') }}</span>
            <span class="agent-value">{{ agent.bond_text }}</span>
          </div>
        </div>
      </div>
      <n-text v-else-if="searched && !loading && !errorMsg" depth="3" class="hint">
        {{ t('agents.empty') }}
      </n-text>
    </n-spin>
  </div>
</template>

<style scoped>
.discover-bar {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 14px;
}

.capability-input {
  flex: 1;
  min-width: 0;
}

.error-text {
  display: block;
  margin-bottom: 10px;
}

.agent-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.agent-card {
  padding: 12px 14px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  background: var(--bg-elevated);
}

.agent-card-head {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}

.agent-id {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.card-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  margin-left: auto;
  padding: 2px 8px;
  border-radius: 999px;
  font-size: 11px;
  font-weight: 600;
  flex-shrink: 0;
}

.card-badge svg {
  width: 12px;
  height: 12px;
}

.card-badge--verified {
  color: #18a058;
  background: rgba(24, 160, 88, 0.14);
}

.card-badge--mismatch {
  color: #f0a020;
  background: rgba(240, 160, 32, 0.14);
}

.card-badge--unreachable {
  color: #d03050;
  background: rgba(208, 48, 80, 0.14);
}

.agent-row {
  display: flex;
  align-items: baseline;
  gap: 10px;
  margin-top: 4px;
  font-size: 12px;
}

.agent-label {
  flex-shrink: 0;
  width: 90px;
  color: var(--text-muted);
}

.agent-value {
  min-width: 0;
  color: var(--text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.agent-tags {
  display: inline-flex;
  flex-wrap: wrap;
  gap: 6px;
}
</style>
