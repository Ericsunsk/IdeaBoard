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

export const SUPPORTED_LOCALES = ['en'] as const
export type SupportedLocale = (typeof SUPPORTED_LOCALES)[number]

const messages = {
  en: {
    common,
    auth,
    search,
    canvas,
    library,
    billing,
    errorCodes: errors,
    workspace,
    account,
  },
} as const

/**
 * English-only i18n foundation for User-facing surfaces.
 * Deliberately minimal: no locale switching, detection, or persistence in this phase.
 */
export const i18n = createI18n({
  legacy: false,
  locale: 'en' satisfies SupportedLocale,
  fallbackLocale: 'en',
  messages,
  missingWarn: false,
  fallbackWarn: false,
})

export type AppI18n = typeof i18n
