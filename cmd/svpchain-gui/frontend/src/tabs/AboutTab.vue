<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import * as App from '../../wailsjs/go/desktop/App'

const { t } = useI18n()
const version = ref('')

onMounted(async () => {
  version.value = await App.CurrentVersion()
})
</script>

<template>
  <div class="about-pane">
    <div class="about-card">
      <div class="about-header">
        <span class="about-logo">S</span>
        <div>
          <h2 class="about-title">{{ t('about.title') }}</h2>
          <p class="about-version">{{ t('about.version', { v: version }) }}</p>
        </div>
      </div>
      <div class="about-body">{{ t('about.body') }}</div>
    </div>
  </div>
</template>

<style scoped>
.about-pane {
  width: 100%;
  padding-bottom: 24px;
}

.about-card {
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
  padding: 24px 28px;
}

.about-header {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 20px;
  padding-bottom: 20px;
  border-bottom: 1px solid var(--border-subtle);
}

.about-logo {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  border-radius: 12px;
  background: linear-gradient(135deg, var(--accent) 0%, #0d8c6d 100%);
  color: #fff;
  font-size: 20px;
  font-weight: 700;
  flex-shrink: 0;
}

.about-title {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  letter-spacing: -0.02em;
  color: var(--text-primary);
}

.about-version {
  margin: 4px 0 0;
  font-size: 12px;
  color: var(--text-muted);
}

.about-body {
  white-space: pre-line;
  line-height: 1.7;
  font-size: 13px;
  color: var(--text-secondary);
  word-break: break-word;
}
</style>
