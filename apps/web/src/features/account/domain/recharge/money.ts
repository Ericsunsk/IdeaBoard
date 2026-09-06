import { i18n } from '../../../../shared/i18n'
import { formatLocaleNumber, getLocaleNumberFormatter } from '../../../../shared/i18n/formatters'
import { getActiveUserLocale } from '../../../../shared/i18n/userLocale'

const MIN_IDEA_QUANTITY = 500
const MAX_IDEA_QUANTITY = 500_000
const integerPattern = /^\d+$/

function integerQuantityLabel(value: number): string {
  return formatLocaleNumber(value)
}

function integerRangeError(): string {
  return i18n.global.t('account.errors.quantityIntegerRange', {
    min: integerQuantityLabel(MIN_IDEA_QUANTITY),
    max: integerQuantityLabel(MAX_IDEA_QUANTITY),
  })
}

export type IdeaQuantityParseResult = { ok: true; value: number } | { ok: false; error: string }

export function parseIdeaQuantity(rawValue: string): IdeaQuantityParseResult {
  const value = rawValue.trim()
  if (!value) return { ok: false, error: i18n.global.t('account.errors.quantityRequired') }
  if (!integerPattern.test(value)) {
    return { ok: false, error: integerRangeError() }
  }

  const quantity = Number(value)
  if (!Number.isSafeInteger(quantity)) {
    return { ok: false, error: integerRangeError() }
  }
  if (quantity < MIN_IDEA_QUANTITY) {
    return {
      ok: false,
      error: i18n.global.t('account.errors.quantityBelowMin', {
        min: integerQuantityLabel(MIN_IDEA_QUANTITY),
      }),
    }
  }
  if (quantity > MAX_IDEA_QUANTITY) {
    return {
      ok: false,
      error: i18n.global.t('account.errors.quantityAboveMax', {
        max: integerQuantityLabel(MAX_IDEA_QUANTITY),
      }),
    }
  }
  return { ok: true, value: quantity }
}

export function normalizeIdeaQuantity(rawValue: string): number | null {
  const value = rawValue.trim().replaceAll(',', '')
  if (!value) return null

  const quantity = Number(value)
  if (!Number.isFinite(quantity)) return null

  return Math.min(MAX_IDEA_QUANTITY, Math.max(MIN_IDEA_QUANTITY, Math.floor(quantity)))
}

export function formatUsdForIdeas(ideaQuantity: number): string {
  return formatCurrencyMinor(ideaQuantity, 'USD')
}

export function formatCurrencyMinor(amountMinor: number, currency: string): string {
  if (!Number.isSafeInteger(amountMinor) || amountMinor < 0) {
    throw new RangeError('amountMinor must be a non-negative safe integer')
  }

  const normalizedCurrency = currency.trim().toUpperCase()
  if (!/^[A-Z]{3}$/.test(normalizedCurrency)) {
    throw new RangeError('currency must be a three-letter ISO code')
  }

  const fractionDigits = currencyFractionDigits(normalizedCurrency)
  const minorUnitsPerMajor = 10n ** BigInt(fractionDigits)
  const minorAmount = BigInt(amountMinor)
  const major = minorAmount / minorUnitsPerMajor
  const minor = minorAmount % minorUnitsPerMajor
  const activeLocale = getActiveUserLocale()
  const wholeNumber = getLocaleNumberFormatter(activeLocale, {
    maximumFractionDigits: 0,
    useGrouping: true,
  }).format(major)
  const minorDisplay = minor.toString().padStart(fractionDigits, '0')
  const currencyFormatter = getLocaleNumberFormatter(activeLocale, {
    style: 'currency',
    currency: normalizedCurrency,
    currencyDisplay: 'symbol',
    minimumFractionDigits: fractionDigits,
    maximumFractionDigits: fractionDigits,
  })

  // Build the localized currency pattern from Intl while keeping minor-unit math exact.
  const localizedAmount = currencyFormatter
    .formatToParts(0)
    .map((part) => {
      if (part.type === 'integer') return wholeNumber
      if (part.type === 'fraction') return minorDisplay
      return part.value
    })
    .join('')

  // Payment history keeps the ISO code visible even where the locale symbol is ambiguous.
  return `${localizedAmount} ${normalizedCurrency}`
}

function currencyFractionDigits(currency: string): number {
  return (
    getLocaleNumberFormatter('en', {
      style: 'currency',
      currency,
    }).resolvedOptions().maximumFractionDigits ?? 2
  )
}

export { MAX_IDEA_QUANTITY, MIN_IDEA_QUANTITY }
