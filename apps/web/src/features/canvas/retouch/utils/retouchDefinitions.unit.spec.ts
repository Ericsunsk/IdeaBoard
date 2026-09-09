import { describe, expect, it } from 'vitest'
import { getImageRetouchDefaultParams, normalizeImageEffectLayer } from './retouchDefinitions'
import type { RetouchEffectLayer } from '../types'

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
})
