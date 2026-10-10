import { describe, expect, it } from 'vitest'
import {
  IMAGE_RETOUCH_DEFINITIONS,
  cloneRetouchParamsForKind,
  getImageRetouchDefaultParams,
  getImageRetouchTrait,
  isRetouchParamsForKind,
  normalizeImageEffectLayer,
} from './definitions'
import type { RetouchEffectLayer } from './model.ts'

describe('imageRetouchDefinitions', () => {
  it('returns defensive copies for default retouch params', () => {
    const hslDefaults = getImageRetouchDefaultParams('hsl')
    const sharpenDefaults = getImageRetouchDefaultParams('sharpen')
    hslDefaults.hue = 90
    sharpenDefaults.amount = 120

    expect(getImageRetouchDefaultParams('hsl')).toEqual({
      hue: 0,
      saturation: 0,
      lightness: 0,
    })
    expect(getImageRetouchDefaultParams('sharpen')).toEqual({
      amount: 0,
      radius: 1,
      threshold: 4,
      protectColor: true,
    })
  })

  it('normalizes levels layers through the definition registry', () => {
    const layer: RetouchEffectLayer = {
      id: 'retouch:levels',
      type: 'effect',
      kind: 'levels',
      name: '色阶',
      visible: true,
      params: {
        shadows: 64,
        midtones: 128,
        highlights: 191,
        outputBlack: 0,
        outputWhite: 255,
      },
    }

    const normalized = normalizeImageEffectLayer(layer)

    expect(normalized.kind).toBe('levels')
    if (normalized.kind !== 'levels') return
    expect(normalized.params).toMatchObject({
      blackPoint: 64,
      whitePoint: 191,
      outputBlack: 0,
      outputWhite: 255,
    })
  })

  it('returns independent curves defaults without sharing channel references', () => {
    const firstCurvesDefaults = getImageRetouchDefaultParams('curves')
    firstCurvesDefaults.channels.rgb = [{ id: 'midtone', input: 50, output: 60 }]

    expect(getImageRetouchDefaultParams('curves')).toEqual({ channels: {} })
  })

  it('normalizes sharpen layers through the definition registry', () => {
    const layer: RetouchEffectLayer = {
      id: 'retouch:sharpen',
      type: 'effect',
      kind: 'sharpen',
      name: '锐化',
      visible: true,
      params: {
        amount: 240,
        radius: Number.NaN,
        threshold: -10,
        protectColor: false,
      },
    }

    const normalized = normalizeImageEffectLayer(layer)

    expect(normalized.kind).toBe('sharpen')
    if (normalized.kind !== 'sharpen') return
    expect(normalized.params).toEqual({
      amount: 200,
      radius: 1,
      threshold: 0,
      protectColor: false,
    })
  })

  it('exposes one trait per kind with mutually exclusive whole-params guards', () => {
    expect(Object.keys(IMAGE_RETOUCH_DEFINITIONS).sort()).toEqual([
      'curves',
      'hsl',
      'levels',
      'sharpen',
      'threshold',
    ])
    // Sharpen also carries a `threshold` key with a different meaning, so the
    // threshold/sharpen guards must discriminate without dispatch order.
    expect(isRetouchParamsForKind('sharpen', { threshold: 128, invert: false })).toBe(false)
    expect(
      isRetouchParamsForKind('threshold', {
        amount: 10,
        radius: 1,
        threshold: 4,
        protectColor: true,
      }),
    ).toBe(false)
    expect(isRetouchParamsForKind('threshold', { threshold: 10, invert: true })).toBe(true)
    expect(isRetouchParamsForKind('sharpen', { amount: 10, radius: 1, threshold: 4 })).toBe(true)
    expect(isRetouchParamsForKind('curves', { channels: {} })).toBe(true)
    expect(isRetouchParamsForKind('curves', { threshold: 1, invert: true })).toBe(false)
    expect(
      isRetouchParamsForKind('levels', {
        shadows: 0,
        midtones: 128,
        highlights: 255,
        outputBlack: 0,
        outputWhite: 255,
      }),
    ).toBe(true)
    expect(isRetouchParamsForKind('hsl', { hue: 0, saturation: 0, lightness: 0 })).toBe(true)
    expect(getImageRetouchTrait('hsl').defaults).toEqual({ hue: 0, saturation: 0, lightness: 0 })
  })

  it('clones params through the owning trait with per-kind sanitization', () => {
    expect(
      cloneRetouchParamsForKind('sharpen', {
        amount: 240,
        radius: 1,
        threshold: 4,
        protectColor: true,
      }),
    ).toEqual({ amount: 200, radius: 1, threshold: 4, protectColor: true })
    expect(cloneRetouchParamsForKind('threshold', { threshold: -5, invert: true })).toEqual({
      threshold: 0,
      invert: true,
    })
    const clonedDefaults = getImageRetouchTrait('curves').cloneDefaults()
    expect(clonedDefaults).toEqual({ channels: {} })
    expect(clonedDefaults).not.toBe(IMAGE_RETOUCH_DEFINITIONS.curves.defaults)
  })

  it('exposes range labelKeys for call-time t() without baking locale copy', () => {
    // Ranges carry only labelKeys; components translate at render time via
    // t(labelKey) so runtime locale switches stay reactive (same precedent
    // as CurvesControl preset labels and the retouch view-model rail).
    expect(getImageRetouchTrait('hsl').ranges.hue.labelKey).toBe('canvas.retouch.params.hue')
    expect(getImageRetouchTrait('hsl').ranges.saturation.labelKey).toBe(
      'canvas.retouch.params.saturation',
    )
    expect(getImageRetouchTrait('hsl').ranges.lightness.labelKey).toBe(
      'canvas.retouch.params.lightness',
    )
    expect(getImageRetouchTrait('threshold').ranges.threshold.labelKey).toBe(
      'canvas.retouch.params.threshold',
    )
    expect(getImageRetouchTrait('levels').ranges.shadows.labelKey).toBe(
      'canvas.retouch.params.shadows',
    )
    expect(getImageRetouchTrait('levels').ranges.outputWhite.labelKey).toBe(
      'canvas.retouch.params.outputWhite',
    )
    expect(getImageRetouchTrait('sharpen').ranges.amount.labelKey).toBe(
      'canvas.retouch.params.amount',
    )
    for (const trait of Object.values(IMAGE_RETOUCH_DEFINITIONS)) {
      for (const range of Object.values((trait as { ranges: Record<string, unknown> }).ranges)) {
        expect(range).not.toHaveProperty('label')
      }
    }
  })
})
