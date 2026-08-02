import { createI18n } from 'vue-i18n'
import en from './locales/en'
import zh from './locales/zh'

export const i18n = createI18n({
  legacy: false,
  locale: 'en',
  fallbackLocale: 'en',
  messages: { en, zh },
})

export function setLocale(lang: string) {
  i18n.global.locale.value = lang === 'zh' ? 'zh' : 'en'
}
