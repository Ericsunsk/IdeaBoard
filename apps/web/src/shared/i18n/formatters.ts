import { getActiveUserLocale, type SupportedLocale } from './userLocale'
import { LOCALE_MANIFEST } from './localeManifest'

const numberCache = new Map<string, Intl.NumberFormat>()
const dateCache = new Map<string, Intl.DateTimeFormat>()

export function toIntlLocale(locale: SupportedLocale): string {
  return LOCALE_MANIFEST[locale].intlTag
}

function cacheKey(locale: SupportedLocale, options?: Record<string, unknown>): string {
  return `${locale}:${JSON.stringify(options ?? {})}`
}

export function getLocaleNumberFormatter(
  locale: SupportedLocale,
  options?: Intl.NumberFormatOptions,
): Intl.NumberFormat {
  const key = cacheKey(locale, options as Record<string, unknown>)
  let fmt = numberCache.get(key)
  if (!fmt) {
    fmt = new Intl.NumberFormat(toIntlLocale(locale), options)
    numberCache.set(key, fmt)
  }
  return fmt
}

export function getLocaleDateFormatter(
  locale: SupportedLocale,
  options?: Intl.DateTimeFormatOptions,
): Intl.DateTimeFormat {
  const key = cacheKey(locale, options as Record<string, unknown>)
  let fmt = dateCache.get(key)
  if (!fmt) {
    fmt = new Intl.DateTimeFormat(toIntlLocale(locale), options)
    dateCache.set(key, fmt)
  }
  return fmt
}

export function formatLocaleNumber(
  value: number,
  options?: Intl.NumberFormatOptions,
  locale?: SupportedLocale,
): string {
  const active = locale ?? getActiveUserLocale()
  return getLocaleNumberFormatter(active, options).format(value)
}

export function formatLocaleDate(
  value: Date | number,
  options?: Intl.DateTimeFormatOptions,
  locale?: SupportedLocale,
): string {
  const active = locale ?? getActiveUserLocale()
  return getLocaleDateFormatter(active, options).format(value)
}
