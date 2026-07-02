import { describe, expect, it } from 'vitest'
import {
  getImageAdjustmentDefaultParams,
  normalizeImageAdjustmentLayer,
} from './imageAdjustmentDefinitions'
import type { ImageEditorAdjustmentLayer } from '../types'

describe('imageAdjustmentDefinitions', () => {
  it('returns defensive copies for default adjustment params', () => {
    const hslDefaults = getImageAdjustmentDefaultParams('hsl')
    const sharpenDefaults = getImageAdjustmentDefaultParams('sharpen')
    hslDefaults.hue = 90
    sharpenDefaults.amount = 120

    expect(getImageAdjustmentDefaultParams('hsl')).toEqual({
      hue: 0,
      saturation: 0,
      lightness: 0,
    })
    expect(getImageAdjustmentDefaultParams('sharpen')).toEqual({
      amount: 0,
      radius: 1,
      threshold: 4,
      protectColor: true,
    })
  })

  it('normalizes levels layers through the definition registry', () => {
    const layer: ImageEditorAdjustmentLayer = {
      id: 'adjustment:levels',
      type: 'adjustment',
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

    const normalized = normalizeImageAdjustmentLayer(layer)

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
    const layer: ImageEditorAdjustmentLayer = {
      id: 'adjustment:sharpen',
      type: 'adjustment',
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

    const normalized = normalizeImageAdjustmentLayer(layer)

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
