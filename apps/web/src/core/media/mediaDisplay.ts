import { resolveApiUrl } from '../config/runtimeUrls'
import { isRecord } from '../utils/guards'
import { resolveMediaDisplayTier, type MediaDisplayTier } from './displayTier'

type PreviewTier = Exclude<MediaDisplayTier, 'original'>

export type MediaDisplay = {
  variants: Array<{ tier: PreviewTier; max_side: number; url: string }>
  expires_at?: string | undefined
}

export function toMediaDisplay(raw: unknown): MediaDisplay | undefined {
  if (raw == null) return undefined
  if (!isRecord(raw) || !Array.isArray(raw.variants)) throw new Error('Invalid media display')
  const variants = raw.variants.map((variant: unknown): MediaDisplay['variants'][number] => {
    if (
      !isRecord(variant) ||
      (variant.tier !== 'thumb' &&
        variant.tier !== 'preview' &&
        variant.tier !== 'large-preview') ||
      typeof variant.max_side !== 'number' ||
      !Number.isFinite(variant.max_side) ||
      variant.max_side <= 0 ||
      typeof variant.url !== 'string' ||
      !variant.url.startsWith('/api/')
    ) {
      throw new Error('Invalid media display variant')
    }
    return { tier: variant.tier, max_side: variant.max_side, url: variant.url }
  })
  const expiresAt = raw.expires_at
  if (
    expiresAt != null &&
    (typeof expiresAt !== 'string' || !Number.isFinite(Date.parse(expiresAt)))
  ) {
    throw new Error('Invalid media display expiry')
  }
  return { variants, ...(expiresAt == null ? {} : { expires_at: expiresAt }) }
}

export function isMediaDisplayExpired(
  display: MediaDisplay | undefined,
  now = Date.now(),
): boolean {
  return display?.expires_at != null && Date.parse(display.expires_at) <= now
}

export function resolveMediaDisplayUrl(
  display: MediaDisplay | undefined,
  effectiveLongestSide: number,
): string {
  const variants = display?.variants
  if (!variants?.length || isMediaDisplayExpired(display)) return ''
  const tier = resolveMediaDisplayTier(effectiveLongestSide)
  const requested = tier === 'original' ? 'large-preview' : tier
  const url = variants.find((variant) => variant.tier === requested)?.url
  return url ? resolveApiUrl(url.slice('/api'.length)) : ''
}

export function fitMediaDisplaySize(
  source: { width: number; height: number } | null,
  maxWidth: number,
  maxHeight: number,
): { width: number; height: number } {
  if (!source || source.width <= 0 || source.height <= 0) {
    return { width: maxWidth, height: maxHeight }
  }
  const scale = Math.min(maxWidth / source.width, maxHeight / source.height)
  return { width: source.width * scale, height: source.height * scale }
}
