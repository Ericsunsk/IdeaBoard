export const MASK_PAINTING_RGB = {
  r: 122,
  g: 107,
  b: 213,
} as const

export const MASK_PAINTING_COLOR = `rgba(${MASK_PAINTING_RGB.r}, ${MASK_PAINTING_RGB.g}, ${MASK_PAINTING_RGB.b}, 1)`

export const MASK_PAINTING_PREVIEW_ALPHA = 0.5
export const MASK_PAINTING_EMPTY_ALPHA_THRESHOLD = 0
export const INPAINT_HISTORY_LIMIT = 40

export const INPAINT_DEFAULT_BRUSH_SIZE = 18
export const INPAINT_MIN_BRUSH_SIZE = 4
export const INPAINT_MAX_BRUSH_SIZE = 48
export const INPAINT_DEFAULT_LAYER_OPACITY = 72
export const INPAINT_MIN_LAYER_OPACITY = 15
export const INPAINT_MAX_LAYER_OPACITY = 100

export function clampMaskBrushSize(
  value: number,
  fallback = INPAINT_DEFAULT_BRUSH_SIZE,
): number {
  const next = Number(value)
  const resolved = Number.isFinite(next) && next > 0 ? next : fallback
  return Math.max(INPAINT_MIN_BRUSH_SIZE, Math.min(INPAINT_MAX_BRUSH_SIZE, resolved))
}

export function clampMaskLayerOpacity(
  value: number,
  fallback = INPAINT_DEFAULT_LAYER_OPACITY,
): number {
  const next = Number(value)
  const resolved = Number.isFinite(next) ? next : fallback
  return Math.max(
    INPAINT_MIN_LAYER_OPACITY,
    Math.min(INPAINT_MAX_LAYER_OPACITY, Math.round(resolved)),
  )
}
