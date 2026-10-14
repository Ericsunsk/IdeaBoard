import { resolveAssetUrl } from '../../../../core/config/runtimeUrls'
import { isMediaAssetDeliveryPath } from '../../../../core/media/mediaAssetPath'

const HTTP_URL_RE = /^https?:\/\//i
const ENCODED_BACKSLASH_RE = /%5c/i

function getCurrentOrigin(): string {
  if (typeof window !== 'undefined' && window.location.origin) return window.location.origin
  return 'http://ideaboard.local'
}

function parseResultImageUrl(rawUrl: string): URL | null {
  try {
    return new URL(rawUrl, getCurrentOrigin())
  } catch {
    return null
  }
}

// v1 media-asset delivery paths served by the API behind the same-origin
// gateway: .../media-assets/{asset}/content and
// .../media-assets/{asset}/derivatives/{variant}. Nothing else under
// /api/ is renderable here (JSON envelopes must never bind to <img>).
// Strict grammar lives in core/media/mediaAssetPath; this module only adds
// the demo-fixture prefix.
function hasTrustedDeliveryPath(pathname: string): boolean {
  // Frontend-bundled demo fixtures ship with the app shell, never through
  // the upload pipeline.
  if (pathname === '/demo' || pathname.startsWith('/demo/')) return true
  return isMediaAssetDeliveryPath(pathname)
}

// Browser-visible task images must stay on the same-origin delivery path.
function isTrustedDeliveryUrl(url: URL): boolean {
  const currentOrigin = getCurrentOrigin()
  return url.origin === currentOrigin && hasTrustedDeliveryPath(url.pathname)
}

export function getSafeResultImageUrl(rawUrl: string): string {
  const trimmed = rawUrl.trim()
  if (
    !trimmed ||
    trimmed.startsWith('//') ||
    trimmed.includes('\\') ||
    ENCODED_BACKSLASH_RE.test(trimmed)
  ) {
    return ''
  }

  if (!trimmed.startsWith('/') && !HTTP_URL_RE.test(trimmed)) return ''

  const resolved = resolveAssetUrl(trimmed)
  const parsed = parseResultImageUrl(resolved)
  if (!parsed || !isTrustedDeliveryUrl(parsed)) return ''

  return resolved
}

export function getSafeTaskResultImageUrl(result: string): string {
  return getSafeTaskResultImageUrls(result)[0] ?? ''
}

export function getSafeTaskResultImageUrls(result: string | null | undefined): string[] {
  if (!result) return []
  try {
    const parsed = JSON.parse(result)
    if (!parsed || typeof parsed !== 'object') return []
    const record = parsed as Record<string, unknown>
    const collected: string[] = []
    if (typeof record.image_url === 'string') {
      const safe = getSafeResultImageUrl(record.image_url)
      if (safe) collected.push(safe)
    }
    if (Array.isArray(record.image_urls)) {
      for (const candidate of record.image_urls) {
        if (typeof candidate !== 'string') continue
        const safe = getSafeResultImageUrl(candidate)
        if (safe && !collected.includes(safe)) collected.push(safe)
      }
    }
    return collected
  } catch {
    return []
  }
}

export function hasTaskResultImageCandidate(result: string | null | undefined): boolean {
  if (!result) return false
  try {
    const parsed = JSON.parse(result)
    if (!parsed || typeof parsed !== 'object') return false

    const { image_url: imageUrl, image_urls: imageUrls } = parsed as Record<string, unknown>
    return (
      typeof imageUrl === 'string' ||
      (Array.isArray(imageUrls) && imageUrls.some((candidate) => typeof candidate === 'string'))
    )
  } catch {
    return false
  }
}

export function isTaskResultImageDeletedByUser(result: string | null | undefined): boolean {
  if (!result) return false
  try {
    const parsed = JSON.parse(result)
    return (
      parsed != null &&
      typeof parsed === 'object' &&
      (parsed as Record<string, unknown>).image_availability === 'deleted_by_user'
    )
  } catch {
    return false
  }
}
