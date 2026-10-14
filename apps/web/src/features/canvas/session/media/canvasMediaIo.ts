/** Shared media URL/Blob/Canvas utilities used across canvas services. */
import {
  API_BASE_URL,
  canonicalizeRuntimeUploadRef,
  resolveAssetUrl,
} from '../../../../core/config/runtimeUrls'
import { isMediaAssetDeliveryPath } from '../../../../core/media/mediaAssetPath'
import { createUniqueSuffix } from '../../../../core/utils/ids'

/**
 * Returns true if the value is a data: URL.
 * Accepts trimmed input (all callers trim), but also handles untrimmed.
 */
export function isDataUrl(value: string): boolean {
  return /^data:/i.test(value.trim())
}

/**
 * Parses a raw URL string with window.location.origin fallback.
 * Returns null for invalid or empty input.
 */
function toAbsoluteUrl(raw: string): URL | null {
  const normalized = resolveAssetUrl(canonicalizeRuntimeUploadRef(raw.trim()).trim())
  if (!normalized) return null
  try {
    return new URL(normalized, window.location.origin)
  } catch {
    return null
  }
}

/**
 * Parses any image URL to its pathname for stable-upload matching.
 * Relative and absolute URLs resolve with the page origin as fallback.
 * Returns null for empty or unparseable input.
 */
export function parseImageUrlPathname(raw: string): string | null {
  return toAbsoluteUrl(raw)?.pathname ?? null
}

function encodePathSegment(segment: string): string {
  return encodeURIComponent(segment).replace(
    /[!'()*]/g,
    (ch) => `%${ch.charCodeAt(0).toString(16).toUpperCase()}`,
  )
}

function pathSegmentMatches(segment: string, expected: string): boolean {
  const encoded = encodePathSegment(expected)
  if (segment === expected || segment === encoded) return true
  try {
    return decodeURIComponent(segment) === expected
  } catch {
    return false
  }
}

function isBoardTaskUploadPath(path: string, userUid: string, boardId: string): boolean {
  const segments = path.split('/').filter(Boolean)
  for (let i = 0; i + 4 < segments.length; i += 1) {
    if (segments[i] !== 'uploads' || segments[i + 1] !== 'tasks') continue
    if (!pathSegmentMatches(segments[i + 2] ?? '', userUid)) continue
    if (!pathSegmentMatches(segments[i + 3] ?? '', boardId)) continue
    if (segments[i + 4] !== 'content') continue
    return true
  }
  return false
}

/**
 * Returns true when a canvas image URL is already a stable board-owned
 * upload (`/uploads/tasks/{userUid}/{boardId}/content/...`, historical
 * snapshots) or a v1 media-asset delivery URL (durable asset bytes) that
 * needs no stabilization re-upload. Empty refs count as stable (nothing
 * to fetch). Absolute CDN/loopback upload refs canonicalize before
 * matching.
 */
export function isStableBoardUploadUrl(raw: string, userUid: string, boardId: string): boolean {
  const trimmed = raw.trim()
  if (!trimmed) return true
  const path = parseImageUrlPathname(trimmed)
  if (!path) return false
  if (isMediaAssetDeliveryPath(path)) return true
  return isBoardTaskUploadPath(path, userUid.trim(), boardId.trim())
}

/**
 * Transport error for shared image-blob reads. `status` is the HTTP
 * status for non-ok responses, or null when the network fetch itself
 * failed. Callers map this to surface copy; the helper never touches
 * i18n so domain and display layers keep their own messages.
 */
export class ImageBlobFetchError extends Error {
  readonly status: number | null

  constructor(status: number | null, options?: { cause?: unknown }) {
    super(
      status === null ? 'Image download failed' : `Image download failed with status ${status}`,
      options,
    )
    this.name = 'ImageBlobFetchError'
    this.status = status
  }
}

/**
 * Shared image-blob fetch core: same-origin/API URLs ride the gateway
 * with credentials, data:/external URLs pass through untouched. Throws
 * `ImageBlobFetchError` on network failure (`status: null`) or non-ok
 * responses; callers translate to their own copy.
 */
export async function fetchImageBlobFromUrl(
  fetchUrl: string,
  options?: { signal?: AbortSignal | undefined },
): Promise<Blob> {
  let response: Response
  try {
    response = await fetch(
      fetchUrl,
      options?.signal
        ? { credentials: 'include', signal: options.signal }
        : { credentials: 'include' },
    )
  } catch (error) {
    throw new ImageBlobFetchError(null, { cause: error })
  }
  if (!response.ok) {
    throw new ImageBlobFetchError(response.status)
  }
  return response.blob()
}

/**
 * Resolves an image URL for fetch.
 * - Data URLs pass through.
 * - Same-origin /uploads paths can pass through for local mask/inpaint read.
 * - v1 workspace media URLs stay on the API origin (authenticated direct
 *   read through the same-origin gateway; no legacy download proxy).
 * - External http/https URLs are fetched directly.
 * - options.blockExternal can prevent external URL resolution.
 */
function apiOrigin(): string {
  try {
    return new URL(API_BASE_URL, window.location.origin).origin
  } catch {
    return window.location.origin
  }
}

function resolveRemoteImageUrl(
  url: URL,
  trimmed: string,
  options?: { shouldAllowSameOriginUploads?: boolean; shouldBlockExternal?: boolean },
): string {
  if (url.protocol === 'http:' || url.protocol === 'https:') {
    if (options?.shouldBlockExternal) return ''
    return trimmed
  }
  return trimmed
}

export function resolveFetchUrl(
  raw: string,
  options?: { shouldAllowSameOriginUploads?: boolean; shouldBlockExternal?: boolean },
): string {
  const trimmed = resolveAssetUrl(canonicalizeRuntimeUploadRef(raw.trim()).trim())
  if (!trimmed) return ''
  if (/^data:/i.test(trimmed)) return trimmed
  const url = toAbsoluteUrl(trimmed)
  if (!url) return ''
  const origin = apiOrigin()
  const isSameOrigin = url.origin === window.location.origin
  const isApiOrigin = url.origin === origin
  if (
    (isSameOrigin || isApiOrigin) &&
    (options?.shouldAllowSameOriginUploads ?? true) &&
    url.pathname.startsWith('/uploads')
  ) {
    return url.toString()
  }
  // API Media endpoints are authenticated resources. Keep them on the API
  // origin instead of sending them through the remote-image download proxy.
  if (isApiOrigin) return url.toString()
  if (isSameOrigin) return trimmed
  return resolveRemoteImageUrl(url, trimmed, options)
}

/** Convert a canvas to a Blob for upload. */
export function canvasToBlob(
  canvas: HTMLCanvasElement,
  type = 'image/png',
  quality?: number,
): Promise<Blob> {
  return new Promise<Blob>((resolve, reject) => {
    canvas.toBlob(
      (blob) => {
        if (blob) resolve(blob)
        else reject(new Error('Canvas toBlob returned null'))
      },
      type,
      quality,
    )
  })
}

/** Guess a file extension from a MIME type string. */
export function inferImageExtension(mime: string): string {
  const value = mime.toLowerCase()
  if (value.includes('png')) return 'png'
  if (value.includes('webp')) return 'webp'
  if (value.includes('gif')) return 'gif'
  if (value.includes('bmp')) return 'bmp'
  if (value.includes('avif')) return 'avif'
  if (value.includes('heic')) return 'heic'
  if (value.includes('heif')) return 'heif'
  if (value.includes('svg')) return 'svg'
  if (value.includes('jpeg') || value.includes('jpg')) return 'jpg'
  return 'jpg'
}

/** Derive a filename from a URL and MIME type. */
export function filenameFromImageUrl(raw: string, mime: string, fallbackPrefix = 'image'): string {
  const ext = inferImageExtension(mime)
  const stamp = createUniqueSuffix()
  if (isDataUrl(raw)) {
    return `flow-mask-${stamp}.${ext}`
  }
  const url = toAbsoluteUrl(raw)
  if (!url) return `flow-stable-${stamp}.${ext}`
  const segments = url.pathname.split('/').filter(Boolean)
  const last = segments[segments.length - 1] || fallbackPrefix
  const decodedLast = (() => {
    try {
      return decodeURIComponent(last)
    } catch {
      return last
    }
  })()
  const safeLast = decodedLast.replace(/[^\w.-]+/g, '-').replace(/^-+|-+$/g, '')
  const hasExt = /\.[A-Za-z0-9]{1,10}$/.test(safeLast)
  if (!safeLast || !hasExt) return `flow-stable-${stamp}.${ext}`
  const base = safeLast.replace(/\.[^.]+$/, '')
  return `${base}.${ext}`
}
