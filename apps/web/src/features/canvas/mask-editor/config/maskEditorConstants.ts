export const MASK_EDITOR_PAINT_RGB = {
  r: 122,
  g: 107,
  b: 213,
} as const

export const MASK_EDITOR_PAINT_COLOR = `rgba(${MASK_EDITOR_PAINT_RGB.r}, ${MASK_EDITOR_PAINT_RGB.g}, ${MASK_EDITOR_PAINT_RGB.b}, 1)`

export const MASK_EDITOR_PREVIEW_ALPHA = 0.5
export const MASK_EDITOR_EMPTY_ALPHA_THRESHOLD = 0
export const MASK_EDITOR_HISTORY_LIMIT = 40

export const MASK_EDITOR_DEFAULT_BRUSH_SIZE = 18
export const MASK_EDITOR_MIN_BRUSH_SIZE = 4
export const MASK_EDITOR_MAX_BRUSH_SIZE = 48
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
