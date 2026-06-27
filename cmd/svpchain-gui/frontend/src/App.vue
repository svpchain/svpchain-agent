<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NConfigProvider,
  NMessageProvider,
  NDialogProvider,
  darkTheme,
  zhCN,
  enUS,
  dateZhCN,
  dateEnUS,
} from 'naive-ui'
import MainView from './MainView.vue'
import { useAppTheme } from './composables/useAppTheme'
import { themeOverridesFor } from './theme'

const { locale } = useI18n()
const { isDark } = useAppTheme()

const naiveLocale = computed(() => (locale.value === 'zh' ? zhCN : enUS))
const naiveDateLocale = computed(() => (locale.value === 'zh' ? dateZhCN : dateEnUS))
const themeOverrides = computed(() => themeOverridesFor(isDark.value ? 'dark' : 'light'))
const naiveTheme = computed(() => (isDark.value ? darkTheme : null))
</script>

<template>
  <n-config-provider
    :theme="naiveTheme"
    :locale="naiveLocale"
    :date-locale="naiveDateLocale"
    :theme-overrides="themeOverrides"
  >
    <n-message-provider>
      <n-dialog-provider>
        <MainView />
      </n-dialog-provider>
    </n-message-provider>
  </n-config-provider>
</template>
