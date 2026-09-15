import { resolveMediaAssetDerivativeURL } from '../api/mediaAssetDelivery'

/** Immutable browser-safe reference to a concrete Media derivative. */
export type MediaRefDerivativeTier =
  'thumb' | 'preview' | 'large-preview' | 'processing' | 'processing-input'

/**
 * Legacy display tier retired by the pull-model delivery (ADR-0007): the
 * backend no longer serves it, so it normalizes to the largest display tier
 * at URL build time. The union member stays so persisted board facts carrying
 * `processing` still parse; new code must use `large-preview` directly.
 */
function deliveryTierForRef(
  tier: MediaRefDerivativeTier,
): 'thumb' | 'preview' | 'large-preview' | 'processing-input' {
  if (tier === 'processing') return 'large-preview'
  return tier
}

export interface MediaRef {
  assetId: string
  derivativeTier: MediaRefDerivativeTier
}

/** URLs are runtime delivery details, never persisted Canvas facts. */
export function buildMediaBrowserURL(ref: MediaRef): string {
  if (!isMediaRef(ref)) throw new Error('buildMediaBrowserURL: valid MediaRef is required')
  return resolveMediaAssetDerivativeURL(ref.assetId, deliveryTierForRef(ref.derivativeTier))
}

export function isMediaRef(value: unknown): value is MediaRef {
  if (!value || typeof value !== 'object') return false
  const candidate = value as Record<string, unknown>
  return (
    typeof candidate.assetId === 'string' &&
    candidate.assetId.trim().length > 0 &&
    isDerivativeTier(candidate.derivativeTier)
  )
}

export function mediaRefEquals(
  a: MediaRef | null | undefined,
  b: MediaRef | null | undefined,
): boolean {
  return a === b || (!!a && !!b && a.assetId === b.assetId && a.derivativeTier === b.derivativeTier)
}

function isDerivativeTier(value: unknown): value is MediaRefDerivativeTier {
  return (
    value === 'thumb' ||
    value === 'preview' ||
    value === 'large-preview' ||
    value === 'processing' ||
    value === 'processing-input'
  )
}
