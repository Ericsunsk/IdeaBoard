import type { PrimeVueLocaleOptions } from 'primevue/config'
import { useAppToast } from '../composables/useAppToast'
import { getUserPrimeLocale } from './primeLocale'
import { englishMessages, i18n } from './i18nRuntime'
import {
  SUPPORTED_LOCALES,
  USER_LOCALE_STORAGE_KEY,
  type LocaleBundle,
  type SupportedLocale,
} from './localeCore'
import { LOCALE_MANIFEST } from './localeManifest'
import { isValidSupportedLocale, resolveInitialUserLocale } from './localeDetection'

export {
  SUPPORTED_LOCALES,
  USER_LOCALE_STORAGE_KEY,
  isValidSupportedLocale,
  resolveInitialUserLocale,
}
export type { SupportedLocale }

const messageBundleCache = new Map<SupportedLocale, LocaleBundle>()
const inFlightLoaders = new Map<SupportedLocale, Promise<LocaleBundle>>()
let activePrimeVueConfig: { locale?: PrimeVueLocaleOptions } | null = null

/**
 * Dynamically loads and caches the message bundle for the specified locale.
 * 'en' is bundled synchronously.
 */
export async function loadLocaleMessages(locale: SupportedLocale): Promise<LocaleBundle> {
  if (locale === 'en') {
    messageBundleCache.set('en', englishMessages as unknown as LocaleBundle)
    return englishMessages as unknown as LocaleBundle
  }

  const cached = messageBundleCache.get(locale)
  if (cached) {
    return cached
  }

  const inFlight = inFlightLoaders.get(locale)
  if (inFlight) {
    return inFlight
  }

  const loaderPromise = (async () => {
    try {
      const loader = LOCALE_MANIFEST[locale].messagesLoader
      if (!loader) {
        throw new Error(`No locale loader available for locale: ${locale}`)
      }

      const rawModule = await loader()
      const bundle = (
        rawModule && typeof rawModule === 'object' && 'default' in rawModule && rawModule.default
          ? rawModule.default
          : rawModule
      ) as LocaleBundle

      messageBundleCache.set(locale, bundle)

      if (i18n && i18n.global && typeof i18n.global.setLocaleMessage === 'function') {
        i18n.global.setLocaleMessage(locale, bundle as never)
      }

      return bundle
    } finally {
      inFlightLoaders.delete(locale)
    }
  })()

  inFlightLoaders.set(locale, loaderPromise)
  return loaderPromise
}

/**
 * Registers an active PrimeVue config reference to be synchronized when the user locale changes.
 */
export function registerPrimeVueConfig(config: { locale?: PrimeVueLocaleOptions } | null): void {
  activePrimeVueConfig = config
}

/**
 * Synchronizes the active PrimeVue configuration to match the specified locale.
 */
export function syncPrimeVueLocale(locale: SupportedLocale): void {
  if (activePrimeVueConfig) {
    activePrimeVueConfig.locale = getUserPrimeLocale(locale)
  }
}

/**
 * Returns the currently active user locale.
 */
export function getActiveUserLocale(): SupportedLocale {
  if (i18n && i18n.global) {
    const current = (i18n.global.locale as { value: string }).value
    if (isValidSupportedLocale(current)) {
      return current
    }
  }
  return 'en'
}

/**
 * Returns the PrimeVueLocaleOptions matching the active user locale.
 */
export function getActiveUserPrimeLocale(): PrimeVueLocaleOptions {
  return getUserPrimeLocale(getActiveUserLocale())
}

/**
 * Atomically switches the user locale, updating i18n, PrimeVue, <html lang>, and localStorage.
 * On load failure: retains the current locale, displays a toast error, and does NOT mutate storage.
 */
export async function setUserLocale(
  locale: SupportedLocale,
  primeVueConfig?: { locale?: PrimeVueLocaleOptions },
): Promise<boolean> {
  if (!isValidSupportedLocale(locale)) {
    return false
  }

  if (primeVueConfig) {
    activePrimeVueConfig = primeVueConfig
  }

  if (locale !== 'en') {
    try {
      await loadLocaleMessages(locale)
    } catch {
      const toast = useAppToast()
      const fallbackErrorMessage =
        (i18n.global.t &&
          typeof i18n.global.t === 'function' &&
          i18n.global.t('common.errors.requestIncomplete')) ||
        'The request could not be completed.'
      toast.error(fallbackErrorMessage)
      return false
    }
  }

  if (i18n && i18n.global) {
    ;(i18n.global.locale as { value: string }).value = locale
  }

  syncPrimeVueLocale(locale)

  if (typeof document !== 'undefined' && document.documentElement) {
    document.documentElement.lang = locale
  }

  if (typeof localStorage !== 'undefined') {
    try {
      localStorage.setItem(USER_LOCALE_STORAGE_KEY, locale)
    } catch {
      // Ignore storage errors (e.g. sandboxed iframe or private browsing)
    }
  }

  return true
}

/**
 * Pre-mount initialization called in main.ts before mounting the Vue app.
 * Resolves initial locale and loads bundle so returning users don't see an English flash.
 * Gracefully falls back to English on failure without mutating localStorage.
 */
export async function initUserLocale(): Promise<SupportedLocale> {
  const targetLocale = resolveInitialUserLocale()

  if (targetLocale !== 'en') {
    try {
      await loadLocaleMessages(targetLocale)
      if (i18n && i18n.global) {
        ;(i18n.global.locale as { value: string }).value = targetLocale
      }
      if (typeof document !== 'undefined' && document.documentElement) {
        document.documentElement.lang = targetLocale
      }
      syncPrimeVueLocale(targetLocale)
      return targetLocale
    } catch {
      // Graceful fallback to English if non-English bundle fails to load during bootstrap
    }
  }

  if (i18n && i18n.global) {
    ;(i18n.global.locale as { value: string }).value = 'en'
  }
  if (typeof document !== 'undefined' && document.documentElement) {
    document.documentElement.lang = 'en'
  }
  syncPrimeVueLocale('en')
  return 'en'
}
