import { describe, expect, it } from 'vitest'
import {
  clampMaskBrushHardness,
  clampMaskBrushSize,
  clampMaskFeatherStrength,
  clampMaskLayerOpacity,
  MASK_EDITOR_DEFAULT_BRUSH_HARDNESS,
  MASK_EDITOR_DEFAULT_FEATHER_RADIUS,
  normalizeMaskBrushShape,
  resolveMaskFeatherRadius,
} from './maskEditorConstants'

describe('maskEditorConstants', () => {
  it('clamps brush size and hardness to editor bounds', () => {
    expect(clampMaskBrushSize(2)).toBe(4)
    expect(clampMaskBrushSize(80)).toBe(48)
    expect(clampMaskBrushSize(Number.NaN)).toBe(18)
    expect(clampMaskBrushHardness(-5)).toBe(0)
    expect(clampMaskBrushHardness(120)).toBe(100)
    expect(clampMaskBrushHardness(Number.NaN)).toBe(MASK_EDITOR_DEFAULT_BRUSH_HARDNESS)
    expect(clampMaskBrushHardness(Number.NaN, 70)).toBe(70)
  })

  it('normalizes brush shape and feather strength', () => {
    expect(normalizeMaskBrushShape('square')).toBe('square')
    expect(normalizeMaskBrushShape('round')).toBe('round')
    expect(clampMaskFeatherStrength(-10)).toBe(0)
    expect(clampMaskFeatherStrength(120)).toBe(100)
    expect(clampMaskFeatherStrength(Number.NaN)).toBe(35)
    expect(resolveMaskFeatherRadius(0)).toBe(0)
    expect(resolveMaskFeatherRadius(35)).toBe(6)
    expect(MASK_EDITOR_DEFAULT_FEATHER_RADIUS).toBe(resolveMaskFeatherRadius(35))
    expect(resolveMaskFeatherRadius(100)).toBe(16)
  })

  it('clamps mask layer opacity to editor bounds', () => {
    expect(clampMaskLayerOpacity(5)).toBe(15)
    expect(clampMaskLayerOpacity(150)).toBe(100)
    expect(clampMaskLayerOpacity(Number.NaN)).toBe(72)
  })
})
