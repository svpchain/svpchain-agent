<script setup lang="ts">
import { h, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NTabs,
  NTabPane,
  NButton,
  NInput,
  NInputGroup,
  NSelect,
  NRadioGroup,
  NRadioButton,
  NForm,
  NFormItem,
  NDataTable,
  NDivider,
  NCollapse,
  NCollapseItem,
  NSpace,
  NSwitch,
  NModal,
  NCard,
  NProgress,
  NText,
  useMessage,
  useDialog,
  type DataTableColumns,
} from 'naive-ui'
import { setLocale } from './i18n'
import AgentView from './AgentView.vue'
import * as App from '../wailsjs/go/desktop/App'
import { desktop } from '../wailsjs/go/models'
import { EventsOn, EventsOff, WindowToggleMaximise } from '../wailsjs/runtime/runtime'

const { t } = useI18n()
const message = useMessage()
const dialog = useDialog()

type Entry = { ChainID: string; Owner: string; EVMAddr: string }
type WhitelistEntry = { ChainID: string; AddressType: string; Address: string }
type SkillSetting = {
  name: string
  description: string
  enabled: boolean
  locked: boolean
  source: string
}
type UpdateInfo = {
  Current: string
  Latest: string
  TagName: string
  ReleaseURL: string
}

const activeTab = ref('assistant')
const status = ref('')

// keys
const entries = ref<Entry[]>([])
const selectedChainId = ref<string | null>(null)

// import
const defaultChainIds = ref<string[]>([])
const importChainId = ref('')
const importKey = ref('')

// security / whitelist
const whitelistEntries = ref<WhitelistEntry[]>([])
const selectedWhitelist = ref<WhitelistEntry | null>(null)
const whitelistChainId = ref('')
const whitelistAddressType = ref('cosmos')
const whitelistAddress = ref('')

const addressTypeOptions = [
  { label: () => t('addressType.cosmos'), value: 'cosmos' },
  { label: () => t('addressType.evm'), value: 'evm' },
]

// config
const agents = ref<string[]>([])
const agent = ref('')
const signerPath = ref('')
const configPreview = ref('')

// settings
const language = ref('en')
const llmApiKey = ref('')
const llmBaseURL = ref('')
const llmModel = ref('')
const remoteMCPURL = ref('')
const agentChainId = ref('')
const skillSettings = ref<SkillSetting[]>([])
const settingsExpandedSections = ref(['basic', 'llm', 'skills'])

// update
const version = ref('')
const updateInfo = ref<UpdateInfo | null>(null)
const showAvailable = ref(false)
const showProgress = ref(false)
const showReady = ref(false)
const showFailed = ref(false)
const progressPercent = ref(0)
const progressStage = ref<'downloading' | 'verifying' | 'extracting'>('downloading')
const stagedApp = ref('')
const failedErr = ref('')

function setStatus(msg: string) {
  status.value = msg
}

// The status bar is shared across all tabs; clear it on tab switch so a message
// from one tab (e.g. "config generated") doesn't linger on the others.
watch(activeTab, () => {
  status.value = ''
})

// Restore the native macOS "double-click the title bar to zoom" gesture, which the
// custom hidden-inset (draggable) title bar otherwise swallows.
function toggleMaximise() {
  WindowToggleMaximise()
}

function copyAddress(addr: string) {
  if (!addr) return
  App.CopyText(addr)
  setStatus(t('status.addressCopied'))
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

function addressCell(addr: string) {
  return h('div', { class: 'addr-cell' }, [
    h('span', { class: 'addr-text', title: addr }, addr),
    h(
      NButton,
      { size: 'tiny', quaternary: true, onClick: () => copyAddress(addr) },
      { default: () => t('btn.copyShort') },
    )
  ])
}

function normalizeWhitelistEntry(row: WhitelistEntry & {
  chain_id?: string
  address_type?: string
  address?: string
}): WhitelistEntry {
  return {
    ChainID: row.ChainID ?? row.chain_id ?? '',
    AddressType: row.AddressType ?? row.address_type ?? '',
    Address: row.Address ?? row.address ?? '',
  }
}

function whitelistKey(row: WhitelistEntry) {
  return `${row.ChainID}|${row.AddressType}|${row.Address}`
}

function addressTypeLabel(type: string) {
  return type === 'evm' ? t('addressType.evm') : t('addressType.cosmos')
}

const whitelistColumns: DataTableColumns<WhitelistEntry> = [
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
      (await App.AddWhitelist(chainID, whitelistAddressType.value, address)) as WhitelistEntry,
    )
    whitelistAddress.value = ''
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

async function refreshKeys() {
  try {
    const list = (await App.ListKeys()) as Entry[]
    entries.value = list || []
    selectedChainId.value = null
    if (entries.value.length === 0) {
      setStatus(t('status.noKeys'))
    } else {
      setStatus(t('status.keyCount', { n: entries.value.length }))
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

async function generateConfig() {
  const chainID = (selectedChainId.value || '').trim()
  if (!chainID) {
    setStatus(t('status.selectChainId'))
    return
  }
  if (!agent.value) {
    setStatus(t('status.selectAgent'))
    return
  }
  try {
    configPreview.value = await App.GenerateConfig(agent.value, chainID, signerPath.value)
    setStatus(t('status.configGenerated'))
  } catch (err) {
    setStatus(String(err))
  }
}

function copyConfig() {
  if (!configPreview.value) {
    setStatus(t('status.generateFirst'))
    return
  }
  App.CopyText(configPreview.value)
  setStatus(t('status.copied'))
}

async function browseSigner() {
  try {
    const p = await App.BrowseSignerBinary()
    if (p) signerPath.value = p
  } catch {
    /* user cancelled */
  }
}

async function onLanguageChange(lang: string) {
  language.value = lang
  setLocale(lang)
  await App.SetLanguage(lang)
}

function skillLabel(name: string): string {
  const key = `skill.names.${name}`
  const translated = t(key)
  return translated === key ? name : translated
}

async function loadSkillSettings() {
  try {
    const rows = (await App.AgentListSkills()) as SkillSetting[]
    skillSettings.value = rows.map((row) => ({
      name: row.name ?? (row as unknown as { Name?: string }).Name ?? '',
      description: row.description ?? (row as unknown as { Description?: string }).Description ?? '',
      enabled: row.enabled ?? (row as unknown as { Enabled?: boolean }).Enabled ?? true,
      locked: row.locked ?? (row as unknown as { Locked?: boolean }).Locked ?? false,
      source: row.source ?? (row as unknown as { Source?: string }).Source ?? '',
    }))
  } catch {
    skillSettings.value = []
  }
}

async function loadAgentSettings() {
  try {
    const s = (await App.AgentGetSettings()) as {
      chain_id?: string
      llm_api_key?: string
      llm_base_url?: string
      llm_model?: string
      remote_mcp_url?: string
    }
    llmApiKey.value = s.llm_api_key || ''
    llmBaseURL.value = s.llm_base_url || ''
    llmModel.value = s.llm_model || ''
    remoteMCPURL.value = s.remote_mcp_url || ''
    agentChainId.value = s.chain_id || ''
    if (!remoteMCPURL.value) {
      remoteMCPURL.value = await App.AgentDefaultRemoteURL()
    }
    await loadSkillSettings()
  } catch {
    /* bindings not generated yet */
  }
}

async function saveAgentSettings() {
  try {
    const disabledSkills = skillSettings.value
      .filter((s) => !s.enabled && !s.locked)
      .map((s) => s.name)
    await App.AgentSetSettings(
      desktop.AgentSettings.createFrom({
        chain_id: agentChainId.value,
        llm_api_key: llmApiKey.value,
        llm_base_url: llmBaseURL.value,
        llm_model: llmModel.value,
        remote_mcp_url: remoteMCPURL.value,
        disabled_skills: disabledSkills,
      } as Record<string, unknown>),
    )
    setStatus(t('status.settingsSaved'))
  } catch (err) {
    message.error(String(err))
  }
}

// ----- update flow -----

async function maybeCheckUpdate() {
  try {
    if (!(await App.UpdateEnabled())) return
    const info = (await App.CheckUpdate()) as UpdateInfo | null
    if (info && info.Latest) {
      updateInfo.value = info
      showAvailable.value = true
    }
  } catch {
    /* silent */
  }
}

function onUpgrade() {
  showAvailable.value = false
  startUpdate()
}

function onLater() {
  showAvailable.value = false
}

async function onSkip() {
  if (updateInfo.value) await App.SkipVersion(updateInfo.value.TagName)
  showAvailable.value = false
}

async function startUpdate() {
  progressPercent.value = 0
  progressStage.value = 'downloading'
  showProgress.value = true
  const handler = (e: { percent: number; stage: 'downloading' | 'verifying' | 'extracting' }) => {
    progressPercent.value = Math.round((e.percent || 0) * 100)
    if (e.stage) progressStage.value = e.stage
  }
  EventsOn('update:progress', handler)
  try {
    const staged = await App.StartUpdate(updateInfo.value as any)
    showProgress.value = false
    stagedApp.value = staged
    showReady.value = true
  } catch (err) {
    showProgress.value = false
    failedErr.value = String(err)
    showFailed.value = true
  } finally {
    EventsOff('update:progress')
  }
}

async function onInstall() {
  try {
    await App.InstallUpdate(stagedApp.value)
  } catch (err) {
    showReady.value = false
    failedErr.value = String(err)
    showFailed.value = true
  }
}

function onOpenRelease() {
  if (updateInfo.value) App.OpenURL(updateInfo.value.ReleaseURL)
  showFailed.value = false
}

onMounted(async () => {
  language.value = await App.Language()
  setLocale(language.value)
  version.value = await App.CurrentVersion()

  defaultChainIds.value = (await App.DefaultChainIDs()) || []
  importChainId.value = defaultChainIds.value[0] || ''
  whitelistChainId.value = defaultChainIds.value[0] || ''

  agents.value = (await App.AgentNames()) || []
  agent.value = await App.DefaultAgent()
  signerPath.value = await App.GuessSignerPath()

  await refreshKeys()
  await refreshWhitelist()
  await loadAgentSettings()
  if (!agentChainId.value && entries.value.length > 0) {
    agentChainId.value = entries.value[0].ChainID
  }
  await maybeCheckUpdate()
})
</script>

<template>
  <div class="titlebar-drag titlebar" @dblclick="toggleMaximise">svpchain agent</div>

  <n-tabs v-model:value="activeTab" type="line" class="tabs" pane-class="pane">
    <!-- Assistant -->
    <n-tab-pane name="assistant" :tab="t('tab.assistant')" display-directive="show">
      <div class="pane-body assistant-tab">
        <AgentView :entries="entries" @status="setStatus" />
      </div>
    </n-tab-pane>

    <!-- Config -->
    <n-tab-pane name="config" :tab="t('tab.config')">
      <div class="pane-body">
        <n-form label-placement="top">
          <n-form-item :label="t('field.chainId')">
            <n-select
              v-model:value="selectedChainId"
              :placeholder="t('ph.chainConfig')"
              :options="entries.map((e) => ({ label: e.ChainID, value: e.ChainID }))"
            />
          </n-form-item>
          <n-form-item :label="t('field.signerPath')">
            <n-input-group>
              <n-input v-model:value="signerPath" :placeholder="t('ph.binary')" />
              <n-button @click="browseSigner">{{ t('btn.browse') }}</n-button>
            </n-input-group>
          </n-form-item>
          <n-form-item :label="t('field.agent')">
            <n-radio-group v-model:value="agent">
              <n-radio-button v-for="a in agents" :key="a" :value="a">{{ a }}</n-radio-button>
            </n-radio-group>
          </n-form-item>
        </n-form>
        <n-space>
          <n-button type="primary" @click="generateConfig">{{ t('btn.generate') }}</n-button>
          <n-button @click="copyConfig">{{ t('btn.copy') }}</n-button>
        </n-space>
        <n-text depth="3" class="hint">{{ t('hint.config') }}</n-text>
        <pre v-if="configPreview" class="preview">{{ configPreview }}</pre>
      </div>
    </n-tab-pane>

    <!-- Keys: import (top) + stored (bottom) -->
    <n-tab-pane name="keys" :tab="t('tab.keys')">
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
    </n-tab-pane>

    <!-- Security: whitelist -->
    <n-tab-pane name="security" :tab="t('tab.security')">
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
    </n-tab-pane>

    <!-- Settings -->
    <n-tab-pane name="settings" :tab="t('tab.settings')">
      <div class="pane-body">
        <n-collapse
          v-model:expanded-names="settingsExpandedSections"
          class="settings-collapse"
        >
          <n-collapse-item :title="t('settings.section.basic')" name="basic">
            <n-form label-placement="top">
              <n-form-item :label="t('field.language')">
                <n-radio-group :value="language" @update:value="onLanguageChange">
                  <n-radio-button value="zh">{{ t('lang.chinese') }}</n-radio-button>
                  <n-radio-button value="en">{{ t('lang.english') }}</n-radio-button>
                </n-radio-group>
              </n-form-item>
              <n-form-item :label="t('field.chainId')">
                <n-select
                  v-model:value="agentChainId"
                  :placeholder="t('ph.chainConfig')"
                  :options="entries.map((e) => ({ label: e.ChainID, value: e.ChainID }))"
                />
              </n-form-item>
            </n-form>
          </n-collapse-item>

          <n-collapse-item :title="t('settings.section.llm')" name="llm">
            <n-form label-placement="top">
              <n-form-item :label="t('field.llmApiKey')">
                <n-input
                  v-model:value="llmApiKey"
                  type="password"
                  show-password-on="click"
                  :placeholder="t('ph.llmApiKey')"
                />
              </n-form-item>
              <n-form-item :label="t('field.llmBaseURL')">
                <n-input v-model:value="llmBaseURL" :placeholder="t('ph.llmBaseURL')" />
              </n-form-item>
              <n-form-item :label="t('field.llmModel')">
                <n-input v-model:value="llmModel" :placeholder="t('ph.llmModel')" />
              </n-form-item>
              <n-form-item :label="t('field.remoteMCPURL')">
                <n-input v-model:value="remoteMCPURL" :placeholder="t('ph.remoteMCPURL')" />
              </n-form-item>
            </n-form>
            <n-text depth="3" class="hint">{{ t('hint.assistantSettings') }}</n-text>
          </n-collapse-item>

          <n-collapse-item :title="t('skill.section')" name="skills">
            <div v-if="skillSettings.length" class="skill-list">
              <div v-for="skill in skillSettings" :key="skill.name" class="skill-row">
                <div class="skill-copy">
                  <n-text strong>{{ skillLabel(skill.name) }}</n-text>
                  <n-text depth="3" tag="div" class="skill-desc">{{ skill.description }}</n-text>
                </div>
                <n-switch v-model:value="skill.enabled" :disabled="skill.locked" />
              </div>
            </div>
            <n-text v-else depth="3" class="hint">{{ t('skill.empty') }}</n-text>
            <n-text depth="3" class="hint">{{ t('hint.skills') }}</n-text>
          </n-collapse-item>
        </n-collapse>

        <n-button type="primary" class="settings-save" @click="saveAgentSettings">{{ t('btn.saveSettings') }}</n-button>
      </div>
    </n-tab-pane>

    <!-- About -->
    <n-tab-pane name="about" :tab="t('tab.about')">
      <div class="pane-body">
        <h3>{{ t('about.title') }}</h3>
        <p class="about-body">{{ t('about.body') }}</p>
        <n-text depth="3">{{ t('about.version', { v: version }) }}</n-text>
      </div>
    </n-tab-pane>
  </n-tabs>

  <div class="statusbar">{{ status }}</div>

  <!-- Update available -->
  <n-modal v-model:show="showAvailable" :mask-closable="false">
    <n-card style="width: 440px" :title="t('update.availableTitle')">
      <p>{{ t('update.availableBody', { current: updateInfo?.Current, latest: updateInfo?.Latest }) }}</p>
      <template #footer>
        <n-space justify="end">
          <n-button quaternary @click="onSkip">{{ t('update.skip', { tag: updateInfo?.TagName }) }}</n-button>
          <n-button @click="onLater">{{ t('update.later') }}</n-button>
          <n-button type="primary" @click="onUpgrade">{{ t('update.upgrade') }}</n-button>
        </n-space>
      </template>
    </n-card>
  </n-modal>

  <!-- Progress -->
  <n-modal v-model:show="showProgress" :mask-closable="false" :closable="false">
    <n-card style="width: 440px" :title="t('update.downloadingTitle')">
      <p>{{ t('update.' + progressStage) }}</p>
      <n-progress type="line" :percentage="progressPercent" :indicator-placement="'inside'" processing />
    </n-card>
  </n-modal>

  <!-- Ready -->
  <n-modal v-model:show="showReady" :mask-closable="false">
    <n-card style="width: 440px" :title="t('update.readyTitle')">
      <p>{{ t('update.readyBody') }}</p>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showReady = false">{{ t('update.cancel') }}</n-button>
          <n-button type="primary" @click="onInstall">{{ t('update.install') }}</n-button>
        </n-space>
      </template>
    </n-card>
  </n-modal>

  <!-- Failed -->
  <n-modal v-model:show="showFailed" :mask-closable="false">
    <n-card style="width: 440px" :title="t('update.failedTitle')">
      <p class="failed-body">{{ t('update.failedBody', { err: failedErr }) }}</p>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showFailed = false">{{ t('update.later') }}</n-button>
          <n-button type="primary" @click="onOpenRelease">{{ t('update.openRelease') }}</n-button>
        </n-space>
      </template>
    </n-card>
  </n-modal>
</template>

<style scoped>
.titlebar {
  height: 36px;
  line-height: 36px;
  text-align: center;
  font-weight: 600;
  font-size: 13px;
  color: #555;
  border-bottom: 1px solid #ececec;
  user-select: none;
}

.tabs {
  flex: 1;
  min-height: 0;
  padding: 0 16px;
}

/* naive-ui renders .n-tab-pane at content height, leaving empty space below.
   Make the active pane fill the tab area so panes (e.g. the assistant input row)
   can pin to the bottom. */
.tabs :deep(.n-tab-pane) {
  flex: 1;
  min-height: 0;
}

.pane-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
  height: 100%;
  overflow-y: auto;
}

.section-divider {
  margin: 4px 0 !important;
}

.actions {
  padding-top: 4px;
}

.hint {
  font-size: 12px;
}

.skill-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin-bottom: 4px;
}

.settings-collapse {
  margin-bottom: 12px;
}

.settings-collapse :deep(.n-collapse-item) {
  border: 1px solid #ececec;
  border-radius: 10px;
  overflow: hidden;
  margin-top: 0;
}

.settings-collapse :deep(.n-collapse-item + .n-collapse-item) {
  margin-top: 10px;
}

.settings-collapse :deep(.n-collapse-item__header) {
  padding: 10px 14px;
  font-weight: 600;
}

.settings-collapse :deep(.n-collapse-item__content-inner) {
  padding: 0 14px 14px;
}

.settings-save {
  margin-top: 4px;
}

.skill-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 10px 12px;
  border: 1px solid #ececec;
  border-radius: 8px;
}

.skill-copy {
  min-width: 0;
  flex: 1;
}

.skill-desc {
  margin-top: 4px;
  font-size: 12px;
  line-height: 1.4;
}

.preview {
  flex: 1;
  overflow: auto;
  background: #f6f7f9;
  border: 1px solid #ececec;
  border-radius: 12px;
  padding: 12px;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
  white-space: pre;
  margin: 0;
}

.about-body {
  white-space: pre-line;
  line-height: 1.6;
  color: #444;
}

.failed-body {
  white-space: pre-line;
}

.statusbar {
  height: 28px;
  line-height: 28px;
  padding: 0 16px;
  font-size: 12px;
  color: #888;
  border-top: 1px solid #ececec;
}

:deep(.addr-cell) {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

:deep(.addr-text) {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

:deep(.row-selected td) {
  background-color: rgba(24, 160, 88, 0.1) !important;
}

.assistant-tab {
  min-height: 380px;
}
</style>
