import { i18n } from '../../../../shared/i18n'
import { formatLocaleNumber } from '../../../../shared/i18n/formatters'
import { getActiveUserLocale } from '../../../../shared/i18n/userLocale'
import { formatMinorCurrencyValue, normalizeCurrencyCode } from '../../../../shared/utils/currency'

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
  const activeLocale = getActiveUserLocale()
  const localizedAmount = formatMinorCurrencyValue(amountMinor, currency, activeLocale)
  // Payment history keeps the ISO code visible even where the locale symbol is ambiguous.
  return `${localizedAmount} ${normalizeCurrencyCode(currency)}`
}

export { MIN_IDEA_QUANTITY }
