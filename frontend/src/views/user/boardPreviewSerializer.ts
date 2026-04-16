import type { CanvasBoardState } from '../../utils/canvasBoardState'

export type BoardPreviewItem =
  | {
      key: string
      kind: 'image'
      url: string
      left: number
      top: number
      width: number
      height: number
      zIndex: number
    }
  | {
      key: string
      kind: 'node'
      label: string
      text: string
      thumbUrl: string
      left: number
      top: number
      width: number
      height: number
      zIndex: number
    }

export interface BoardPreviewFrame {
  key: string
  kind: 'image' | 'node'
  x: number
  y: number
  width: number
  height: number
  url?: string
  label?: string
  text?: string
  thumbUrl?: string
}

interface NodeImagePreview {
  url: string
  ratio: number
}

interface NodeLike {
  id?: unknown
  type?: unknown
  x?: unknown
  y?: unknown
  width?: unknown
  text?: unknown
  outputImages?: unknown
  sourceImageUrl?: unknown
  sourceImageRatio?: unknown
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value)
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === 'object'
}

function getImageNodePreview(node: NodeLike): NodeImagePreview | null {
  const rawOutputs = Array.isArray(node.outputImages) ? node.outputImages : []
  for (let i = rawOutputs.length - 1; i >= 0; i -= 1) {
    const output = rawOutputs[i]
    if (!isRecord(output)) continue
    if (typeof output.url !== 'string' || !output.url) continue
    const ratio = isFiniteNumber(output.ratio) && output.ratio > 0 ? output.ratio : 1
    return { url: output.url, ratio }
  }
  if (typeof node.sourceImageUrl === 'string' && node.sourceImageUrl) {
    const ratio = isFiniteNumber(node.sourceImageRatio) && node.sourceImageRatio > 0 ? node.sourceImageRatio : 1
    return { url: node.sourceImageUrl, ratio }
  }
  return null
}

function toTitleFromType(rawType: string): string {
  const normalized = rawType
    .replace(/[_-]+/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
  if (!normalized) return 'Node'
  return normalized
    .split(' ')
    .map(part => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ')
}

export function collectBoardPreviewFrames(state: CanvasBoardState): BoardPreviewFrame[] {
  const imageFrames: BoardPreviewFrame[] = state.messages
    .filter(item => item.type === 'image' && !!item.url)
    .slice(-12)
    .map((item) => {
      const width = item.width && item.width > 0 ? item.width : 300
      const ratio = item.height_ratio && item.height_ratio > 0 ? item.height_ratio : 1
      return {
        key: `image-${item.id}`,
        kind: 'image' as const,
        x: item.x,
        y: item.y,
        width,
        height: width * ratio,
        url: item.url,
      }
    })

  const nodeFrames: BoardPreviewFrame[] = []
  const ignoredKeys = new Set(['messages', 'viewport', 'updatedAt'])
  const stateRecord = state as unknown as Record<string, unknown>

  Object.entries(stateRecord).forEach(([key, value]) => {
    if (ignoredKeys.has(key) || !Array.isArray(value)) return
    value.slice(-10).forEach((nodeLike, index) => {
      if (!isRecord(nodeLike)) return
      const node = nodeLike as NodeLike
      if (!isFiniteNumber(node.x) || !isFiniteNumber(node.y)) return

      const type = typeof node.type === 'string' ? node.type : key
      const label = toTitleFromType(type)
      const width = isFiniteNumber(node.width) && node.width > 0 ? node.width : 280
      const text = typeof node.text === 'string' ? node.text.trim() : ''
      const imagePreview = getImageNodePreview(node)
      const thumbUrl = imagePreview?.url || ''

      let height = 124
      if (thumbUrl) {
        const ratio = imagePreview?.ratio || 1
        const stageWidth = Math.max(120, width - 24)
        const stageHeight = Math.max(110, Math.min(250, stageWidth * ratio))
        height = stageHeight + 92
      } else if (text) {
        height = 138
      }

      const idPart = isFiniteNumber(node.id) ? String(node.id) : `${key}-${index}`
      nodeFrames.push({
        key: `node-${key}-${idPart}`,
        kind: 'node',
        x: node.x,
        y: node.y,
        width,
        height,
        label,
        text,
        thumbUrl,
      })
    })
  })

  return [...imageFrames, ...nodeFrames]
}

export function normalizeBoardPreviewFrames(frames: readonly BoardPreviewFrame[]): BoardPreviewItem[] {
  if (frames.length === 0) return []

  const minX = Math.min(...frames.map(item => item.x))
  const minY = Math.min(...frames.map(item => item.y))
  const maxX = Math.max(...frames.map(item => item.x + item.width))
  const maxY = Math.max(...frames.map(item => item.y + item.height))
  const boxW = Math.max(1, maxX - minX)
  const boxH = Math.max(1, maxY - minY)

  const padding = 6
  const viewW = 100 - padding * 2
  const viewH = 100 - padding * 2
  const scale = Math.min(viewW / boxW, viewH / boxH)
  const scaledW = boxW * scale
  const scaledH = boxH * scale
  const offsetX = (100 - scaledW) / 2 - minX * scale
  const offsetY = (100 - scaledH) / 2 - minY * scale

  return frames.map((item, index) => {
    const base = {
      key: item.key,
      left: item.x * scale + offsetX,
      top: item.y * scale + offsetY,
      width: item.width * scale,
      height: item.height * scale,
    }
    if (item.kind === 'image') {
      return {
        ...base,
        kind: 'image' as const,
        url: item.url || '',
        zIndex: 10 + index,
      }
    }
    return {
      ...base,
      kind: 'node' as const,
      label: item.label || 'Node',
      text: item.text || '',
      thumbUrl: item.thumbUrl || '',
      zIndex: 310 + index,
    }
  })
}

export function buildBoardPreviewItemsFromState(state: CanvasBoardState | null): BoardPreviewItem[] {
  if (!state) return []
  const frames = collectBoardPreviewFrames(state)
  return normalizeBoardPreviewFrames(frames)
}
