import { describe, expect, it } from 'vitest'
import {
  cloneSharpenDefaults,
  cloneSharpenParams,
  isNeutralSharpenEffect,
  isSharpenParams,
  normalizeSharpenEffect,
} from './sharpen'

describe('retouchSharpen', () => {
  it('normalizes sharpen params through kind-local ranges', () => {
    expect(
      normalizeSharpenEffect({ amount: 240, radius: 1, threshold: 4, protectColor: true }),
    ).toEqual({ amount: 200, radius: 1, threshold: 4, protectColor: true })
    expect(
      normalizeSharpenEffect({
        amount: 50,
        radius: Number.NaN,
        threshold: -10,
        protectColor: false,
      }),
    ).toEqual({ amount: 50, radius: 1, threshold: 0, protectColor: false })
  })

  it('detects neutral sharpen effects by zero amount', () => {
    expect(isNeutralSharpenEffect({ amount: 0, radius: 1, threshold: 4, protectColor: true })).toBe(
      true,
    )
    expect(isNeutralSharpenEffect({ amount: 1, radius: 1, threshold: 4, protectColor: true })).toBe(
      false,
    )
  })

  it('guards sharpen whole params against threshold params sharing the threshold key', () => {
    expect(isSharpenParams({ amount: 10, radius: 1, threshold: 4, protectColor: true })).toBe(true)
    expect(isSharpenParams({ threshold: 128, invert: false })).toBe(false)
    expect(isSharpenParams({ channels: {} })).toBe(false)
  })

  it('sanitizes sharpen clones and defaults without aliasing', () => {
    expect(
      cloneSharpenParams({ amount: 240, radius: 1, threshold: 4, protectColor: true }),
    ).toEqual({
      amount: 200,
      radius: 1,
      threshold: 4,
      protectColor: true,
    })
    expect(cloneSharpenDefaults()).toEqual({
      amount: 0,
      radius: 1,
      threshold: 4,
      protectColor: true,
    })
  })
})
