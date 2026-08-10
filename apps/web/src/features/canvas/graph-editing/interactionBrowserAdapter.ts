import { USER_DOWNLOAD_ENDPOINT } from '../../../shared/constants/media'

export type CanvasGraphInteractionBrowserAdapter = {
  downloadImage: (request: { url: string; fileName: string }) => void
}

function buildDownloadHref(rawUrl: string, ownerDocument: Document): string {
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
    downloadImage: ({ url, fileName }) => {
      const link = ownerDocument.createElement('a')
      link.href = buildDownloadHref(url, ownerDocument)
      link.download = fileName
      link.rel = 'noopener'
      link.style.display = 'none'
      ownerDocument.body.appendChild(link)
      link.click()
      ownerDocument.body.removeChild(link)
    },
  }
}
