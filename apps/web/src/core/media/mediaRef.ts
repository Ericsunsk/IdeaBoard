import { resolveMediaAssetDerivativeURL } from '@/api/media/mediaAssetDelivery'

/** Immutable browser-safe reference to a concrete Media derivative. */
export type MediaRefDerivativeTier = 'thumb' | 'preview' | 'large-preview'

export interface MediaRef {
  assetId: string
  derivativeTier: MediaRefDerivativeTier
}

/** URLs are runtime delivery details, never persisted Canvas facts. */
export function buildMediaBrowserURL(ref: MediaRef): string {
  if (!isMediaRef(ref)) throw new Error('buildMediaBrowserURL: valid MediaRef is required')
  return resolveMediaAssetDerivativeURL(ref.assetId, ref.derivativeTier)
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
  return value === 'thumb' || value === 'preview' || value === 'large-preview'
}
