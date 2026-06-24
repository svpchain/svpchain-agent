<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NText } from 'naive-ui'
import * as App from '../../wailsjs/go/desktop/App'

const { t } = useI18n()
const version = ref('')

onMounted(async () => {
  version.value = await App.CurrentVersion()
})
</script>

<template>
  <div class="pane-body">
    <h3>{{ t('about.title') }}</h3>
    <p class="about-body">{{ t('about.body') }}</p>
    <n-text depth="3">{{ t('about.version', { v: version }) }}</n-text>
  </div>
</template>

<style scoped>
.about-body {
  white-space: pre-line;
  line-height: 1.6;
  color: #444;
}
</style>
