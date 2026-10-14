// Display-tier selection for canvas image delivery (Public Data Plane, `/api/v1`).
//
// Pure width-to-tier mapping only: no HTTP, no metadata cache, no Vue
// lifecycle. The max-side boundaries mirror the backend `TierSpecs`
// (`apps/api/internal/media/derivatives/domain/model.go`: thumb 256,
// preview 640, large-preview 1280) so canvas display URLs cannot drift from
// the rendered tiers. Anything wider than the largest display tier maps
// to `original` (the `/content` byte route), never a larger derivative.

const MEDIA_DISPLAY_TIER_MAX_SIDE = {
  thumb: 256,
  preview: 640,
  'large-preview': 1280,
} as const

export type MediaDisplayTier = 'thumb' | 'preview' | 'large-preview' | 'original'

/**
 * Maps effective display pixels to the smallest tier that covers them:
 * ≤256 thumb, ≤640 preview, ≤1280 large-preview, else original (`/content`).
 * Unknown/non-positive widths fall back to `large-preview`, matching the
 * canvas default preview side (1280): prefer the durable edit-quality tier
 * over an undersized guess.
 */
export function resolveMediaDisplayTier(effectiveWidthPx: unknown): MediaDisplayTier {
  const width = Number(effectiveWidthPx)
  if (!Number.isFinite(width) || width <= 0) return 'large-preview'
  if (width <= MEDIA_DISPLAY_TIER_MAX_SIDE.thumb) return 'thumb'
  if (width <= MEDIA_DISPLAY_TIER_MAX_SIDE.preview) return 'preview'
  if (width <= MEDIA_DISPLAY_TIER_MAX_SIDE['large-preview']) return 'large-preview'
  return 'original'
}
