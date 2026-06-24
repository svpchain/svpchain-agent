<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { NTabs, NTabPane } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { setLocale } from './i18n'
import * as App from '../wailsjs/go/desktop/App'
import { WindowToggleMaximise } from '../wailsjs/runtime/runtime'
import AssistantTab from './tabs/AssistantTab.vue'
import ConfigTab from './tabs/ConfigTab.vue'
import KeysTab from './tabs/KeysTab.vue'
import SecurityTab from './tabs/SecurityTab.vue'
import SettingsTab from './tabs/SettingsTab.vue'
import AboutTab from './tabs/AboutTab.vue'
import UpdateModals from './UpdateModals.vue'
import type { Entry } from './types'

const { t } = useI18n()

const isMac = /Mac|iPhone|iPad|iPod/i.test(navigator.userAgent)

const activeTab = ref('assistant')
const status = ref('')
const entries = ref<Entry[]>([])
const defaultChainIds = ref<string[]>([])

const updateModalsRef = ref<InstanceType<typeof UpdateModals> | null>(null)

function setStatus(msg: string) {
  status.value = msg
}

watch(activeTab, () => {
  status.value = ''
})

function toggleMaximise() {
  WindowToggleMaximise()
}

onMounted(async () => {
  const lang = await App.Language()
  setLocale(lang)
  defaultChainIds.value = (await App.DefaultChainIDs()) || []
  await updateModalsRef.value?.maybeCheckUpdate()
})
</script>

<template>
  <div
    class="titlebar-drag titlebar"
    :class="{ 'titlebar--mac': isMac }"
    @dblclick="toggleMaximise"
  >
    svpchain agent
  </div>

  <n-tabs v-model:value="activeTab" type="line" class="tabs" pane-class="pane">
    <n-tab-pane name="assistant" :tab="t('tab.assistant')" display-directive="show">
      <AssistantTab :entries="entries" @status="setStatus" />
    </n-tab-pane>

    <n-tab-pane name="config" :tab="t('tab.config')">
      <ConfigTab :entries="entries" @status="setStatus" />
    </n-tab-pane>

    <n-tab-pane name="keys" :tab="t('tab.keys')">
      <KeysTab
        :entries="entries"
        :default-chain-ids="defaultChainIds"
        @status="setStatus"
        @update:entries="entries = $event"
      />
    </n-tab-pane>

    <n-tab-pane name="security" :tab="t('tab.security')">
      <SecurityTab :default-chain-ids="defaultChainIds" @status="setStatus" />
    </n-tab-pane>

    <n-tab-pane name="settings" :tab="t('tab.settings')">
      <SettingsTab :entries="entries" @status="setStatus" />
    </n-tab-pane>

    <n-tab-pane name="about" :tab="t('tab.about')">
      <AboutTab />
    </n-tab-pane>
  </n-tabs>

  <div class="statusbar">{{ status }}</div>

  <UpdateModals ref="updateModalsRef" />
</template>

<style scoped>
.titlebar {
  flex-shrink: 0;
  height: 28px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 600;
  font-size: 13px;
  color: #555;
  border-bottom: 1px solid #ececec;
  user-select: none;
  box-sizing: border-box;
}

.titlebar--mac {
  padding-left: 70px;
}

.tabs {
  flex: 1;
  min-height: 0;
  padding: 0 16px;
}

.tabs :deep(.n-tab-pane) {
  flex: 1;
  min-height: 0;
}

.statusbar {
  height: 28px;
  line-height: 28px;
  padding: 0 16px;
  font-size: 12px;
  color: #888;
  border-top: 1px solid #ececec;
}
</style>
