import { getLocaleNumberFormatter } from '../i18n/formatters'
import type { SupportedLocale } from '../i18n/userLocale'

/** Validate and normalize to an uppercase three-letter ISO code. */
export function normalizeCurrencyCode(currency: string): string {
  const normalized = currency.trim().toUpperCase()
  if (!/^[A-Z]{3}$/.test(normalized)) {
    throw new RangeError('currency must be a three-letter ISO code')
  }
  return normalized
}

export function currencyFractionDigits(normalizedCurrency: string): number {
  return (
    getLocaleNumberFormatter('en', {
      style: 'currency',
      currency: normalizedCurrency,
    }).resolvedOptions().maximumFractionDigits ?? 2
  )
}

/**
 * Format a minor-unit amount with exact BigInt math inside a localized
 * currency pattern (no ISO suffix — callers own display policy).
 * Throws `RangeError` on invalid input so fail-fast and fallback callers
 * can each decide how to present it.
 */
export function formatMinorCurrencyValue(
  amountMinor: number,
  currency: string,
  locale: SupportedLocale,
): string {
  if (!Number.isSafeInteger(amountMinor) || amountMinor < 0) {
    throw new RangeError('amountMinor must be a non-negative safe integer')
  }

  const normalizedCurrency = normalizeCurrencyCode(currency)
  const fractionDigits = currencyFractionDigits(normalizedCurrency)
  const minorUnitsPerMajor = 10n ** BigInt(fractionDigits)
  const minorAmount = BigInt(amountMinor)
  const major = minorAmount / minorUnitsPerMajor
  const minor = minorAmount % minorUnitsPerMajor
  const wholeNumber = getLocaleNumberFormatter(locale, {
    maximumFractionDigits: 0,
    useGrouping: true,
  }).format(major)
  const minorDisplay = minor.toString().padStart(fractionDigits, '0')
  const currencyFormatter = getLocaleNumberFormatter(locale, {
    style: 'currency',
    currency: normalizedCurrency,
    currencyDisplay: 'symbol',
    minimumFractionDigits: fractionDigits,
    maximumFractionDigits: fractionDigits,
  })

  // Build the localized currency pattern from Intl while keeping minor-unit math exact.
  return currencyFormatter
    .formatToParts(0)
    .map((part) => {
      if (part.type === 'integer') return wholeNumber
      if (part.type === 'fraction') return minorDisplay
      return part.value
    })
    .join('')
}
