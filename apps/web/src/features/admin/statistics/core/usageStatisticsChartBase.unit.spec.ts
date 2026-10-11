import { describe, expect, it } from 'vitest'
import {
  getModelColorByName,
  normalizePercent,
  toNonNegativeNumber,
} from './usageStatisticsChartBase'

describe('getModelColorByName', () => {
  it('documents hash-stable assignment: same input order gives same colors and a model inserted at front does not shift existing colors', () => {
    const baseline = ['gpt-image', 'seedream', 'opus'].map((name) => getModelColorByName(name))
    expect(['gpt-image', 'seedream', 'opus'].map((name) => getModelColorByName(name))).toEqual(
      baseline,
    )

    const withFrontInsert = ['new-model', 'gpt-image', 'seedream', 'opus'].map((name) =>
      getModelColorByName(name),
    )
    expect(withFrontInsert.slice(1)).toEqual(baseline)
  })
})

describe('invalid statistic markers', () => {
  it('propagates NaN instead of fabricating 0 for non-finite input', () => {
    expect(toNonNegativeNumber(NaN)).toBeNaN()
    expect(toNonNegativeNumber(null)).toBeNaN()
    expect(toNonNegativeNumber(undefined)).toBeNaN()
    expect(normalizePercent(NaN)).toBeNaN()
    expect(normalizePercent(null)).toBeNaN()
    expect(normalizePercent(undefined)).toBeNaN()
  })

  it('keeps finite normalization: zero stays zero, negatives clamp, percents clamp to 0-100', () => {
    expect(toNonNegativeNumber(0)).toBe(0)
    expect(toNonNegativeNumber(12.7)).toBe(12.7)
    expect(toNonNegativeNumber(-3)).toBe(0)
    expect(normalizePercent(0)).toBe(0)
    expect(normalizePercent(42.5)).toBe(42.5)
    expect(normalizePercent(120)).toBe(100)
    expect(normalizePercent(-5)).toBe(0)
  })
})
