<script setup lang="ts">
import AgentView from '../AgentView.vue'
import type {Entry} from '../types'

defineProps<{ entries: Entry[] }>()
const emit = defineEmits<{ status: [msg: string]; 'focus-assistant': [] }>()
</script>

<template>
  <div class="assistant-tab">
    <AgentView
        :entries="entries"
        @status="emit('status', $event)"
        @focus-assistant="emit('focus-assistant')"
    />
  </div>
</template>

<style scoped>
.assistant-tab {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  height: 100%;
}
</style>
