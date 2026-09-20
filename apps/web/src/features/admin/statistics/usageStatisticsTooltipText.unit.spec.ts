import { describe, expect, it } from 'vitest'
import { formatCount, formatPercent1 } from './usageStatisticsTooltipText'

describe('usageStatisticsTooltipText', () => {
  it('pins formatCount edge cases: 0, 1200, negative, NaN', () => {
    expect(formatCount(0)).toBe('0')
    expect(formatCount(1200)).toBe('1,200')
    expect(formatCount(-1234)).toBe('-1,234')
    expect(formatCount(NaN)).toBe('0')
  })

  it('pins formatPercent1 edge cases: 0, 1200, negative, NaN', () => {
    expect(formatPercent1(0)).toBe('0.0%')
    expect(formatPercent1(12.345)).toBe('12.3%')
    expect(formatPercent1(-1.234)).toBe('-1.2%')
    expect(formatPercent1(NaN)).toBe('0.0%')
  })
})
