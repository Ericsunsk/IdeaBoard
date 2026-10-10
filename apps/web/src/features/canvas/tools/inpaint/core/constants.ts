import { clamp } from '../../../../../core/utils/number.ts'

export const MASK_PAINTING_RGB = {
  r: 122,
  g: 107,
  b: 213,
} as const

export const MASK_PAINTING_COLOR = `rgba(${MASK_PAINTING_RGB.r}, ${MASK_PAINTING_RGB.g}, ${MASK_PAINTING_RGB.b}, 1)`

export const MASK_PAINTING_PREVIEW_ALPHA = 0.5
export const MASK_PAINTING_EMPTY_ALPHA_THRESHOLD = 0
export const INPAINT_HISTORY_LIMIT = 40

/**
 * Pending-stroke buffer cap per node while the painter loads. Drags that
 * finish before Fabric resolves buffer as one-shot strokes; the cap keeps
 * the newest strokes and drops the oldest so a slow load cannot grow memory
 * without bound.
 */
export const INPAINT_MAX_PENDING_STROKES_PER_NODE = 10

/**
 * Per-drag sample cap. Moves past the cap thin the buffered shape (keep
 * every other sample) instead of truncating the tail; the live preview
 * already rendered every sample, the buffer only feeds the one-shot
 * fallback when no live session exists.
 */
export const INPAINT_MAX_POINTS_PER_STROKE = 2048

/**
 * Pointermove decimation floor. Samples closer than this to the previous
 * accepted sample are dropped from both the buffer and the live preview.
 * Below the minimum brush width, so rendering is visually identical.
 */
export const INPAINT_STROKE_MIN_MOVE_DISTANCE_PX = 2

export const INPAINT_DEFAULT_BRUSH_SIZE = 18
export const INPAINT_MIN_BRUSH_SIZE = 4
export const INPAINT_MAX_BRUSH_SIZE = 48
export const INPAINT_DEFAULT_LAYER_OPACITY = 72
export const INPAINT_MIN_LAYER_OPACITY = 15
export const INPAINT_MAX_LAYER_OPACITY = 100

export function clampMaskBrushSize(value: number, fallback = INPAINT_DEFAULT_BRUSH_SIZE): number {
  const next = Number(value)
  const resolved = Number.isFinite(next) && next > 0 ? next : fallback
  return clamp(resolved, INPAINT_MIN_BRUSH_SIZE, INPAINT_MAX_BRUSH_SIZE)
}

export function clampMaskLayerOpacity(
  value: number,
  fallback = INPAINT_DEFAULT_LAYER_OPACITY,
): number {
  const next = Number(value)
  const resolved = Number.isFinite(next) ? next : fallback
  return clamp(Math.round(resolved), INPAINT_MIN_LAYER_OPACITY, INPAINT_MAX_LAYER_OPACITY)
}
