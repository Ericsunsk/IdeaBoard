import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import {
  formatLocaleDate,
  formatLocaleNumber,
  getLocaleDateFormatter,
  getLocaleNumberFormatter,
} from './formatters'
import { i18n } from './index'

function setActiveLocale(locale: string) {
  ;(i18n.global.locale as { value: string }).value = locale
}

// Shared Intl seam contract: all admin number/date rendering routes through
// here. Pins freeze the rendered strings per locale so callers that migrated
// off hardcoded `new Intl.NumberFormat('zh-CN')` / `toLocaleString()` stay
// byte-identical for the active locale.
describe('core/i18n/formatters', () => {
  beforeEach(() => {
    setActiveLocale('en')
  })

  afterEach(() => {
    setActiveLocale('en')
  })

  it('caches formatters per locale+options', () => {
    expect(getLocaleNumberFormatter('zh-CN')).toBe(getLocaleNumberFormatter('zh-CN'))
    expect(getLocaleDateFormatter('zh-CN', { timeZone: 'Asia/Shanghai' })).toBe(
      getLocaleDateFormatter('zh-CN', { timeZone: 'Asia/Shanghai' }),
    )
  })

  it('pins integer grouping for zh-CN and en (identical rendering)', () => {
    expect(formatLocaleNumber(0, undefined, 'zh-CN')).toBe('0')
    expect(formatLocaleNumber(1200, undefined, 'zh-CN')).toBe('1,200')
    expect(formatLocaleNumber(-1234, undefined, 'zh-CN')).toBe('-1,234')
    expect(formatLocaleNumber(1200, undefined, 'en')).toBe('1,200')
    expect(formatLocaleNumber(-1234, undefined, 'en')).toBe('-1,234')
    // BillingPriceDialog max-price pin: 9_000_000_000_000_000.
    expect(formatLocaleNumber(9_000_000_000_000_000, undefined, 'zh-CN')).toBe(
      '9,000,000,000,000,000',
    )
  })

  it('pins fraction options (payment dispute ratio pin)', () => {
    expect(formatLocaleNumber(12.5, { maximumFractionDigits: 2 }, 'zh-CN')).toBe('12.5')
    expect(formatLocaleNumber(12.345, { maximumFractionDigits: 2 }, 'en')).toBe('12.35')
  })

  it('routes the default locale through the active user locale', () => {
    // Test env default is 'en': matches the legacy hardcoded outputs.
    expect(formatLocaleNumber(1200)).toBe('1,200')
    setActiveLocale('zh-CN')
    expect(formatLocaleNumber(1200)).toBe('1,200')
    expect(formatLocaleNumber(9_000_000_000_000_000)).toBe('9,000,000,000,000,000')
  })

  it('pins the reward-code expiry rendering for zh-CN (Asia/Shanghai)', () => {
    const value = new Date('2026-09-19T00:00:00Z')
    const options = {
      timeZone: 'Asia/Shanghai',
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      hour12: false,
    } as const
    expect(formatLocaleDate(value, options, 'zh-CN')).toBe(
      new Intl.DateTimeFormat('zh-CN', options).format(value),
    )
    expect(formatLocaleDate(value, options, 'zh-CN')).toBe('2026/09/19 08:00')
  })
})
