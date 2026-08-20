<script setup lang="ts">
import {computed, onMounted, onUnmounted, ref} from 'vue'
import {useI18n} from 'vue-i18n'
import {NButton, NCard, NModal, NSpace} from 'naive-ui'
import * as App from '../wailsjs/go/desktop/App'
import {EventsOn} from '../wailsjs/runtime/runtime'

type ConfirmRequest = {
  id: number
  kind: string
  title: string
  lines: string[]
}

const {t} = useI18n()

const queue = ref<ConfirmRequest[]>([])
const resolving = ref(false)

const current = computed(() => queue.value[0] || null)
const show = computed(() => !!current.value)
const isSignKind = computed(() => {
  const kind = current.value?.kind || ''
  return kind === 'sign_transaction' || kind === 'sign_evm_transaction' || kind === 'sign_typed_data'
})

let unsubs: (() => void)[] = []

function onConfirm(req: ConfirmRequest) {
  if (!req || typeof req.id !== 'number') return
  if (queue.value.some((r) => r.id === req.id)) return
  queue.value = [...queue.value, {...req, lines: req.lines || []}]
}

function onConfirmExpired(payload: { id: number }) {
  if (!payload) return
  queue.value = queue.value.filter((r) => r.id !== payload.id)
}

async function resolve(approved: boolean) {
  const req = current.value
  if (!req || resolving.value) return
  resolving.value = true
  try {
    await App.ResolveConfirm(req.id, approved)
  } catch {
    /* dialog may have expired server-side */
  } finally {
    queue.value = queue.value.filter((r) => r.id !== req.id)
    resolving.value = false
  }
}

onMounted(() => {
  unsubs = [
    EventsOn('agent:confirm', onConfirm),
    EventsOn('agent:confirm-expired', onConfirmExpired),
  ]
})

onUnmounted(() => {
  unsubs.forEach((u) => u())
  unsubs = []
})
</script>

<template>
  <n-modal :show="show" :mask-closable="false" :closable="false" :close-on-esc="false">
    <n-card v-if="current" style="width: 480px" :title="current.title">
      <ul class="confirm-lines">
        <li v-for="(line, i) in current.lines" :key="i">{{ line }}</li>
      </ul>
      <p v-if="isSignKind" class="confirm-hint">{{ t('confirm.signHint') }}</p>
      <template #footer>
        <n-space justify="end">
          <n-button :disabled="resolving" @click="resolve(false)">
            {{ t('confirm.decline') }}
          </n-button>
          <n-button type="primary" :loading="resolving" @click="resolve(true)">
            {{ t('confirm.approve') }}
          </n-button>
        </n-space>
      </template>
    </n-card>
  </n-modal>
</template>

<style scoped>
.confirm-lines {
  margin: 0;
  padding-left: 18px;
  display: flex;
  flex-direction: column;
  gap: 6px;
  line-height: 1.5;
  word-break: break-all;
}

.confirm-hint {
  margin: 12px 0 0;
  font-size: 12px;
  line-height: 1.5;
  color: var(--text-secondary, #888);
}
</style>
