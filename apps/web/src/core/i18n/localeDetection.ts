import { SUPPORTED_LOCALES, USER_LOCALE_STORAGE_KEY, type SupportedLocale } from './localeCore'
import { readLocalStorageItem } from '../../core/utils/storage'

export function isValidSupportedLocale(locale: unknown): locale is SupportedLocale {
  return typeof locale === 'string' && (SUPPORTED_LOCALES as readonly string[]).includes(locale)
}

type LocaleMatchRule = {
  locale: SupportedLocale
  exact: readonly string[]
  prefix: readonly string[]
}

// Ordered: first matching rule wins. `exact` requires a full-tag match,
// `prefix` matches tag prefixes (dash handling included in the entries).
const LOCALE_MATCH_RULES: readonly LocaleMatchRule[] = [
  // Traditional Chinese tags fall back to 'en' per product decision §3.2
  {
    locale: 'en',
    exact: ['zh-tw', 'zh-hk', 'zh-mo'],
    prefix: ['zh-tw-', 'zh-hk-', 'zh-mo-', 'zh-hant'],
  },
  // Simplified Chinese tags map to 'zh-CN'
  {
    locale: 'zh-CN',
    exact: ['zh-cn', 'zh-sg', 'zh-hans'],
    prefix: ['zh-cn-', 'zh-sg-', 'zh-hans'],
  },
  // Spanish dialects map to 'es'
  { locale: 'es', exact: ['es'], prefix: ['es-'] },
  // Korean dialects map to 'ko'
  { locale: 'ko', exact: ['ko'], prefix: ['ko-'] },
  // English dialects map to 'en'
  { locale: 'en', exact: ['en'], prefix: ['en-'] },
]

function matchLocaleTag(tag: string): SupportedLocale | null {
  const rule = LOCALE_MATCH_RULES.find(
    ({ exact, prefix }) => exact.includes(tag) || prefix.some((part) => tag.startsWith(part)),
  )
  return rule?.locale ?? null
}

function resolveNavigatorLanguages(
  navigatorLanguages?: readonly string[],
): readonly unknown[] | null {
  if (navigatorLanguages !== undefined) return navigatorLanguages
  if (typeof navigator === 'undefined') return null
  return navigator.languages?.length ? navigator.languages : [navigator.language].filter(Boolean)
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
  if (resolvedStorageValue === undefined) {
    resolvedStorageValue = readLocalStorageItem(USER_LOCALE_STORAGE_KEY)
  }

  if (isValidSupportedLocale(resolvedStorageValue)) {
    return resolvedStorageValue
  }

  const resolvedLanguages = resolveNavigatorLanguages(navigatorLanguages)
  if (resolvedLanguages && resolvedLanguages.length > 0) {
    for (const rawTag of resolvedLanguages) {
      if (typeof rawTag !== 'string') continue
      const tag = rawTag.trim().toLowerCase()
      if (!tag) continue
      const matched = matchLocaleTag(tag)
      if (matched) return matched
    }
  }

  // Fallback for all unsupported locales
  return 'en'
}
