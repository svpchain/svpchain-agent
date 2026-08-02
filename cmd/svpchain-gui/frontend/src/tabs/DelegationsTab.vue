<script setup lang="ts">
import { h, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NButton,
  NDataTable,
  NSpace,
  NTag,
  NText,
  useMessage,
  type DataTableColumns,
} from 'naive-ui'
import * as App from '../../wailsjs/go/desktop/App'
import type { DelegationRow } from '../types'

const emit = defineEmits<{ status: [msg: string] }>()

const { t, locale } = useI18n()
const message = useMessage()

const rows = ref<DelegationRow[]>([])
const loading = ref(false)
const busyRootId = ref('')
const revokePendingId = ref('')
const lastTx = ref('')

function setStatus(msg: string) {
  emit('status', msg)
}

function truncateMiddle(s: string, head = 10, tail = 6) {
  if (!s || s.length <= head + tail + 1) return s
  return `${s.slice(0, head)}…${s.slice(-tail)}`
}

function copyRootId(rootId: string) {
  if (!rootId) return
  App.CopyText(rootId)
  setStatus(t('delegations.status.rootCopied'))
}

function formatExpiry(unixSeconds: number) {
  if (!unixSeconds) return '—'
  return new Date(unixSeconds * 1000).toLocaleString(locale.value === 'zh' ? 'zh-CN' : 'en-US')
}

async function refresh() {
  loading.value = true
  setStatus(t('delegations.status.loading'))
  try {
    const list = (await App.DelegationsList()) as unknown as DelegationRow[]
    rows.value = list || []
    setStatus(t('delegations.status.count', { n: rows.value.length }))
  } catch (err) {
    setStatus(t('delegations.status.loadFailed', { err: String(err) }))
  } finally {
    loading.value = false
  }
}

async function runOp(row: DelegationRow, op: 'pause' | 'resume' | 'revoke') {
  const opLabel = t(`delegations.op.${op}`)
  busyRootId.value = row.root_id
  try {
    let hash = ''
    if (op === 'pause') hash = await App.DelegationPause(row.root_id)
    else if (op === 'resume') hash = await App.DelegationResume(row.root_id)
    else hash = await App.DelegationRevoke(row.root_id)
    lastTx.value = t('delegations.status.txSubmitted', { op: opLabel, hash })
    setStatus(lastTx.value)
    await refresh()
  } catch (err) {
    message.error(t('delegations.status.opFailed', { op: opLabel, err: String(err) }))
  } finally {
    busyRootId.value = ''
    revokePendingId.value = ''
  }
}

function onRevokeClick(row: DelegationRow) {
  if (revokePendingId.value === row.root_id) {
    runOp(row, 'revoke')
  } else {
    revokePendingId.value = row.root_id
  }
}

const columns: DataTableColumns<DelegationRow> = [
  {
    title: () => t('delegations.col.rootId'),
    key: 'root_id',
    width: 170,
    render: (row) =>
      h('div', { class: 'addr-cell' }, [
        h('span', { class: 'addr-text mono', title: row.root_id }, truncateMiddle(row.root_id)),
        h(
          NButton,
          { size: 'tiny', quaternary: true, onClick: () => copyRootId(row.root_id) },
          { default: () => t('btn.copyShort') },
        ),
      ]),
  },
  {
    title: () => t('delegations.col.agent'),
    key: 'agent_id',
    width: 170,
    render: (row) =>
      h('div', { class: 'agent-cell' }, [
        h('span', { class: 'addr-text mono', title: row.agent_id }, truncateMiddle(row.agent_id)),
        row.self_issued
          ? h(
              NTag,
              { size: 'small', round: true, type: 'info', bordered: false },
              { default: () => t('delegations.self') },
            )
          : null,
      ]),
  },
  {
    title: () => t('delegations.col.actions'),
    key: 'actions',
    width: 140,
    render: (row) => (row.actions || []).join(', ') || '—',
  },
  {
    title: () => t('delegations.col.subaccounts'),
    key: 'subaccounts',
    width: 110,
    render: (row) => (row.subaccounts || []).join(', ') || '—',
  },
  {
    title: () => t('delegations.col.caps'),
    key: 'caps',
    width: 180,
    render: (row) =>
      h('div', { class: 'caps-cell' }, [
        h('div', {}, `${t('delegations.total')} ${row.total_cap} · ${t('delegations.daily')} ${row.daily_cap}`),
        h('div', { class: 'caps-spent' }, `${t('delegations.spent')} ${row.spent_total}`),
      ]),
  },
  {
    title: () => t('delegations.col.epoch'),
    key: 'epoch',
    width: 70,
  },
  {
    title: () => t('delegations.col.state'),
    key: 'paused',
    width: 100,
    render: (row) =>
      h(
        NTag,
        { size: 'small', round: true, bordered: false, type: row.paused ? 'warning' : 'success' },
        { default: () => (row.paused ? t('delegations.paused') : t('delegations.active')) },
      ),
  },
  {
    title: () => t('delegations.col.expires'),
    key: 'expires_at',
    width: 160,
    render: (row) => formatExpiry(row.expires_at),
  },
  {
    title: () => t('delegations.col.ops'),
    key: 'ops',
    width: 220,
    render: (row) => {
      const busy = busyRootId.value === row.root_id
      const buttons = []
      if (!row.paused) {
        buttons.push(
          h(
            NButton,
            { size: 'tiny', loading: busy, disabled: !!busyRootId.value && !busy, onClick: () => runOp(row, 'pause') },
            { default: () => t('delegations.btn.pause') },
          ),
        )
      } else {
        buttons.push(
          h(
            NButton,
            { size: 'tiny', loading: busy, disabled: !!busyRootId.value && !busy, onClick: () => runOp(row, 'resume') },
            { default: () => t('delegations.btn.resume') },
          ),
        )
      }
      buttons.push(
        h(
          NButton,
          {
            size: 'tiny',
            type: 'error',
            ghost: revokePendingId.value !== row.root_id,
            loading: busy,
            disabled: !!busyRootId.value && !busy,
            onClick: () => onRevokeClick(row),
          },
          {
            default: () =>
              revokePendingId.value === row.root_id
                ? t('delegations.btn.confirmRevoke')
                : t('delegations.btn.revoke'),
          },
        ),
      )
      if (revokePendingId.value === row.root_id && !busy) {
        buttons.push(
          h(
            NButton,
            { size: 'tiny', quaternary: true, onClick: () => (revokePendingId.value = '') },
            { default: () => t('dialog.cancel') },
          ),
        )
      }
      return h('div', { class: 'ops-cell' }, buttons)
    },
  },
]

onMounted(refresh)
</script>

<template>
  <div class="pane-body">
    <n-space class="delegations-toolbar" align="center">
      <n-button :loading="loading" @click="refresh">{{ t('btn.refresh') }}</n-button>
      <n-text v-if="lastTx" depth="3" class="tx-line">{{ lastTx }}</n-text>
    </n-space>
    <n-data-table
      :columns="columns"
      :data="rows"
      :loading="loading"
      :row-key="(row: DelegationRow) => row.root_id"
      :scroll-x="1320"
      size="small"
      :max-height="520"
    />
    <n-text v-if="!loading && rows.length === 0" depth="3" class="hint">
      {{ t('delegations.empty') }}
    </n-text>
  </div>
</template>

<style scoped>
.delegations-toolbar {
  margin-bottom: 12px;
}

.tx-line {
  font-size: 12px;
  word-break: break-all;
}

:deep(.mono) {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
}

:deep(.agent-cell) {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

:deep(.caps-cell) {
  font-size: 12px;
  line-height: 1.5;
}

:deep(.caps-spent) {
  color: var(--text-muted);
}

:deep(.ops-cell) {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}
</style>
