<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NConfigProvider,
  NMessageProvider,
  NDialogProvider,
  zhCN,
  enUS,
  dateZhCN,
  dateEnUS,
  type GlobalThemeOverrides,
} from 'naive-ui'
import MainView from './MainView.vue'

const { locale } = useI18n()
const naiveLocale = computed(() => (locale.value === 'zh' ? zhCN : enUS))
const naiveDateLocale = computed(() => (locale.value === 'zh' ? dateZhCN : dateEnUS))

// Apple-like soft rounded corners across all controls.
const themeOverrides: GlobalThemeOverrides = {
  common: {
    borderRadius: '10px',
    borderRadiusSmall: '8px',
  },
  Button: { borderRadius: '10px' },
  Input: { borderRadius: '10px' },
  Select: { peers: { InternalSelection: { borderRadius: '10px' } } },
  DataTable: { borderRadius: '12px' },
  Card: { borderRadius: '14px' },
  Tag: { borderRadius: '8px' },
}
</script>

<template>
  <n-config-provider :locale="naiveLocale" :date-locale="naiveDateLocale" :theme-overrides="themeOverrides">
    <n-message-provider>
      <n-dialog-provider>
        <MainView />
      </n-dialog-provider>
    </n-message-provider>
  </n-config-provider>
</template>
