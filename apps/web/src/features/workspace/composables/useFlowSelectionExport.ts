import type { Ref } from 'vue'
import { USER_DOWNLOAD_ENDPOINT } from '../../../shared/constants/media'
import type { CanvasConnectionNodeType, ImageNodeOutputItem } from '../types/canvasNodes'
import type { FlowNodeData } from '../types/flowTypes'

interface MessageLoadingHandle {
  destroy: () => void
}

interface MessageApiLike {
  success(content: string): unknown
  warning(content: string): unknown
  loading(
    content: string,
    options?: {
      duration?: number
      keepAliveOnHover?: boolean
    },
  ): MessageLoadingHandle
}

type ExportableNodeKind = CanvasConnectionNodeType

type FlowNodeLike = {
  id: string
  selected?: boolean
  data?: FlowNodeData
}

type UseFlowSelectionExportOptions = {
  nodes: Ref<FlowNodeLike[]>
  message: MessageApiLike
}

function toSafeFileName(rawName: string): string {
  const decoded = rawName ? decodeURIComponent(rawName) : ''
  const cleaned = decoded.replace(/[\\/:*?"<>|]+/g, '_').trim()
  if (!cleaned) return `export-${Date.now()}.png`
  if (/\.[a-z0-9]+$/i.test(cleaned)) return cleaned
  return `${cleaned}.png`
}

function buildExportFileName(url: string, index: number, total: number, timestamp: number): string {
  const fromUrl = String(url).split('/').pop()?.split('?')[0] || ''
  const safeName = toSafeFileName(fromUrl)
  if (total > 1) {
    return `${String(index + 1).padStart(2, '0')}-${timestamp}-${safeName}`
  }
  return safeName
}

function buildExportDownloadHref(rawUrl: string): string {
  const url = rawUrl.trim()
  if (!url) return ''
  if (url.startsWith('blob:') || url.startsWith('data:') || url.startsWith('/')) {
    return url
  }
  try {
    const parsed = new URL(url, window.location.origin)
    if (parsed.origin === window.location.origin) {
      return parsed.toString()
    }
  } catch {
    return url
  }
  return `${USER_DOWNLOAD_ENDPOINT}?url=${encodeURIComponent(url)}`
}

function readOutputImages(snapshotNode: Record<string, unknown>): ImageNodeOutputItem[] {
  const rawOutputs = Array.isArray(snapshotNode.outputImages) ? snapshotNode.outputImages : []
  return rawOutputs
    .map((item) => {
      if (!item || typeof item !== 'object') return null
      const rawUrl =
        typeof (item as { url?: unknown }).url === 'string'
          ? (item as { url: string }).url.trim()
          : ''
      if (!rawUrl) return null
      return { url: rawUrl, ratio: 1 }
    })
    .filter((item): item is ImageNodeOutputItem => !!item)
}

function resolveExportUrls(
  kind: ExportableNodeKind,
  snapshotNode: Record<string, unknown>,
): string[] {
  const outputUrls = readOutputImages(snapshotNode)
    .map((item) => item.url)
    .filter(Boolean)
  if (kind === 'image') {
    const latestOutputUrl = outputUrls.length > 0 ? outputUrls[outputUrls.length - 1] : ''
    if (latestOutputUrl) return [latestOutputUrl]
    const sourceImageUrl =
      typeof snapshotNode.sourceImageUrl === 'string' ? snapshotNode.sourceImageUrl.trim() : ''
    if (sourceImageUrl) return [sourceImageUrl]
  }
  return outputUrls
}

export function useFlowSelectionExport(options: UseFlowSelectionExportOptions) {
  let exportLoading: MessageLoadingHandle | null = null

  function getSelectedExportUrls(): string[] {
    const selectedNodes = options.nodes.value.filter((node) => !!node.selected)
    const urls: string[] = []
    for (const node of selectedNodes) {
      const data = node.data
      if (!data) continue
      const kind = data.kind
      if (
        kind !== 'image' &&
        kind !== 'edit' &&
        kind !== 'enhance' &&
        kind !== 'color' &&
        kind !== 'fusion' &&
        kind !== 'wear' &&
        kind !== 'model' &&
        kind !== 'viewpoint'
      ) {
        continue
      }
      const snapshotNode = data.snapshotNode
      if (!snapshotNode || typeof snapshotNode !== 'object') continue
      urls.push(...resolveExportUrls(kind, snapshotNode as Record<string, unknown>))
    }
    return Array.from(new Set(urls.map((url) => url.trim()).filter(Boolean)))
  }

  async function exportSelectedImages() {
    const urls = getSelectedExportUrls()
    if (urls.length === 0) {
      options.message.warning('当前选中节点没有可导出的图片')
      return
    }
    if (exportLoading) {
      exportLoading.destroy()
      exportLoading = null
    }
    const loading = options.message.loading(`Exporting ${urls.length} images...`, {
      duration: 1500,
      keepAliveOnHover: false,
    })
    exportLoading = loading
    const timestamp = Date.now()
    let failed = 0
    try {
      for (let i = 0; i < urls.length; i += 1) {
        const url = urls[i]
        const fileName = buildExportFileName(url, i, urls.length, timestamp)
        try {
          const link = document.createElement('a')
          link.href = buildExportDownloadHref(url)
          link.download = fileName
          link.rel = 'noopener'
          link.style.display = 'none'
          document.body.appendChild(link)
          link.click()
          document.body.removeChild(link)
        } catch {
          failed += 1
        }
      }
    } finally {
      loading.destroy()
      if (exportLoading === loading) exportLoading = null
    }

    if (failed === 0) {
      options.message.success(`Export queued: ${urls.length} image${urls.length > 1 ? 's' : ''}`)
    } else {
      const success = urls.length - failed
      options.message.warning(`Export queued ${success}/${urls.length}, failed ${failed}`)
    }
  }

  return {
    exportSelectedImages,
    hasSelectedNodes: () => options.nodes.value.some((node) => !!node.selected),
  }
}
