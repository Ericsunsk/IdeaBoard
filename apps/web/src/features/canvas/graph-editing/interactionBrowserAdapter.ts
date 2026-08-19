import { USER_DOWNLOAD_ENDPOINT } from '../../../shared/constants/media'
import { resolveMediaAssetOriginalURL } from '../../../shared/api/mediaAssetDelivery'

export type CanvasGraphInteractionBrowserAdapter = {
  downloadImage: (request: { url: string; mediaAssetId?: number; fileName: string }) => void
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

export function createCanvasGraphInteractionBrowserAdapter(
  ownerDocument: Document = document,
): CanvasGraphInteractionBrowserAdapter {
  return {
    downloadImage: ({ url, mediaAssetId, fileName }) => {
      const link = ownerDocument.createElement('a')
      link.href = buildDownloadHref(url, mediaAssetId, ownerDocument)
      link.download = fileName
      link.rel = 'noopener'
      link.style.display = 'none'
      ownerDocument.body.appendChild(link)
      link.click()
      ownerDocument.body.removeChild(link)
    },
  }
}
