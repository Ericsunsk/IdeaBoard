import { useAppToast } from '../composables/useAppToast'
import { englishMessages, i18n } from './i18nRuntime'
import {
  SUPPORTED_LOCALES,
  USER_LOCALE_STORAGE_KEY,
  type LocaleBundle,
  type SupportedLocale,
} from './localeCore'
import { LOCALE_MANIFEST } from './localeManifest'
import { writeLocalStorageItem } from '../utils/storage'
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
 * Atomically switches the user locale, updating i18n, <html lang>, and localStorage.
 * On load failure: retains the current locale, displays a toast error, and does NOT mutate storage.
 */
export async function setUserLocale(locale: SupportedLocale): Promise<boolean> {
  if (!isValidSupportedLocale(locale)) {
    return false
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

  if (typeof document !== 'undefined' && document.documentElement) {
    document.documentElement.lang = locale
  }

  // Best-effort: sandboxed iframes or private browsing must not break the switch.
  writeLocalStorageItem(USER_LOCALE_STORAGE_KEY, locale)

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
  return 'en'
}
