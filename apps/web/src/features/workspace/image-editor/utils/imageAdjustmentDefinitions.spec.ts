import { describe, expect, it } from 'vitest'
import {
  getImageAdjustmentDefaultParams,
  normalizeImageAdjustmentLayer,
} from './imageAdjustmentDefinitions'
import type { ImageEditorAdjustmentLayer } from '../types'

describe('imageAdjustmentDefinitions', () => {
  it('returns defensive copies for default adjustment params', () => {
    const hslDefaults = getImageAdjustmentDefaultParams('hsl')
    hslDefaults.hue = 90

    expect(getImageAdjustmentDefaultParams('hsl')).toEqual({
      hue: 0,
      saturation: 0,
      brightness: 0,
      contrast: 0,
    })
  })

  it('keeps levels gamma finite for programmatic boundary input', () => {
    const layer: ImageEditorAdjustmentLayer = {
      id: 'adjustment:levels',
      type: 'adjustment',
      kind: 'levels',
      name: '色阶',
      visible: true,
      params: {
        shadows: 0,
        midtones: 100,
        highlights: 100,
      },
    }

    const normalized = normalizeImageAdjustmentLayer(layer)

    expect(normalized.kind).toBe('levels')
    if (normalized.kind !== 'levels') return
    expect(Number.isFinite(normalized.params.gamma)).toBe(true)
  })

  it('uses the default levels gamma for non-finite midtones', () => {
    const layer: ImageEditorAdjustmentLayer = {
      id: 'adjustment:levels',
      type: 'adjustment',
      kind: 'levels',
      name: '色阶',
      visible: true,
      params: {
        shadows: 0,
        midtones: Number.NaN,
        highlights: 100,
      },
    }

    const normalized = normalizeImageAdjustmentLayer(layer)

    expect(normalized.kind).toBe('levels')
    if (normalized.kind !== 'levels') return
    expect(normalized.params.gamma).toBe(1)
  })
})
