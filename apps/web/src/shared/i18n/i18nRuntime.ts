import { createI18n } from 'vue-i18n'
import common from './locales/en/common'
import auth from './locales/en/auth'
import search from './locales/en/search'
import canvas from './locales/en/canvas'
import library from './locales/en/library'
import billing from './locales/en/billing'
import errors from './locales/en/errors'
import workspace from './locales/en/workspace'
import account from './locales/en/account'
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
