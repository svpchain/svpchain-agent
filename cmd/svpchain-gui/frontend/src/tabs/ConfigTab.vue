<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NButton,
  NInput,
  NInputGroup,
  NForm,
  NFormItem,
  NRadioGroup,
  NRadioButton,
  NSpace,
  NSelect,
  NText,
} from 'naive-ui'
import * as App from '../../wailsjs/go/desktop/App'
import type { Entry } from '../types'

const props = defineProps<{ entries: Entry[] }>()
const emit = defineEmits<{ status: [msg: string] }>()

const { t } = useI18n()

const selectedChainId = ref<string | null>(null)
const agents = ref<string[]>([])
const agent = ref('')
const signerPath = ref('')
const configPreview = ref('')

function setStatus(msg: string) {
  emit('status', msg)
}

async function init() {
  agents.value = (await App.AgentNames()) || []
  agent.value = await App.DefaultAgent()
  signerPath.value = await App.GuessSignerPath()
}

watch(
  () => props.entries,
  (list) => {
    if (!selectedChainId.value && list.length > 0) {
      selectedChainId.value = list[0].ChainID
    }
  },
  { immediate: true },
)

onMounted(init)

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

</script>

<template>
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
</template>

<style scoped>
.preview {
  flex: 1;
  overflow: auto;
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  padding: 14px 16px;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
  color: var(--text-secondary);
  white-space: pre;
  margin: 0;
}
</style>
