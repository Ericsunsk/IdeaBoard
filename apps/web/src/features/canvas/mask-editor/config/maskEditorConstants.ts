import type { ImageMaskBrushShape } from '../types'

export const MASK_EDITOR_PAINT_RGB = {
  r: 122,
  g: 107,
  b: 213,
} as const

export const MASK_EDITOR_PAINT_COLOR = `rgba(${MASK_EDITOR_PAINT_RGB.r}, ${MASK_EDITOR_PAINT_RGB.g}, ${MASK_EDITOR_PAINT_RGB.b}, 1)`

export const MASK_EDITOR_PREVIEW_ALPHA = 0.5
export const MASK_EDITOR_EMPTY_ALPHA_THRESHOLD = 0
export const MASK_EDITOR_MIN_BRUSH_STAMP_SPACING = 0.75
export const MASK_EDITOR_BRUSH_STAMP_PADDING = 2
export const MASK_EDITOR_HISTORY_LIMIT = 40
export const MASK_EDITOR_MAX_FEATHER_RADIUS = 16
export const MASK_EDITOR_DEFAULT_FEATHER_STRENGTH = 35
export const MASK_EDITOR_MIN_FEATHER_STRENGTH = 0
export const MASK_EDITOR_MAX_FEATHER_STRENGTH = 100

export const MASK_EDITOR_DEFAULT_BRUSH_SIZE = 18
export const MASK_EDITOR_MIN_BRUSH_SIZE = 4
export const MASK_EDITOR_MAX_BRUSH_SIZE = 48
export const MASK_EDITOR_DEFAULT_BRUSH_HARDNESS = 70
export const MASK_EDITOR_MIN_BRUSH_HARDNESS = 0
export const MASK_EDITOR_MAX_BRUSH_HARDNESS = 100
export const MASK_EDITOR_DEFAULT_LAYER_OPACITY = 72
export const MASK_EDITOR_MIN_LAYER_OPACITY = 15
export const MASK_EDITOR_MAX_LAYER_OPACITY = 100

export function clampMaskBrushSize(
  value: number,
  fallback = MASK_EDITOR_DEFAULT_BRUSH_SIZE,
): number {
  const next = Number(value)
  const resolved = Number.isFinite(next) && next > 0 ? next : fallback
  return Math.max(MASK_EDITOR_MIN_BRUSH_SIZE, Math.min(MASK_EDITOR_MAX_BRUSH_SIZE, resolved))
}

export function clampMaskBrushHardness(
  value: number,
  fallback = MASK_EDITOR_DEFAULT_BRUSH_HARDNESS,
): number {
  const next = Number(value)
  const resolved = Number.isFinite(next) ? next : fallback
  return Math.max(
    MASK_EDITOR_MIN_BRUSH_HARDNESS,
    Math.min(MASK_EDITOR_MAX_BRUSH_HARDNESS, resolved),
  )
}

export function normalizeMaskBrushShape(
  value: ImageMaskBrushShape | undefined,
): ImageMaskBrushShape {
  return value === 'square' ? 'square' : 'round'
}

export function clampMaskFeatherStrength(
  strength: number,
  fallback = MASK_EDITOR_DEFAULT_FEATHER_STRENGTH,
): number {
  const next = Number(strength)
  const resolved = Number.isFinite(next) ? next : fallback
  return Math.max(
    MASK_EDITOR_MIN_FEATHER_STRENGTH,
    Math.min(MASK_EDITOR_MAX_FEATHER_STRENGTH, Math.round(resolved)),
  )
}

export function clampMaskLayerOpacity(
  value: number,
  fallback = MASK_EDITOR_DEFAULT_LAYER_OPACITY,
): number {
  const next = Number(value)
  const resolved = Number.isFinite(next) ? next : fallback
  return Math.max(
    MASK_EDITOR_MIN_LAYER_OPACITY,
    Math.min(MASK_EDITOR_MAX_LAYER_OPACITY, Math.round(resolved)),
  )
}

export function resolveMaskFeatherRadius(strength: number): number {
  const clamped = clampMaskFeatherStrength(strength)
  if (clamped <= 0) return 0
  return Math.max(1, Math.round((clamped / 100) * MASK_EDITOR_MAX_FEATHER_RADIUS))
}

export const MASK_EDITOR_DEFAULT_FEATHER_RADIUS = resolveMaskFeatherRadius(
  MASK_EDITOR_DEFAULT_FEATHER_STRENGTH,
)
