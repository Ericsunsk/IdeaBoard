import { createI18n } from 'vue-i18n'
import {
  common,
  auth,
  search,
  canvas,
  library,
  billing,
  errors,
  workspace,
  account,
  board,
  admin,
} from './locales/en/index'
import type { SupportedLocale } from './localeCore'

export const englishMessages = {
  common,
  auth,
  search,
  canvas,
  library,
  billing,
  errorCodes: errors,
  workspace,
  account,
  board,
  admin,
} as const

export const i18n = createI18n({
  legacy: false,
  locale: 'en' as SupportedLocale,
  fallbackLocale: 'en',
  messages: {
    en: englishMessages,
  },
  missingWarn: false,
  fallbackWarn: false,
})

export type AppI18n = typeof i18n
