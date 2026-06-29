import type { Ref } from 'vue'
import { USER_DOWNLOAD_ENDPOINT } from '../../../shared/constants/media'
import type { CanvasConnectionNodeType, ImageNodeOutputItem } from '../../canvas-board/domain'
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
    },
  ): MessageLoadingHandle
}

type ExportableNodeKind = CanvasConnectionNodeType

type FlowNodeLike = {
  id: string
  selected?: boolean
  data?: FlowNodeData
}

type ExportImageEntry = {
  url: string
  title: string
}

type UseFlowSelectionExportOptions = {
  nodes: Ref<FlowNodeLike[]>
  message: MessageApiLike
}

function safeDecodeURIComponent(value: string): string {
  try {
    return decodeURIComponent(value)
  } catch {
    return value
  }
}

function sanitizeFileNameSegment(rawName: string): string {
  return rawName
    .replace(/[\\/:*?"<>|]+/g, '_')
    .split('')
    .filter((char) => char.charCodeAt(0) >= 32)
    .join('')
    .replace(/\s+/g, ' ')
    .trim()
    .replace(/[. ]+$/g, '')
}

function inferExtensionFromUrl(rawUrl: string): string {
  const rawName = String(rawUrl).split('/').pop()?.split('?')[0] || ''
  const decodedName = safeDecodeURIComponent(rawName)
  const extension = decodedName.match(/\.([a-z0-9]{2,5})$/i)?.[1]
  return extension ? `.${extension.toLowerCase()}` : '.png'
}

function splitTitleAndExtension(
  rawTitle: string,
  rawUrl: string,
): { stem: string; extension: string } {
  const safeTitle = sanitizeFileNameSegment(rawTitle)
  const titleExtension = safeTitle.match(/\.([a-z0-9]{2,5})$/i)?.[0] ?? ''
  if (titleExtension) {
    const stem = sanitizeFileNameSegment(safeTitle.slice(0, -titleExtension.length))
    return {
      stem: stem || 'image',
      extension: titleExtension.toLowerCase(),
    }
  }
  return {
    stem: safeTitle || 'image',
    extension: inferExtensionFromUrl(rawUrl),
  }
}

function buildUniqueExportFileName(
  entry: ExportImageEntry,
  titleCounts: Map<string, number>,
): string {
  const { stem, extension } = splitTitleAndExtension(entry.title, entry.url)
  const dedupeKey = `${stem}${extension}`.toLowerCase()
  const count = titleCounts.get(dedupeKey) ?? 0
  titleCounts.set(dedupeKey, count + 1)
  if (count === 0) {
    return `${stem}${extension}`
  }
  return `${stem}(${count})${extension}`
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

function resolveExportTitle(data: FlowNodeData, snapshotNode: Record<string, unknown>): string {
  const snapshotTitle = typeof snapshotNode.title === 'string' ? snapshotNode.title.trim() : ''
  if (snapshotTitle) return snapshotTitle
  return data.title.trim() || 'image'
}

export function useFlowSelectionExport(options: UseFlowSelectionExportOptions) {
  let exportLoading: MessageLoadingHandle | null = null

  function getSelectedExportEntries(): ExportImageEntry[] {
    const selectedNodes = options.nodes.value.filter((node) => !!node.selected)
    const entries: ExportImageEntry[] = []
    for (const node of selectedNodes) {
      const data = node.data
      if (!data) continue
      const kind = data.kind
      if (kind !== 'image' && kind !== 'fusion') {
        continue
      }
      const snapshotNode = data.snapshotNode
      if (!snapshotNode || typeof snapshotNode !== 'object') continue
      const snapshotRecord = snapshotNode as Record<string, unknown>
      const title = resolveExportTitle(data, snapshotRecord)
      entries.push(
        ...resolveExportUrls(kind, snapshotRecord)
          .map((url) => url.trim())
          .filter(Boolean)
          .map((url) => ({ url, title })),
      )
    }
    return entries
  }

  async function exportSelectedImages() {
    const entries = getSelectedExportEntries()
    if (entries.length === 0) {
      options.message.warning('当前选中节点没有可导出的图片')
      return
    }
    if (exportLoading) {
      exportLoading.destroy()
      exportLoading = null
    }
    const loading = options.message.loading(`Exporting ${entries.length} images...`, {
      duration: 1500,
    })
    exportLoading = loading
    let failed = 0
    const titleCounts = new Map<string, number>()
    try {
      for (const entry of entries) {
        const fileName = buildUniqueExportFileName(entry, titleCounts)
        try {
          const link = document.createElement('a')
          link.href = buildExportDownloadHref(entry.url)
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
      options.message.success(
        `Export queued: ${entries.length} image${entries.length > 1 ? 's' : ''}`,
      )
    } else {
      const success = entries.length - failed
      options.message.warning(`Export queued ${success}/${entries.length}, failed ${failed}`)
    }
  }

  return {
    exportSelectedImages,
    hasSelectedNodes: () => options.nodes.value.some((node) => !!node.selected),
  }
}
