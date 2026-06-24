<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NModal, NCard, NButton, NSpace, NProgress } from 'naive-ui'
import * as App from '../wailsjs/go/desktop/App'
import { EventsOn, EventsOff } from '../wailsjs/runtime/runtime'
import type { UpdateInfo } from './types'

const { t } = useI18n()

const updateInfo = ref<UpdateInfo | null>(null)
const showAvailable = ref(false)
const showProgress = ref(false)
const showReady = ref(false)
const showFailed = ref(false)
const progressPercent = ref(0)
const progressStage = ref<'downloading' | 'verifying' | 'extracting'>('downloading')
const stagedApp = ref('')
const failedErr = ref('')

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

defineExpose({ maybeCheckUpdate })
</script>

<template>
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

  <n-modal v-model:show="showProgress" :mask-closable="false" :closable="false">
    <n-card style="width: 440px" :title="t('update.downloadingTitle')">
      <p>{{ t('update.' + progressStage) }}</p>
      <n-progress type="line" :percentage="progressPercent" :indicator-placement="'inside'" processing />
    </n-card>
  </n-modal>

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
.failed-body {
  white-space: pre-line;
}
</style>
