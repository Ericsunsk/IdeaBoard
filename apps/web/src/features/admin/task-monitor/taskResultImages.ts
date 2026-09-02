import { resolveAssetUrl } from '../../../shared/config/runtimeUrls'

const HTTP_URL_RE = /^https?:\/\//i
const UPLOAD_ASSET_PATH_PREFIX = '/uploads/'
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

function hasUploadAssetPath(pathname: string): boolean {
  return pathname.startsWith(UPLOAD_ASSET_PATH_PREFIX)
}

// Browser-visible task images must stay on the same-origin upload path.
function isTrustedUploadUrl(url: URL): boolean {
  const currentOrigin = getCurrentOrigin()
  return url.origin === currentOrigin && hasUploadAssetPath(url.pathname)
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
  if (!parsed || !isTrustedUploadUrl(parsed)) return ''

  return resolved
}

export function getSafeTaskResultImageUrl(result: string): string {
  try {
    const parsed = JSON.parse(result)
    if (parsed && typeof parsed === 'object') {
      const imageUrl = (parsed as Record<string, unknown>).image_url
      const safeImageUrl = typeof imageUrl === 'string' ? getSafeResultImageUrl(imageUrl) : ''
      if (safeImageUrl) return safeImageUrl

      const imageUrls = (parsed as Record<string, unknown>).image_urls
      if (Array.isArray(imageUrls)) {
        for (const candidate of imageUrls) {
          if (typeof candidate !== 'string') continue
          const safeCandidate = getSafeResultImageUrl(candidate)
          if (safeCandidate) return safeCandidate
        }
      }
    }
  } catch {
    return ''
  }
  return ''
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
