<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NButton,
  NInput,
  NInputGroup,
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
import type { Entry } from '../types'

const props = defineProps<{
  defaultChainIds: string[]
  entries: Entry[]
}>()

const emit = defineEmits<{
  status: [msg: string]
  'update:entries': [entries: Entry[]]
}>()

const { t } = useI18n()
const message = useMessage()
const dialog = useDialog()
const { addressCell } = useAddressCell((msg) => emit('status', msg))

const selectedChainId = ref<string | null>(null)
const importChainId = ref('')
const importKey = ref('')

function setStatus(msg: string) {
  emit('status', msg)
}

const keyColumns: DataTableColumns<Entry> = [
  { title: () => t('col.chainId'), key: 'ChainID', width: 140 },
  {
    title: () => t('col.cosmos'),
    key: 'Owner',
    render: (row) => addressCell(row.Owner),
  },
  {
    title: () => t('col.evm'),
    key: 'EVMAddr',
    render: (row) => addressCell(row.EVMAddr),
  },
]

async function refreshKeys() {
  try {
    const list = (await App.ListKeys()) as Entry[]
    const entries = list || []
    emit('update:entries', entries)
    selectedChainId.value = null
    if (entries.length === 0) {
      setStatus(t('status.noKeys'))
    } else {
      setStatus(t('status.keyCount', { n: entries.length }))
    }
  } catch (err) {
    setStatus(t('status.readKeysFailed', { err: String(err) }))
  }
}

function selectRow(row: Entry) {
  selectedChainId.value = row.ChainID
  importChainId.value = row.ChainID
}

function deleteSelected() {
  const id = selectedChainId.value
  if (!id) {
    setStatus(t('status.selectToDelete'))
    return
  }
  dialog.warning({
    title: t('dialog.confirmDeleteTitle'),
    content: t('dialog.confirmDeleteBody', { id }),
    positiveText: t('dialog.confirm'),
    negativeText: t('dialog.cancel'),
    onPositiveClick: async () => {
      try {
        await App.DeleteKey(id)
        await refreshKeys()
        setStatus(t('status.deleted', { id }))
      } catch (err) {
        message.error(String(err))
      }
    },
  })
}

async function generateKey() {
  try {
    importKey.value = await App.GenerateKey()
    setStatus(t('status.keyGenerated'))
  } catch (err) {
    setStatus(t('status.genKeyFailed', { err: String(err) }))
  }
}

async function doImport() {
  const chainID = importChainId.value.trim()
  const key = importKey.value.trim()
  if (!chainID) {
    setStatus(t('status.enterChainId'))
    return
  }
  if (!key) {
    setStatus(t('status.enterKey'))
    return
  }
  try {
    const res = (await App.ImportKey(chainID, key)) as {
      Owner: string
      EVMAddr: string
      Conflicts: string[] | null
    }
    importKey.value = ''
    await refreshKeys()
    let msg = t('status.savedKey', { owner: res.Owner, evm: res.EVMAddr, chain: chainID })
    if (res.Conflicts && res.Conflicts.length > 0) {
      msg += t('status.conflictSuffix', { ids: res.Conflicts.join(', ') })
      dialog.warning({
        title: t('dialog.conflictTitle'),
        content: msg,
        positiveText: t('dialog.confirm'),
      })
    }
    setStatus(msg)
  } catch (err) {
    message.error(String(err))
  }
}

async function init() {
  importChainId.value = props.defaultChainIds[0] || ''
  await refreshKeys()
}

onMounted(init)
</script>

<template>
  <div class="pane-body">
    <n-divider title-placement="left" class="section-divider">{{ t('tab.import') }}</n-divider>
    <n-form label-placement="top">
      <n-form-item :label="t('field.chainId')">
        <n-select
          v-model:value="importChainId"
          filterable
          tag
          :placeholder="t('ph.chainId')"
          :options="defaultChainIds.map((c) => ({ label: c, value: c }))"
        />
      </n-form-item>
      <n-form-item :label="t('field.privateKey')">
        <n-input-group>
          <n-input
            v-model:value="importKey"
            type="password"
            show-password-on="click"
            :placeholder="t('ph.key')"
          />
          <n-button type="default" @click="generateKey">{{ t('btn.genKey') }}</n-button>
        </n-input-group>
      </n-form-item>
    </n-form>
    <n-button type="primary" @click="doImport">{{ t('btn.import') }}</n-button>
    <n-text depth="3" class="hint">{{ t('hint.import') }}</n-text>

    <n-divider title-placement="left" class="section-divider">{{ t('tab.storedKeys') }}</n-divider>
    <n-data-table
      :columns="keyColumns"
      :data="entries"
      :row-key="(row: Entry) => row.ChainID"
      :row-props="(row: Entry) => ({ onClick: () => selectRow(row), class: row.ChainID === selectedChainId ? 'row-selected' : '' })"
      size="small"
      :max-height="360"
    />
    <n-space class="actions">
      <n-button @click="refreshKeys">{{ t('btn.refresh') }}</n-button>
      <n-button type="error" ghost @click="deleteSelected">{{ t('btn.delete') }}</n-button>
    </n-space>
  </div>
</template>
