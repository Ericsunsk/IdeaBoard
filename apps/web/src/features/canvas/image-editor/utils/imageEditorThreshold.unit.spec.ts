import { describe, expect, it } from 'vitest'
import {
  clampThresholdValue,
  createThresholdMarker,
  moveThresholdHandle,
  normalizeThresholdAdjustment,
} from './imageEditorThreshold'

describe('imageEditorThreshold', () => {
  it('clamps threshold values and falls back for non-finite input', () => {
    expect(clampThresholdValue(-10)).toBe(0)
    expect(clampThresholdValue(300)).toBe(255)
    expect(clampThresholdValue(127.6)).toBe(128)
    expect(clampThresholdValue(Number.NaN)).toBe(128)
  })

  it('normalizes threshold params without dropping invert', () => {
    expect(normalizeThresholdAdjustment({ threshold: Number.NaN, invert: true })).toEqual({
      threshold: 128,
      invert: true,
    })
  })

  it('creates a marker in percent plot coordinates', () => {
    expect(createThresholdMarker({ threshold: 128 })).toEqual({
      id: 'threshold',
      position: (128 / 255) * 100,
      value: 128,
      ariaLabel: 'Threshold 128',
    })
  })

  it('moves the threshold handle from percent plot coordinates and preserves invert', () => {
    expect(moveThresholdHandle({ threshold: 10, invert: true }, 50)).toEqual({
      threshold: 128,
      invert: true,
    })
    expect(moveThresholdHandle({ threshold: 10, invert: false }, Number.NaN)).toEqual({
      threshold: 128,
      invert: false,
    })
  })
})
