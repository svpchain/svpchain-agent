<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NButton,
  NInput,
  NForm,
  NFormItem,
  NDataTable,
  NDivider,
  NSpace,
  NSelect,
  NText,
  useMessage,
  useDialog,
  type DataTableColumns,
} from 'naive-ui'
import * as App from '../../wailsjs/go/desktop/App'
import { useAddressCell } from '../composables/useAddressCell'
import type { WhitelistEntry } from '../types'

const props = defineProps<{ defaultChainIds: string[] }>()
const emit = defineEmits<{ status: [msg: string] }>()

const { t } = useI18n()
const message = useMessage()
const dialog = useDialog()
const { addressCell } = useAddressCell((msg) => emit('status', msg))

const whitelistEntries = ref<WhitelistEntry[]>([])
const selectedWhitelist = ref<WhitelistEntry | null>(null)
const whitelistChainId = ref('')
const whitelistAddressType = ref('cosmos')
const whitelistAddress = ref('')
const whitelistAlias = ref('')

const addressTypeOptions = [
  { label: () => t('addressType.cosmos'), value: 'cosmos' },
  { label: () => t('addressType.evm'), value: 'evm' },
]

function setStatus(msg: string) {
  emit('status', msg)
}

function normalizeWhitelistEntry(row: WhitelistEntry & {
  chain_id?: string
  address_type?: string
  address?: string
  alias?: string
}): WhitelistEntry {
  return {
    ChainID: row.ChainID ?? row.chain_id ?? '',
    AddressType: row.AddressType ?? row.address_type ?? '',
    Address: row.Address ?? row.address ?? '',
    Alias: row.Alias ?? row.alias ?? '',
  }
}

function whitelistKey(row: WhitelistEntry) {
  return `${row.ChainID}|${row.AddressType}|${row.Address}`
}

function addressTypeLabel(type: string) {
  return type === 'evm' ? t('addressType.evm') : t('addressType.cosmos')
}

const whitelistColumns: DataTableColumns<WhitelistEntry> = [
  { title: () => t('col.alias'), key: 'Alias', width: 140 },
  { title: () => t('col.chainId'), key: 'ChainID', width: 140 },
  {
    title: () => t('col.addressType'),
    key: 'AddressType',
    width: 140,
    render: (row) => addressTypeLabel(row.AddressType),
  },
  {
    title: () => t('col.address'),
    key: 'Address',
    render: (row) => addressCell(row.Address),
  },
]

async function refreshWhitelist() {
  try {
    const list = (await App.ListWhitelist()) as WhitelistEntry[]
    whitelistEntries.value = (list || []).map(normalizeWhitelistEntry)
    selectedWhitelist.value = null
    if (whitelistEntries.value.length === 0) {
      setStatus(t('status.noWhitelist'))
    } else {
      setStatus(t('status.whitelistCount', { n: whitelistEntries.value.length }))
    }
  } catch (err) {
    setStatus(t('status.readWhitelistFailed', { err: String(err) }))
  }
}

function selectWhitelistRow(row: WhitelistEntry) {
  selectedWhitelist.value = row
  whitelistChainId.value = row.ChainID
  whitelistAddressType.value = row.AddressType
  whitelistAddress.value = row.Address
  whitelistAlias.value = row.Alias
}

async function saveWhitelist() {
  const chainID = whitelistChainId.value.trim()
  const address = whitelistAddress.value.trim()
  if (!chainID) {
    setStatus(t('status.enterChainId'))
    return
  }
  if (!address) {
    setStatus(t('status.enterWhitelistAddress'))
    return
  }
  try {
    const saved = normalizeWhitelistEntry(
      (await App.AddWhitelist(
        chainID,
        whitelistAddressType.value,
        address,
        whitelistAlias.value.trim(),
      )) as WhitelistEntry,
    )
    whitelistAddress.value = ''
    whitelistAlias.value = ''
    await refreshWhitelist()
    setStatus(t('status.savedWhitelist', { address: saved.Address, chain: saved.ChainID }))
  } catch (err) {
    message.error(String(err))
  }
}

function deleteSelectedWhitelist() {
  const row = selectedWhitelist.value
  if (!row) {
    setStatus(t('status.selectWhitelistToDelete'))
    return
  }
  dialog.warning({
    title: t('dialog.confirmDeleteWhitelistTitle'),
    content: t('dialog.confirmDeleteWhitelistBody', {
      chain: row.ChainID,
      type: addressTypeLabel(row.AddressType),
      address: row.Address,
    }),
    positiveText: t('dialog.confirm'),
    negativeText: t('dialog.cancel'),
    onPositiveClick: async () => {
      try {
        await App.DeleteWhitelist(row.ChainID, row.AddressType, row.Address)
        await refreshWhitelist()
        setStatus(t('status.deletedWhitelist', { address: row.Address }))
      } catch (err) {
        message.error(String(err))
      }
    },
  })
}

async function init() {
  whitelistChainId.value = props.defaultChainIds[0] || ''
  await refreshWhitelist()
}

onMounted(init)
</script>

<template>
  <div class="pane-body">
    <n-divider title-placement="left" class="section-divider">{{ t('tab.addWhitelist') }}</n-divider>
    <n-form label-placement="top">
      <n-form-item :label="t('field.chainId')">
        <n-select
          v-model:value="whitelistChainId"
          filterable
          tag
          :placeholder="t('ph.chainId')"
          :options="defaultChainIds.map((c) => ({ label: c, value: c }))"
        />
      </n-form-item>
      <n-form-item :label="t('field.addressType')">
        <n-select
          v-model:value="whitelistAddressType"
          :options="addressTypeOptions.map((o) => ({ label: o.label(), value: o.value }))"
        />
      </n-form-item>
      <n-form-item :label="t('field.whitelistAddress')">
        <n-input v-model:value="whitelistAddress" :placeholder="t('ph.whitelistAddress')" />
      </n-form-item>
      <n-form-item :label="t('field.whitelistAlias')">
        <n-input v-model:value="whitelistAlias" :placeholder="t('ph.whitelistAlias')" />
      </n-form-item>
    </n-form>
    <n-button type="primary" @click="saveWhitelist">{{ t('btn.saveWhitelist') }}</n-button>
    <n-text depth="3" class="hint">{{ t('hint.whitelist') }}</n-text>

    <n-divider title-placement="left" class="section-divider">{{ t('tab.storedWhitelist') }}</n-divider>
    <n-data-table
      :columns="whitelistColumns"
      :data="whitelistEntries"
      :row-key="(row: WhitelistEntry) => whitelistKey(row)"
      :row-props="(row: WhitelistEntry) => ({
        onClick: () => selectWhitelistRow(row),
        class: selectedWhitelist && whitelistKey(row) === whitelistKey(selectedWhitelist) ? 'row-selected' : '',
      })"
      size="small"
      :max-height="360"
    />
    <n-space class="actions">
      <n-button @click="refreshWhitelist">{{ t('btn.refresh') }}</n-button>
      <n-button type="error" ghost @click="deleteSelectedWhitelist">{{ t('btn.delete') }}</n-button>
    </n-space>
  </div>
</template>
