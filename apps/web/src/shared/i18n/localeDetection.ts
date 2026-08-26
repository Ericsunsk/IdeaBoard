import { SUPPORTED_LOCALES, USER_LOCALE_STORAGE_KEY, type SupportedLocale } from './localeCore'

export function isValidSupportedLocale(locale: unknown): locale is SupportedLocale {
  return typeof locale === 'string' && (SUPPORTED_LOCALES as readonly string[]).includes(locale)
}

/**
 * Resolves the initial user locale.
 * Checks storage first (if valid); falls back to navigator.languages (mapped per product rules);
 * defaults to 'en'.
 */
export function resolveInitialUserLocale(
  storageValue?: string | null,
  navigatorLanguages?: readonly string[],
): SupportedLocale {
  let resolvedStorageValue = storageValue
  if (resolvedStorageValue === undefined && typeof localStorage !== 'undefined') {
    try {
      resolvedStorageValue = localStorage.getItem(USER_LOCALE_STORAGE_KEY)
    } catch {
      resolvedStorageValue = null
    }
  }

  if (isValidSupportedLocale(resolvedStorageValue)) {
    return resolvedStorageValue
  }

  let resolvedLanguages = navigatorLanguages
  if (resolvedLanguages === undefined && typeof navigator !== 'undefined') {
    resolvedLanguages = navigator.languages?.length
      ? navigator.languages
      : [navigator.language].filter(Boolean)
  }

  if (resolvedLanguages && resolvedLanguages.length > 0) {
    for (const rawTag of resolvedLanguages) {
      if (typeof rawTag !== 'string') continue
      const tag = rawTag.trim().toLowerCase()
      if (!tag) continue

      // Traditional Chinese tags fall back to 'en' per product decision §3.2
      if (
        tag === 'zh-tw' ||
        tag === 'zh-hk' ||
        tag === 'zh-mo' ||
        tag.startsWith('zh-tw-') ||
        tag.startsWith('zh-hk-') ||
        tag.startsWith('zh-mo-') ||
        tag.startsWith('zh-hant')
      ) {
        return 'en'
      }

      // Simplified Chinese tags map to 'zh-CN'
      if (
        tag === 'zh-cn' ||
        tag === 'zh-sg' ||
        tag === 'zh-hans' ||
        tag.startsWith('zh-cn-') ||
        tag.startsWith('zh-sg-') ||
        tag.startsWith('zh-hans')
      ) {
        return 'zh-CN'
      }

      // Spanish dialects map to 'es'
      if (tag === 'es' || tag.startsWith('es-')) {
        return 'es'
      }

      // Korean dialects map to 'ko'
      if (tag === 'ko' || tag.startsWith('ko-')) {
        return 'ko'
      }

      // English dialects map to 'en'
      if (tag === 'en' || tag.startsWith('en-')) {
        return 'en'
      }
    }
  }

  // Fallback for all unsupported locales
  return 'en'
}
