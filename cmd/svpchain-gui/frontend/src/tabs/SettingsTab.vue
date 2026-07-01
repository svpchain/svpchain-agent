<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NButton,
  NInput,
  NForm,
  NFormItem,
  NCollapse,
  NCollapseItem,
  NPopover,
  NSwitch,
  NSelect,
  NRadioGroup,
  NRadioButton,
  NText,
  useMessage,
} from 'naive-ui'
import { setLocale } from '../i18n'
import * as App from '../../wailsjs/go/desktop/App'
import { desktop } from '../../wailsjs/go/models'
import type { Entry, SkillSetting } from '../types'

const props = defineProps<{
  entries: Entry[]
  tourExpandedSections?: string[]
}>()
const emit = defineEmits<{
  status: [msg: string]
  'restart-onboarding': []
}>()

const { t } = useI18n()
const message = useMessage()

const language = ref('en')
const llmApiKey = ref('')
const llmProvider = ref('openai')
const llmBaseURL = ref('')
const llmModel = ref('')
const remoteMCPURL = ref('')
const agentChainId = ref('')
const skillsConfigBase = ref('')
const defaultSkillsConfigBase = ref('')
const skillSettings = ref<SkillSetting[]>([])
const settingsExpandedSections = ref<string[]>([])
const showToolSteps = ref(false)
const agentRunLogDisabled = ref(false)

function setStatus(msg: string) {
  emit('status', msg)
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

async function onLanguageChange(lang: string) {
  language.value = lang
  setLocale(lang)
  await App.SetLanguage(lang)
}

async function loadAgentSettings() {
  try {
    const s = (await App.AgentGetSettings()) as {
      chain_id?: string
      llm_api_key?: string
      llm_provider?: string
      llm_base_url?: string
      llm_model?: string
      remote_mcp_url?: string
      skills_config_base?: string
      show_tool_steps?: boolean
      agent_run_log_disabled?: boolean
    }
    llmApiKey.value = s.llm_api_key || ''
    llmProvider.value = s.llm_provider || 'openai'
    llmBaseURL.value = s.llm_base_url || ''
    llmModel.value = s.llm_model || ''
    remoteMCPURL.value = s.remote_mcp_url || ''
    agentChainId.value = s.chain_id || ''
    showToolSteps.value = !!s.show_tool_steps
    agentRunLogDisabled.value = !!s.agent_run_log_disabled
    skillsConfigBase.value = s.skills_config_base || ''
    try {
      defaultSkillsConfigBase.value = await App.AgentDefaultSkillsConfigBase()
    } catch {
      defaultSkillsConfigBase.value = ''
    }
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
        llm_provider: llmProvider.value,
        llm_base_url: llmBaseURL.value,
        llm_model: llmModel.value,
        remote_mcp_url: remoteMCPURL.value,
        disabled_skills: disabledSkills,
        skills_config_base: skillsConfigBase.value.trim(),
        show_tool_steps: showToolSteps.value,
        agent_run_log_disabled: agentRunLogDisabled.value,
      } as Record<string, unknown>),
    )
    await loadSkillSettings()
    setStatus(t('status.settingsSaved'))
  } catch (err) {
    message.error(String(err))
  }
}

async function init() {
  language.value = await App.Language()
  await loadAgentSettings()
}

watch(
  () => props.entries,
  (list) => {
    if (!agentChainId.value && list.length > 0) {
      agentChainId.value = list[0].ChainID
    }
  },
  { immediate: true },
)

watch(
  () => props.tourExpandedSections,
  (sections) => {
    if (!sections?.length) return
    const merged = new Set([...settingsExpandedSections.value, ...sections])
    settingsExpandedSections.value = [...merged]
  },
  { immediate: true },
)

onMounted(init)
</script>

<template>
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
          <n-form-item>
            <template #label>
              <span class="label-with-help">
                <span>{{ t('field.showToolSteps') }}</span>
                <n-popover trigger="hover" placement="top-start" :show-arrow="true">
                  <template #trigger>
                    <span
                      class="help-icon"
                      tabindex="0"
                      role="button"
                      :title="t('hint.showToolSteps')"
                    >?</span>
                  </template>
                  <div class="help-tooltip-text">{{ t('hint.showToolSteps') }}</div>
                </n-popover>
              </span>
            </template>
            <n-switch v-model:value="showToolSteps" />
          </n-form-item>
          <n-form-item>
            <template #label>
              <span class="label-with-help">
                <span>{{ t('field.agentRunLog') }}</span>
                <n-popover trigger="hover" placement="top-start" :show-arrow="true">
                  <template #trigger>
                    <span
                      class="help-icon"
                      tabindex="0"
                      role="button"
                      :title="t('hint.agentRunLog')"
                    >?</span>
                  </template>
                  <div class="help-tooltip-text">{{ t('hint.agentRunLog') }}</div>
                </n-popover>
              </span>
            </template>
            <n-switch :value="!agentRunLogDisabled" @update:value="agentRunLogDisabled = !$event" />
          </n-form-item>
          <n-form-item>
            <template #label>
              <span class="label-with-help">
                <span>{{ t('field.skillsConfigBase') }}</span>
                <n-popover trigger="hover" placement="top-start" :show-arrow="true">
                  <template #trigger>
                    <span
                      class="help-icon"
                      tabindex="0"
                      role="button"
                      :title="t('hint.skillsConfigBase')"
                    >?</span>
                  </template>
                  <div class="help-tooltip-text">{{ t('hint.skillsConfigBase') }}</div>
                </n-popover>
              </span>
            </template>
            <n-input
              v-model:value="skillsConfigBase"
              :placeholder="defaultSkillsConfigBase || t('ph.skillsConfigBase')"
            />
          </n-form-item>
        </n-form>
      </n-collapse-item>

      <n-collapse-item :title="t('settings.section.llm')" name="llm">
        <n-form label-placement="top">
          <n-form-item :label="t('field.llmProvider')">
            <n-select
              v-model:value="llmProvider"
              :options="[
                { label: 'OpenAI-compatible', value: 'openai' },
                { label: 'Anthropic', value: 'anthropic' },
              ]"
            />
          </n-form-item>
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

    <div class="settings-actions">
      <n-button quaternary @click="emit('restart-onboarding')">{{ t('btn.restartOnboarding') }}</n-button>
      <n-button type="primary" class="settings-save" @click="saveAgentSettings">{{ t('btn.saveSettings') }}</n-button>
    </div>
  </div>
</template>

<style scoped>
.settings-collapse {
  margin-bottom: 12px;
}

.settings-collapse :deep(.n-collapse-item) {
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  overflow: hidden;
  margin-top: 0;
  background: var(--bg-elevated);
}

.settings-collapse :deep(.n-collapse-item + .n-collapse-item) {
  margin-top: 10px;
}

.settings-collapse :deep(.n-collapse-item__header) {
  display: flex;
  align-items: center;
  min-height: 44px;
  padding: 0 14px;
  font-weight: 600;
}

.settings-collapse :deep(.n-collapse-item__header-main) {
  display: flex;
  align-items: center;
  flex: 1;
  min-height: 44px;
  line-height: 1.4;
}

.settings-collapse :deep(.n-collapse-item-arrow) {
  display: flex;
  align-items: center;
  align-self: center;
}

.settings-collapse :deep(.n-collapse-item__content-inner) {
  padding: 0 14px 14px;
}

.settings-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-top: 4px;
}

.settings-save {
  flex-shrink: 0;
}

.skill-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin-bottom: 4px;
}

.skill-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 12px 14px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  background: var(--bg-surface);
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

.help-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  flex-shrink: 0;
  border-radius: 50%;
  border: 1px solid var(--border-default);
  background: var(--bg-hover);
  font-size: 11px;
  font-weight: 600;
  line-height: 1;
  color: var(--text-secondary);
  cursor: help;
  user-select: none;
}

.label-with-help {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  max-width: 100%;
}

.label-with-help :deep(.n-popover-trigger) {
  display: inline-flex;
  flex-shrink: 0;
}

.help-tooltip-text {
  display: inline-block;
  max-width: 280px;
  white-space: normal;
  line-height: 1.5;
}
</style>
