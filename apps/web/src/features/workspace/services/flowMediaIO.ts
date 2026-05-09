/** Shared media URL/Blob/Canvas utilities used across workspace services. */
import { USER_DOWNLOAD_ENDPOINT } from '../../../shared/constants/media'
import { uploadApi } from '../../../shared/api'

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
export function toAbsoluteUrl(raw: string): URL | null {
  if (!raw.trim()) return null
  try {
    return new URL(raw, window.location.origin)
  } catch {
    return null
  }
}

/**
 * Resolves an image URL for fetch.
 * - Data URLs pass through.
 * - Same-origin /api/user/download URLs are normalized (no nested proxying).
 * - Same-origin /uploads paths can pass through for local mask/inpaint read.
 * - External http/https URLs are proxied through USER_DOWNLOAD_ENDPOINT.
 * - options.blockExternal can prevent external URL resolution.
 */
export function resolveFetchUrl(
  raw: string,
  options?: { allowSameOriginUploads?: boolean; blockExternal?: boolean },
): string {
  const trimmed = raw.trim()
  if (!trimmed) return ''
  if (/^data:/i.test(trimmed)) return trimmed
  const url = toAbsoluteUrl(trimmed)
  if (!url) return ''
  const endpoint = new URL(USER_DOWNLOAD_ENDPOINT, window.location.origin)
  const isSameOrigin = url.origin === window.location.origin
  if (isSameOrigin && url.pathname === endpoint.pathname) {
    const target = (url.searchParams.get('url') || '').trim()
    if (!target) return ''
    return `${USER_DOWNLOAD_ENDPOINT}?url=${encodeURIComponent(target)}`
  }
  if (isSameOrigin && (options?.allowSameOriginUploads ?? true) && url.pathname.startsWith('/uploads')) {
    return trimmed
  }
  if (isSameOrigin) return trimmed
  if (url.protocol === 'http:' || url.protocol === 'https:') {
    if (options?.blockExternal) return ''
    return `${USER_DOWNLOAD_ENDPOINT}?url=${encodeURIComponent(trimmed)}`
  }
  return trimmed
}

/** Convert a canvas to a Blob for upload. */
export function canvasToBlob(canvas: HTMLCanvasElement, type = 'image/png', quality?: number): Promise<Blob> {
  return new Promise<Blob>((resolve, reject) => {
    canvas.toBlob((blob) => {
      if (blob) resolve(blob)
      else reject(new Error('Canvas toBlob returned null'))
    }, type, quality)
  })
}

/** Upload a generated image Blob to the workspace upload API. */
export async function uploadWorkspaceBlob(blob: Blob, fileName: string, boardId?: string): Promise<string> {
  const file = new File([blob], fileName, { type: blob.type || 'image/png' })
  const res = await uploadApi.upload(file, boardId)
  if (res.code !== 0 || !res.data?.url) {
    throw new Error(res.msg || '上传图片失败')
  }
  return res.data.url
}

/** Guess a file extension from a MIME type string. */
export function inferImageExtension(mime: string): string {
  const value = mime.toLowerCase()
  if (value.includes('png')) return 'png'
  if (value.includes('webp')) return 'webp'
  if (value.includes('gif')) return 'gif'
  if (value.includes('bmp')) return 'bmp'
  if (value.includes('svg')) return 'svg'
  if (value.includes('jpeg') || value.includes('jpg')) return 'jpg'
  return 'jpg'
}

/** Derive a filename from a URL and MIME type. */
export function filenameFromImageUrl(raw: string, mime: string, fallbackPrefix = 'image'): string {
  const ext = inferImageExtension(mime)
  const stamp = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
  if (isDataUrl(raw)) {
    return `flow-mask-${stamp}.${ext}`
  }
  const url = toAbsoluteUrl(raw)
  if (!url) return `flow-stable-${stamp}.${ext}`
  const segments = url.pathname.split('/').filter(Boolean)
  const last = segments[segments.length - 1] || fallbackPrefix
  let decodedLast = last
  try {
    decodedLast = decodeURIComponent(last)
  } catch {
    decodedLast = last
  }
  const safeLast = decodedLast.replace(/[^\w.-]+/g, '-').replace(/^-+|-+$/g, '')
  const hasExt = /\.[A-Za-z0-9]{1,10}$/.test(safeLast)
  if (!safeLast || !hasExt) return `flow-stable-${stamp}.${ext}`
  const base = safeLast.replace(/\.[^.]+$/, '')
  return `${base}.${ext}`
}
