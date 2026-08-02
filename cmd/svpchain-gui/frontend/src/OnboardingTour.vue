<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NSpace } from 'naive-ui'
import * as App from '../wailsjs/go/desktop/App'

type TabId = 'keys' | 'settings' | 'assistant'

interface TourStep {
  target: string
  tab: TabId
  expandSettings?: string[]
  titleKey: string
  descKey: string
  placement: 'right' | 'bottom'
}

const { t } = useI18n()

const emit = defineEmits<{
  'step-change': [payload: { tab: TabId; expandSettings?: string[] }]
  'ensure-sidebar': []
}>()

const steps: TourStep[] = [
  {
    target: '[data-tour="nav-keys"]',
    tab: 'keys',
    titleKey: 'onboarding.step1Title',
    descKey: 'onboarding.step1Desc',
    placement: 'right',
  },
  {
    target: '[data-tour="nav-settings"]',
    tab: 'settings',
    expandSettings: ['basic', 'llm'],
    titleKey: 'onboarding.step2Title',
    descKey: 'onboarding.step2Desc',
    placement: 'right',
  },
  {
    target: '[data-tour="assistant-chain"]',
    tab: 'assistant',
    titleKey: 'onboarding.step3Title',
    descKey: 'onboarding.step3Desc',
    placement: 'bottom',
  },
]

const active = ref(false)
const stepIndex = ref(0)
const spotlight = ref({ top: 0, left: 0, width: 0, height: 0 })
const tooltip = ref({ top: 0, left: 0 })

const currentStep = computed(() => steps[stepIndex.value])
const isLastStep = computed(() => stepIndex.value >= steps.length - 1)
const stepLabel = computed(() =>
  t('onboarding.stepOf', { current: stepIndex.value + 1, total: steps.length }),
)

let resizeObserver: ResizeObserver | null = null
let retryTimer: ReturnType<typeof setTimeout> | null = null

function clearRetry() {
  if (retryTimer) {
    clearTimeout(retryTimer)
    retryTimer = null
  }
}

function spotlightStyle() {
  const s = spotlight.value
  return {
    top: `${s.top}px`,
    left: `${s.left}px`,
    width: `${s.width}px`,
    height: `${s.height}px`,
  }
}

function tooltipStyle() {
  const p = tooltip.value
  return {
    top: `${p.top}px`,
    left: `${p.left}px`,
  }
}

function positionTooltip(rect: DOMRect, placement: 'right' | 'bottom') {
  const gap = 14
  const maxW = 320
  const vw = window.innerWidth
  const vh = window.innerHeight

  if (placement === 'right') {
    let left = rect.right + gap
    let top = rect.top
    if (left + maxW > vw - 16) {
      left = Math.max(16, rect.left - gap - maxW)
    }
    top = Math.min(Math.max(16, top), vh - 200)
    tooltip.value = { top, left }
    return
  }

  let top = rect.bottom + gap
  let left = rect.left
  if (top + 180 > vh - 16) {
    top = Math.max(16, rect.top - gap - 180)
  }
  left = Math.min(Math.max(16, left), vw - maxW - 16)
  tooltip.value = { top, left }
}

function updatePosition(retry = 0) {
  if (!active.value) return
  const step = currentStep.value
  const el = document.querySelector(step.target) as HTMLElement | null
  if (!el) {
    if (retry < 8) {
      clearRetry()
      retryTimer = setTimeout(() => updatePosition(retry + 1), 80)
    }
    return
  }

  el.scrollIntoView({ block: 'nearest', inline: 'nearest', behavior: 'instant' })
  const rect = el.getBoundingClientRect()
  const pad = 6
  spotlight.value = {
    top: rect.top - pad,
    left: rect.left - pad,
    width: rect.width + pad * 2,
    height: rect.height + pad * 2,
  }
  positionTooltip(rect, step.placement)
}

async function applyStep(index: number) {
  stepIndex.value = index
  const step = steps[index]
  emit('ensure-sidebar')
  emit('step-change', { tab: step.tab, expandSettings: step.expandSettings })
  await nextTick()
  clearRetry()
  retryTimer = setTimeout(() => updatePosition(0), 120)
}

async function maybeShow() {
  try {
    if (await App.OnboardingDone()) return
  } catch {
    /* bindings not ready */
  }
  await restart()
}

async function restart() {
  active.value = true
  await applyStep(0)
}

async function finish() {
  try {
    await App.CompleteOnboarding()
  } catch {
    /* ignore */
  }
  active.value = false
  clearRetry()
}

function skip() {
  finish()
}

function next() {
  if (isLastStep.value) {
    finish()
    return
  }
  applyStep(stepIndex.value + 1)
}

function prev() {
  if (stepIndex.value <= 0) return
  applyStep(stepIndex.value - 1)
}

function onWindowChange() {
  updatePosition()
}

onMounted(() => {
  window.addEventListener('resize', onWindowChange)
  window.addEventListener('scroll', onWindowChange, true)
  resizeObserver = new ResizeObserver(onWindowChange)
  resizeObserver.observe(document.body)
})

onUnmounted(() => {
  window.removeEventListener('resize', onWindowChange)
  window.removeEventListener('scroll', onWindowChange, true)
  resizeObserver?.disconnect()
  clearRetry()
})

defineExpose({ maybeShow, restart })
</script>

<template>
  <Teleport to="body">
    <div v-if="active" class="tour-root" role="presentation">
      <div class="tour-backdrop" aria-hidden="true" />
      <div class="tour-spotlight" :style="spotlightStyle()" aria-hidden="true" />
      <div
        class="tour-popover"
        :style="tooltipStyle()"
        role="dialog"
        aria-modal="true"
        :aria-label="t(currentStep.titleKey)"
      >
        <div class="tour-step-label">{{ stepLabel }}</div>
        <div class="tour-title">{{ t(currentStep.titleKey) }}</div>
        <p class="tour-desc">{{ t(currentStep.descKey) }}</p>
        <div class="tour-actions">
          <button type="button" class="tour-skip" @click="skip">{{ t('onboarding.skip') }}</button>
          <n-space :size="8" align="center">
            <n-button v-if="stepIndex > 0" size="small" quaternary @click="prev">
              {{ t('onboarding.prev') }}
            </n-button>
            <n-button size="small" type="primary" @click="next">
              {{ isLastStep ? t('onboarding.finish') : t('onboarding.next') }}
            </n-button>
          </n-space>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.tour-root {
  position: fixed;
  inset: 0;
  z-index: 10000;
  pointer-events: none;
}

.tour-backdrop {
  position: absolute;
  inset: 0;
  pointer-events: auto;
}

.tour-spotlight {
  position: fixed;
  border-radius: var(--radius-sm, 8px);
  box-shadow: 0 0 0 9999px rgba(0, 0, 0, 0.58);
  border: 2px solid var(--accent);
  pointer-events: none;
  z-index: 10001;
  transition:
    top 0.22s ease,
    left 0.22s ease,
    width 0.22s ease,
    height 0.22s ease;
  animation: tour-pulse 2s ease-in-out infinite;
}

@keyframes tour-pulse {
  0%,
  100% {
    box-shadow:
      0 0 0 9999px rgba(0, 0, 0, 0.58),
      0 0 0 0 rgba(16, 163, 127, 0.35);
  }
  50% {
    box-shadow:
      0 0 0 9999px rgba(0, 0, 0, 0.58),
      0 0 0 6px rgba(16, 163, 127, 0.15);
  }
}

.tour-popover {
  position: fixed;
  z-index: 10002;
  width: min(320px, calc(100vw - 32px));
  padding: 16px 18px;
  border-radius: var(--radius-md, 12px);
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  box-shadow: var(--shadow-lg, 0 12px 40px rgba(0, 0, 0, 0.18));
  pointer-events: auto;
  transition:
    top 0.22s ease,
    left 0.22s ease;
}

.tour-step-label {
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--accent);
  margin-bottom: 6px;
}

.tour-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--text-primary);
  margin-bottom: 8px;
}

.tour-desc {
  margin: 0 0 16px;
  font-size: 13px;
  line-height: 1.6;
  color: var(--text-secondary);
  white-space: pre-line;
}

.tour-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.tour-skip {
  border: none;
  background: none;
  padding: 0;
  font-family: inherit;
  font-size: 12px;
  color: var(--text-muted);
  cursor: pointer;
  transition: color 0.15s ease;
}

.tour-skip:hover {
  color: var(--text-secondary);
}
</style>
