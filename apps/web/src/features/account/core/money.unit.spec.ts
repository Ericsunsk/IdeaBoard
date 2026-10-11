import { describe, expect, it } from 'vitest'
import { normalizeIdeaQuantity } from './money'

describe('normalizeIdeaQuantity', () => {
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
