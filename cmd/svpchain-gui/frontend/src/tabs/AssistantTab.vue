<script setup lang="ts">
import AgentView from '../AgentView.vue'
import type { Entry } from '../types'

defineProps<{ entries: Entry[] }>()
const emit = defineEmits<{ status: [msg: string] }>()
</script>

<template>
  <div class="pane-body assistant-tab">
    <AgentView :entries="entries" @status="emit('status', $event)" />
  </div>
</template>

<style scoped>
.assistant-tab {
  min-height: 380px;
}
</style>
