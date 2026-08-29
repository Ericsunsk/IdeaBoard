import { describe, expect, it } from 'vitest'
import {
  clampMaskBrushSize,
  clampMaskLayerOpacity,
} from './inpaintConstants'

describe('inpaintConstants', () => {
  it('clamps brush size to editor bounds', () => {
    expect(clampMaskBrushSize(2)).toBe(4)
    expect(clampMaskBrushSize(80)).toBe(48)
    expect(clampMaskBrushSize(Number.NaN)).toBe(18)
  })

  it('clamps mask layer opacity to editor bounds', () => {
    expect(clampMaskLayerOpacity(5)).toBe(15)
    expect(clampMaskLayerOpacity(150)).toBe(100)
    expect(clampMaskLayerOpacity(Number.NaN)).toBe(72)
  })
})
