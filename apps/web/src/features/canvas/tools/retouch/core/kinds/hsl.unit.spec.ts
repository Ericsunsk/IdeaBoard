import { describe, expect, it } from 'vitest'
import {
  cloneHslDefaults,
  cloneHslParams,
  isHslParams,
  isNeutralHslEffect,
  normalizeHslEffect,
} from './hsl'

describe('retouchHsl', () => {
  it('normalizes hsl params into unit ranges with neutral defaults', () => {
    expect(normalizeHslEffect({ hue: 0, saturation: 0, lightness: 0 })).toEqual({
      hueShift: 0,
      saturationScale: 1,
      lightnessOffset: 0,
    })
    expect(normalizeHslEffect({ hue: 180, saturation: 50, lightness: -50 })).toEqual({
      hueShift: 0.5,
      saturationScale: 1.5,
      lightnessOffset: -0.5,
    })
  })

  it('falls back non-finite hsl input to neutral', () => {
    expect(
      normalizeHslEffect({
        hue: Number.NaN,
        saturation: Number.POSITIVE_INFINITY,
        lightness: Number.NEGATIVE_INFINITY,
      }),
    ).toEqual({ hueShift: 0, saturationScale: 1, lightnessOffset: 0 })
  })

  it('detects neutral hsl effects', () => {
    expect(isNeutralHslEffect({ hueShift: 0, saturationScale: 1, lightnessOffset: 0 })).toBe(true)
    expect(isNeutralHslEffect({ hueShift: 0.5, saturationScale: 1, lightnessOffset: 0 })).toBe(
      false,
    )
  })

  it('guards hsl whole params by exclusive hue keys', () => {
    expect(isHslParams({ hue: 0, saturation: 0, lightness: 0 })).toBe(true)
    expect(isHslParams({ threshold: 1, invert: true })).toBe(false)
    expect(isHslParams(null)).toBe(false)
  })

  it('clones hsl params and defaults without aliasing', () => {
    const source = { hue: 10, saturation: 20, lightness: 30 }
    expect(cloneHslParams(source)).toEqual(source)
    expect(cloneHslParams(source)).not.toBe(source)
    expect(cloneHslDefaults()).toEqual({ hue: 0, saturation: 0, lightness: 0 })
  })
})
