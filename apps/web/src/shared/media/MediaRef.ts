import { resolveMediaAssetDerivativeURL } from '../api/mediaAssetDelivery'

/** Immutable browser-safe reference to a concrete Media derivative. */
export type MediaRefDerivativeTier =
  'thumb' | 'preview' | 'large-preview' | 'processing' | 'processing-input'

export type MediaRefDerivativeFormat = 'webp' | 'jpeg' | 'png'

export interface MediaRef {
  assetId: string
  derivativeTier: MediaRefDerivativeTier
  derivativeFormat: MediaRefDerivativeFormat
}

/** URLs are runtime delivery details, never persisted Canvas facts. */
export function buildMediaBrowserURL(ref: MediaRef): string {
  if (!isMediaRef(ref)) throw new Error('buildMediaBrowserURL: valid MediaRef is required')
  return resolveMediaAssetDerivativeURL(ref.assetId, ref.derivativeTier, ref.derivativeFormat)
}

export function isMediaRef(value: unknown): value is MediaRef {
  if (!value || typeof value !== 'object') return false
  const candidate = value as Record<string, unknown>
  return (
    typeof candidate.assetId === 'string' &&
    candidate.assetId.trim().length > 0 &&
    isDerivativeTier(candidate.derivativeTier) &&
    isDerivativeFormat(candidate.derivativeFormat)
  )
}

export function mediaRefEquals(
  a: MediaRef | null | undefined,
  b: MediaRef | null | undefined,
): boolean {
  return (
    a === b ||
    (!!a &&
      !!b &&
      a.assetId === b.assetId &&
      a.derivativeTier === b.derivativeTier &&
      a.derivativeFormat === b.derivativeFormat)
  )
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

function isDerivativeFormat(value: unknown): value is MediaRefDerivativeFormat {
  return value === 'webp' || value === 'jpeg' || value === 'png'
}
