const MIN_IDEA_QUANTITY = 500
const MAX_IDEA_QUANTITY = 500_000
const integerPattern = /^\d+$/

export type IdeaQuantityParseResult = { ok: true; value: number } | { ok: false; error: string }

export function parseIdeaQuantity(rawValue: string): IdeaQuantityParseResult {
  const value = rawValue.trim()
  if (!value) return { ok: false, error: '请输入充值数量' }
  if (!integerPattern.test(value)) {
    return { ok: false, error: '请输入 500–500,000 之间的整数' }
  }

  const quantity = Number(value)
  if (!Number.isSafeInteger(quantity)) {
    return { ok: false, error: '请输入 500–500,000 之间的整数' }
  }
  if (quantity < MIN_IDEA_QUANTITY) return { ok: false, error: '最少充值 500 Ideas' }
  if (quantity > MAX_IDEA_QUANTITY) return { ok: false, error: '最多充值 500,000 Ideas' }
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
  const majorDisplay = new Intl.NumberFormat('en-US', { maximumFractionDigits: 0 }).format(major)
  const symbol = currencySymbol(normalizedCurrency)
  const decimal = fractionDigits > 0 ? `.${minor.toString().padStart(fractionDigits, '0')}` : ''
  return `${symbol}${majorDisplay}${decimal} ${normalizedCurrency}`
}

function currencyFractionDigits(currency: string): number {
  return (
    new Intl.NumberFormat('en-US', {
      style: 'currency',
      currency,
    }).resolvedOptions().maximumFractionDigits ?? 2
  )
}

function currencySymbol(currency: string): string {
  const symbol = new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency,
    currencyDisplay: 'symbol',
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  })
    .formatToParts(0)
    .find((part) => part.type === 'currency')?.value
  return symbol === currency ? '' : (symbol ?? '')
}

export { MAX_IDEA_QUANTITY, MIN_IDEA_QUANTITY }
