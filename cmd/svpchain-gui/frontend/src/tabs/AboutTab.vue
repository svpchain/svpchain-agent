<script setup lang="ts">
import {computed, onMounted, ref} from 'vue'
import {useI18n} from 'vue-i18n'
import * as App from '../../wailsjs/go/desktop/App'

const {t, tm} = useI18n()
const version = ref('')

const startSteps = computed(() => {
  const raw = tm('about.start.steps')
  return Array.isArray(raw) ? (raw as string[]) : []
})

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
          <p class="about-version">{{ t('about.version', {v: version}) }}</p>
        </div>
      </div>

      <p class="about-lead">{{ t('about.lead') }}</p>

      <section class="about-section">
        <h3>{{ t('about.trust.title') }}</h3>
        <ul>
          <li>
            <span class="about-kicker">{{ t('about.trust.localTitle') }}</span>
            {{ t('about.trust.local') }}
          </li>
          <li>
            <span class="about-kicker">{{ t('about.trust.remoteTitle') }}</span>
            {{ t('about.trust.remote') }}
          </li>
          <li>
            <span class="about-kicker">{{ t('about.trust.assistantTitle') }}</span>
            {{ t('about.trust.assistant') }}
          </li>
        </ul>
      </section>

      <section class="about-section">
        <h3>{{ t('about.canDo.title') }}</h3>
        <ul>
          <li>
            <span class="about-kicker">{{ t('about.canDo.orchestrateTitle') }}</span>
            {{ t('about.canDo.orchestrate') }}
          </li>
          <li>
            <span class="about-kicker">{{ t('about.canDo.directTitle') }}</span>
            {{ t('about.canDo.direct') }}
          </li>
          <li>
            <span class="about-kicker">{{ t('about.canDo.keysTitle') }}</span>
            {{ t('about.canDo.keys') }}
          </li>
          <li>
            <span class="about-kicker">{{ t('about.canDo.securityTitle') }}</span>
            {{ t('about.canDo.security') }}
          </li>
          <li>
            <span class="about-kicker">{{ t('about.canDo.mcpTitle') }}</span>
            {{ t('about.canDo.mcp') }}
          </li>
        </ul>
      </section>

      <section class="about-section">
        <h3>{{ t('about.safety.title') }}</h3>
        <ul>
          <li>{{ t('about.safety.whitelist') }}</li>
          <li>{{ t('about.safety.confirm') }}</li>
          <li>{{ t('about.safety.a2a') }}</li>
        </ul>
      </section>

      <section class="about-section">
        <h3>{{ t('about.start.title') }}</h3>
        <ol>
          <li v-for="(step, i) in startSteps" :key="i">{{ step }}</li>
        </ol>
      </section>
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

.about-lead {
  margin: 0 0 24px;
  line-height: 1.7;
  font-size: 14px;
  color: var(--text-secondary);
}

.about-section {
  margin-bottom: 22px;
}

.about-section:last-child {
  margin-bottom: 0;
}

.about-section h3 {
  margin: 0 0 10px;
  font-size: 13px;
  font-weight: 600;
  letter-spacing: -0.01em;
  color: var(--text-primary);
}

.about-section ul,
.about-section ol {
  margin: 0;
  padding-left: 18px;
  line-height: 1.7;
  font-size: 13px;
  color: var(--text-secondary);
}

.about-section li + li {
  margin-top: 8px;
}

.about-kicker {
  color: var(--text-primary);
  font-weight: 600;
}
</style>
