import { describe, expect, it } from 'vitest'
import {
  formatCurrencyMinor,
  formatUsdForIdeas,
  normalizeIdeaQuantity,
  parseIdeaQuantity,
} from './paymentMoney'

describe('paymentMoney', () => {
  it('derives the exact two-decimal USD price from integer Ideas', () => {
    expect(formatUsdForIdeas(500)).toBe('$5.00 USD')
    expect(formatUsdForIdeas(501)).toBe('$5.01 USD')
    expect(formatUsdForIdeas(500_000)).toBe('$5,000.00 USD')
  })

  it("uses each currency's Intl minor-unit precision without floating point arithmetic", () => {
    expect(formatCurrencyMinor(12_345, 'usd')).toBe('$123.45 USD')
    expect(formatCurrencyMinor(500, 'cny')).toBe('CN¥5.00 CNY')
    expect(formatCurrencyMinor(500, 'hkd')).toBe('HK$5.00 HKD')
    expect(formatCurrencyMinor(500, 'jpy')).toBe('¥500 JPY')
  })

  it.each([
    ['500', 500],
    ['501', 501],
    ['500000', 500_000],
    [' 1001 ', 1001],
  ])('accepts an in-range integer quantity %s', (value, expected) => {
    expect(parseIdeaQuantity(value)).toEqual({ ok: true, value: expected })
  })

  it.each([
    ['', 'Enter an amount to top up.'],
    ['499', 'Minimum top-up is 500 Ideas.'],
    ['500001', 'Maximum top-up is 500,000 Ideas.'],
    ['500.5', 'Enter a whole number between 500 and 500,000.'],
    ['5e2', 'Enter a whole number between 500 and 500,000.'],
    ['1,000', 'Enter a whole number between 500 and 500,000.'],
  ])('rejects invalid quantity %s', (value, error) => {
    expect(parseIdeaQuantity(value)).toEqual({ ok: false, error })
  })

  it.each([
    ['499', 500],
    ['500.9', 500],
    ['501.9', 501],
    ['500001', 500_000],
    ['1,000', 1000],
    ['not-a-number', null],
    ['', null],
  ])('normalizes custom quantity %s to %s', (value, expected) => {
    expect(normalizeIdeaQuantity(value)).toBe(expected)
  })
})
