<script setup lang="ts">
import {h, onMounted, ref} from 'vue'
import {useI18n} from 'vue-i18n'
import {NButton, NDataTable, NEmpty, NSpin, NTag, NText, type DataTableColumns, useDialog, useMessage} from 'naive-ui'
import * as App from '../../wailsjs/go/desktop/App'
import type {SettlementRefund} from '../types'

const props = defineProps<{active?: boolean}>()
const emit = defineEmits<{status: [msg: string]}>()

const {t} = useI18n()
const dialog = useDialog()
const message = useMessage()
const loading = ref(false)
const acting = ref('')
const rows = ref<SettlementRefund[]>([])

function setStatus(msg: string) {
  emit('status', msg)
}

function identifier(value: unknown): string {
  if (typeof value === 'string' && /^0x[0-9a-f]{64}$/i.test(value)) return value
  if (Array.isArray(value)) {
    for (const item of value) {
      const resolved = identifier(item)
      if (resolved) return resolved
    }
    return ''
  }
  if (value && typeof value === 'object') {
    const record = value as Record<string, unknown>
    for (const key of ['hex', 'Hex', 'id', 'ID', 'value', 'Value']) {
      const resolved = identifier(record[key])
      if (resolved) return resolved
    }
  }
  return ''
}

function normalize(row: SettlementRefund & Record<string, unknown>): SettlementRefund {
  return {
    chain_id: String(row.chain_id ?? row.ChainID ?? ''),
    payer: String(row.payer ?? row.Payer ?? ''),
    intent_id: identifier(row.display_intent_id ?? row.DisplayIntentID ?? row.intent_id ?? row.IntentID),
    task_id: identifier(row.task_id ?? row.TaskID),
    task_status: String(row.task_status ?? row.TaskStatus ?? ''),
    amount: String(row.amount ?? row.Amount ?? ''),
    available: String(row.available ?? row.Available ?? ''),
    token: String(row.token ?? row.Token ?? ''),
    refundable: Boolean(row.refundable ?? row.Refundable),
    cancellable: Boolean(row.cancellable ?? row.Cancellable),
    validator_state: String(row.validator_state ?? row.ValidatorState ?? ''),
    validator_error: String(row.validator_error ?? row.ValidatorError ?? ''),
  }
}

function short(value: string) {
  if (value.length <= 16) return value
  return `${value.slice(0, 10)}…${value.slice(-6)}`
}

function stateType(state: string): 'success' | 'error' | 'warning' | 'info' | 'default' {
  if (state === 'success') return 'success'
  if (state === 'failed' || state === 'cancelled') return 'error'
  if (state === 'assigned' || state === 'bound' || state === 'submitted' || state === 'validating' || state === 'retrying') return 'warning'
  return 'default'
}

function stateLabel(state: string) {
  const key = `settlement.state.${state}`
  const translated = t(key)
  return translated === key ? state : translated
}

function actionKey(row: SettlementRefund, action: string) {
  return `${action}:${row.chain_id}:${row.intent_id}:${row.task_id || ''}`
}

async function refresh() {
  loading.value = true
  try {
    const list = (await App.SettlementRefunds()) as SettlementRefund[]
    rows.value = (list || []).map((row) => normalize(row as SettlementRefund & Record<string, unknown>))
    setStatus(rows.value.length ? t('settlement.status.count', {n: rows.value.length}) : t('settlement.status.empty'))
  } catch (err) {
    setStatus(t('settlement.status.loadFailed', {err: String(err)}))
  } finally {
    loading.value = false
  }
}

function cancelTask(row: SettlementRefund) {
  if (!row.task_id) return
  dialog.warning({
    title: t('settlement.dialog.cancelTitle'),
    content: t('settlement.dialog.cancelBody', {task: short(row.task_id)}),
    positiveText: t('settlement.btn.cancelTask'),
    negativeText: t('dialog.cancel'),
    onPositiveClick: async () => {
      const key = actionKey(row, 'cancel')
      acting.value = key
      try {
        const txHash = await App.SettlementCancel(row.chain_id, row.task_id)
        message.success(t('settlement.status.cancelled', {tx: short(String(txHash))}))
        await refresh()
      } catch (err) {
        message.error(t('settlement.status.cancelFailed', {err: String(err)}))
      } finally {
        acting.value = ''
      }
    },
  })
}

function refund(row: SettlementRefund) {
  dialog.warning({
    title: t('settlement.dialog.refundTitle'),
    content: t('settlement.dialog.refundBody', {amount: row.available, task: short(row.task_id || '')}),
    positiveText: t('settlement.btn.refund'),
    negativeText: t('dialog.cancel'),
    onPositiveClick: async () => {
      const key = actionKey(row, 'refund')
      acting.value = key
      try {
        const txHash = await App.SettlementRefundEntry(row.chain_id, row.task_id || '', row.task_status === 'unassigned')
        message.success(t('settlement.status.refunded', {tx: short(String(txHash))}))
        await refresh()
      } catch (err) {
        message.error(t('settlement.status.refundFailed', {err: String(err)}))
      } finally {
        acting.value = ''
      }
    },
  })
}

const columns: DataTableColumns<SettlementRefund> = [
  {title: () => t('col.chainId'), key: 'chain_id', width: 148},
  {title: () => t('settlement.col.task'), key: 'task_id', width: 220, render: (row) => row.task_id ? h(NText, {code: true, class: 'settlement-id'}, {default: () => short(row.task_id)}) : '—'},
  {title: () => t('settlement.col.status'), key: 'task_status', width: 150, render: (row) => h(NTag, {type: stateType(row.task_status), size: 'small', title: row.validator_error || undefined}, {default: () => stateLabel(row.task_status)})},
  {title: () => t('settlement.col.taskAmount'), key: 'amount', width: 138, render: (row) => row.amount || '—'},
  {title: () => t('settlement.col.available'), key: 'available', width: 144},
  {
    title: () => t('settlement.col.action'),
    key: 'action',
    width: 214,
    render: (row) => h('div', {style: 'display:flex; gap:8px'}, [
      row.cancellable ? h(NButton, {
        size: 'small', type: 'warning', secondary: true, loading: acting.value === actionKey(row, 'cancel'), onClick: () => cancelTask(row),
      }, {default: () => t('settlement.btn.cancelTask')}) : null,
      row.refundable ? h(NButton, {
        size: 'small', type: 'primary', loading: acting.value === actionKey(row, 'refund'), onClick: () => refund(row),
      }, {default: () => t('settlement.btn.refund')}) : null,
      !row.cancellable && !row.refundable ? h(NText, {depth: 3}, {default: () => t('settlement.noAction')}) : null,
    ]),
  },
]

onMounted(() => {
  if (props.active) void refresh()
})
defineExpose({refresh})
</script>

<template>
  <div class="pane-body">
    <div class="section-heading">
      <div>
        <h2>{{ t('settlement.title') }}</h2>
        <n-text depth="3">{{ t('settlement.hint') }}</n-text>
      </div>
      <n-button secondary :loading="loading" @click="refresh">{{ t('btn.refresh') }}</n-button>
    </div>

    <n-spin :show="loading">
      <n-empty v-if="!loading && rows.length === 0" :description="t('settlement.empty')"/>
      <n-data-table
          v-else
          :columns="columns"
          :data="rows"
          :row-key="(row: SettlementRefund) => `${row.chain_id}:${row.intent_id}:${row.task_id || 'intent'}`"
          :bordered="false"
          :single-line="false"
          :scroll-x="984"
      />
    </n-spin>
  </div>
</template>
