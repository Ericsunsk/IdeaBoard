import { USER_DOWNLOAD_ENDPOINT } from '../../../shared/constants/media'
import { resolveMediaAssetOriginalURL } from '../../../shared/api/mediaAssetDelivery'

export type CanvasGraphInteractionBrowserAdapter = {
  downloadImage: (request: {
    url: string
    mediaAssetId?: number
    fileName: string
  }) => Promise<void> | void
}

function buildDownloadHref(
  rawUrl: string,
  mediaAssetId: number | undefined,
  ownerDocument: Document,
): string {
  if (mediaAssetId && Number.isInteger(mediaAssetId) && mediaAssetId > 0) {
    return resolveMediaAssetOriginalURL(mediaAssetId)
  }
  const url = rawUrl.trim()
  if (!url) return ''
  if (url.startsWith('blob:') || url.startsWith('data:') || url.startsWith('/')) return url
  try {
    const origin = ownerDocument.defaultView?.location.origin ?? window.location.origin
    const parsed = new URL(url, origin)
    if (parsed.origin === origin) return parsed.toString()
  } catch {
    return url
  }
  return `${USER_DOWNLOAD_ENDPOINT}?url=${encodeURIComponent(url)}`
}

function filenameWithBlobMime(fileName: string, mimeType: string): string {
  const normalizedMime = mimeType.trim().toLowerCase().split(';', 1)[0]
  const extension =
    normalizedMime === 'image/jpeg'
      ? 'jpg'
      : normalizedMime === 'image/png'
        ? 'png'
        : normalizedMime === 'image/webp'
          ? 'webp'
          : ''
  if (!extension) return fileName
  const stem = fileName.replace(/\.[A-Za-z0-9]{2,8}$/, '') || 'image'
  return `${stem}.${extension}`
}

export function createCanvasGraphInteractionBrowserAdapter(
  ownerDocument: Document = document,
): CanvasGraphInteractionBrowserAdapter {
  return {
    downloadImage: async ({ url, mediaAssetId, fileName }) => {
      const href = buildDownloadHref(url, mediaAssetId, ownerDocument)
      if (!href) throw new Error('Image URL is empty')
      const response = await fetch(href, { credentials: 'include' })
      if (!response.ok) throw new Error(`Image download failed (${response.status})`)
      const blob = await response.blob()
      const objectUrl = URL.createObjectURL(blob)
      try {
        const link = ownerDocument.createElement('a')
        link.href = objectUrl
        link.download = filenameWithBlobMime(fileName, blob.type)
        link.rel = 'noopener'
        link.style.display = 'none'
        ownerDocument.body.appendChild(link)
        try {
          link.click()
        } finally {
          link.remove()
        }
      } finally {
        const schedule = ownerDocument.defaultView?.setTimeout ?? globalThis.setTimeout
        schedule(() => URL.revokeObjectURL(objectUrl), 0)
      }
    },
  }
}
