import type {
  CanvasBoardSnapshot,
  CanvasConnectionNodeType,
} from '../../workspace/types/canvasNodes'
import {
  resolveCanvasConnectionTone,
  type CanvasConnectionTone,
} from '../../workspace/config/canvasConnections'

export interface BoardPreviewFrame {
  key: string
  kind: 'image' | 'node'
  x: number
  y: number
  width: number
  height: number
  label?: string
  text?: string
  imageUrl?: string
  nodeType?: CanvasConnectionNodeType
  nodeId?: number
}

interface BoardPreviewConnection {
  key: string
  tone: CanvasConnectionTone
  startX: number
  startY: number
  endX: number
  endY: number
}

interface NodeImagePreview {
  url: string
  ratio: number
}

interface NodeLike {
  id?: unknown
  x?: unknown
  y?: unknown
  width?: unknown
  text?: unknown
  mode?: unknown
  outputImages?: unknown
  sourceImageUrl?: unknown
  sourceImageRatio?: unknown
  executionMaskDataUrl?: unknown
  maskDataUrl?: unknown
}

interface BoardPreviewBounds {
  minX: number
  minY: number
  maxX: number
  maxY: number
  width: number
  height: number
}

const BOARD_NODE_COLLECTIONS = [
  { key: 'imageNodes', nodeType: 'image' },
  { key: 'promptNodes', nodeType: 'prompt' },
  { key: 'modelNodes', nodeType: 'model' },
  { key: 'viewpointNodes', nodeType: 'viewpoint' },
  { key: 'maskNodes', nodeType: 'mask' },
  { key: 'colorNodes', nodeType: 'color' },
  { key: 'editNodes', nodeType: 'edit' },
  { key: 'fusionNodes', nodeType: 'fusion' },
  { key: 'enhanceNodes', nodeType: 'enhance' },
  { key: 'wearNodes', nodeType: 'wear' },
] as const satisfies ReadonlyArray<{ key: keyof CanvasBoardSnapshot; nodeType: CanvasConnectionNodeType }>

const PREVIEW_VIEWPORT_WIDTH = 1180
const PREVIEW_VIEWPORT_HEIGHT = 760
const PREVIEW_PADDING = 50
const PREVIEW_BACKGROUND = '#eef3fb'
const PREVIEW_DOT_COLOR = 'rgba(201, 209, 225, 0.68)'
const PREVIEW_STAGE_GLOW_PRIMARY = 'rgba(180, 199, 255, 0.28)'
const PREVIEW_STAGE_GLOW_SECONDARY = 'rgba(224, 233, 255, 0.62)'
const USER_DOWNLOAD_ENDPOINT = '/api/user/download'
const NODE_ACCENT_COLORS: Record<CanvasConnectionNodeType, string> = {
  image: '#6c8fff',
  prompt: '#d4a34d',
  model: '#7b8cf3',
  viewpoint: '#67a2ff',
  mask: '#4fcab2',
  color: '#f09164',
  edit: '#f17a88',
  fusion: '#b278ff',
  enhance: '#5aa5ff',
  wear: '#7cb86d',
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value)
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === 'object'
}

function clamp(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, value))
}

function withAlpha(hex: string, alpha: number): string {
  const normalized = hex.replace('#', '')
  if (normalized.length !== 6) return `rgba(15, 23, 42, ${alpha})`
  const value = Number.parseInt(normalized, 16)
  const r = (value >> 16) & 255
  const g = (value >> 8) & 255
  const b = value & 255
  return `rgba(${r}, ${g}, ${b}, ${alpha})`
}

function toAbsolutePreviewUrl(url: string): string {
  const normalized = url.trim()
  if (!normalized) return ''
  if (
    normalized.startsWith('http://') ||
    normalized.startsWith('https://') ||
    normalized.startsWith('data:') ||
    normalized.startsWith('blob:')
  ) {
    return normalized
  }
  if (typeof window !== 'undefined' && normalized.startsWith('/')) {
    return `${window.location.origin}${normalized}`
  }
  return normalized
}

function shouldProxyPreviewUrl(resolvedUrl: string): boolean {
  if (typeof window === 'undefined') return false
  if (!resolvedUrl.startsWith('http://') && !resolvedUrl.startsWith('https://')) return false
  try {
    return new URL(resolvedUrl).origin !== window.location.origin
  } catch {
    return false
  }
}

function getLatestOutputPreview(node: NodeLike): NodeImagePreview | null {
  const rawOutputs = Array.isArray(node.outputImages) ? node.outputImages : []
  for (let i = rawOutputs.length - 1; i >= 0; i -= 1) {
    const output = rawOutputs[i]
    if (!isRecord(output)) continue
    if (typeof output.url !== 'string' || !output.url) continue
    return {
      url: output.url,
      ratio: isFiniteNumber(output.ratio) && output.ratio > 0 ? output.ratio : 1,
    }
  }
  return null
}

function getSourcePreview(node: NodeLike): NodeImagePreview | null {
  if (typeof node.sourceImageUrl !== 'string' || !node.sourceImageUrl) return null
  return {
    url: node.sourceImageUrl,
    ratio: isFiniteNumber(node.sourceImageRatio) && node.sourceImageRatio > 0 ? node.sourceImageRatio : 1,
  }
}

function getMaskPreview(node: NodeLike): NodeImagePreview | null {
  if (typeof node.executionMaskDataUrl === 'string' && node.executionMaskDataUrl) {
    return {
      url: node.executionMaskDataUrl,
      ratio: isFiniteNumber(node.sourceImageRatio) && node.sourceImageRatio > 0 ? node.sourceImageRatio : 1,
    }
  }
  if (typeof node.maskDataUrl === 'string' && node.maskDataUrl) {
    return {
      url: node.maskDataUrl,
      ratio: isFiniteNumber(node.sourceImageRatio) && node.sourceImageRatio > 0 ? node.sourceImageRatio : 1,
    }
  }
  return getSourcePreview(node)
}

function resolveNodePreview(nodeType: CanvasConnectionNodeType, node: NodeLike): NodeImagePreview | null {
  if (nodeType === 'mask') return getMaskPreview(node)
  return getLatestOutputPreview(node) ?? getSourcePreview(node)
}

function resolveNodeLabel(nodeType: CanvasConnectionNodeType): string {
  if (nodeType === 'image') return 'Image'
  if (nodeType === 'prompt') return 'Prompt'
  if (nodeType === 'model') return 'Model'
  if (nodeType === 'viewpoint') return 'Viewpoint'
  if (nodeType === 'mask') return 'Mask'
  if (nodeType === 'color') return 'Color'
  if (nodeType === 'edit') return 'Edit'
  if (nodeType === 'fusion') return 'Fusion'
  if (nodeType === 'enhance') return 'Enhance'
  if (nodeType === 'wear') return 'Wear'
  return 'Node'
}

function resolveNodeText(nodeType: CanvasConnectionNodeType, node: NodeLike): string {
  if (nodeType === 'prompt') {
    return typeof node.text === 'string' ? node.text.trim() : ''
  }
  if (nodeType === 'enhance' && typeof node.mode === 'string') {
    return node.mode
  }
  if (nodeType === 'edit' && typeof node.mode === 'string') {
    return node.mode
  }
  return ''
}

function getImageNodeHeight(width: number, ratio: number): number {
  const stageWidth = Math.max(120, width - 24)
  return 46 + Math.max(96, Math.round(stageWidth * ratio))
}

function getMaskNodeHeight(width: number, ratio: number, hasSource: boolean): number {
  const stageWidth = Math.max(120, width - 24)
  const stageHeight = hasSource
    ? Math.max(112, Math.min(360, Math.round(stageWidth * ratio)))
    : 172
  return 46 + stageHeight
}

function resolveNodeHeight(nodeType: CanvasConnectionNodeType, width: number, node: NodeLike): number {
  if (nodeType === 'image') {
    return getImageNodeHeight(width, resolveNodePreview(nodeType, node)?.ratio ?? 1)
  }
  if (nodeType === 'mask') {
    const preview = resolveNodePreview(nodeType, node)
    return getMaskNodeHeight(width, preview?.ratio ?? 1, !!preview)
  }
  if (nodeType === 'prompt') {
    const text = typeof node.text === 'string' ? node.text.trim() : ''
    return Math.max(128, 94 + Math.min(48, Math.ceil(text.length / 34) * 10))
  }
  if (nodeType === 'model') return 244
  if (nodeType === 'viewpoint') return 210
  if (nodeType === 'color') return 228
  if (nodeType === 'edit') return node.mode === 'face-swap' ? 188 : 220
  if (nodeType === 'fusion') return 206
  if (nodeType === 'enhance') return 172
  if (nodeType === 'wear') return 224
  return 156
}

function resolveBounds(frames: readonly BoardPreviewFrame[]): BoardPreviewBounds | null {
  if (!frames.length) return null
  const minX = Math.min(...frames.map(frame => frame.x))
  const minY = Math.min(...frames.map(frame => frame.y))
  const maxX = Math.max(...frames.map(frame => frame.x + frame.width))
  const maxY = Math.max(...frames.map(frame => frame.y + frame.height))
  if (!Number.isFinite(minX) || !Number.isFinite(minY) || !Number.isFinite(maxX) || !Number.isFinite(maxY)) {
    return null
  }
  return {
    minX,
    minY,
    maxX,
    maxY,
    width: Math.max(1, maxX - minX),
    height: Math.max(1, maxY - minY),
  }
}

function chunkText(text: string, maxCharsPerLine: number, maxLines: number): string[] {
  const normalized = text.trim().replace(/\s+/g, ' ')
  if (!normalized) return []
  const result: string[] = []
  let cursor = 0
  while (cursor < normalized.length && result.length < maxLines) {
    const nextCursor = Math.min(normalized.length, cursor + maxCharsPerLine)
    const suffix = nextCursor < normalized.length && result.length === maxLines - 1 ? '...' : ''
    result.push(normalized.slice(cursor, nextCursor) + suffix)
    cursor = nextCursor
  }
  return result
}

function scaleBounds(bounds: BoardPreviewBounds) {
  const availableWidth = PREVIEW_VIEWPORT_WIDTH - PREVIEW_PADDING * 2
  const availableHeight = PREVIEW_VIEWPORT_HEIGHT - PREVIEW_PADDING * 2
  const scale = clamp(Math.min(availableWidth / bounds.width, availableHeight / bounds.height), 0.08, 2.4)
  const scaledWidth = bounds.width * scale
  const scaledHeight = bounds.height * scale
  return {
    scale,
    offsetX: (PREVIEW_VIEWPORT_WIDTH - scaledWidth) / 2 - bounds.minX * scale,
    offsetY: (PREVIEW_VIEWPORT_HEIGHT - scaledHeight) / 2 - bounds.minY * scale,
  }
}

function collectNodeFrames(state: CanvasBoardSnapshot): BoardPreviewFrame[] {
  const frames: BoardPreviewFrame[] = []
  BOARD_NODE_COLLECTIONS.forEach(({ key, nodeType }) => {
    const nodes = state[key]
    nodes.forEach((nodeLike, index) => {
      if (!isRecord(nodeLike)) return
      const node = nodeLike as NodeLike
      if (!isFiniteNumber(node.id) || !isFiniteNumber(node.x) || !isFiniteNumber(node.y)) return
      const width = isFiniteNumber(node.width) && node.width > 0 ? node.width : 280
      const preview = resolveNodePreview(nodeType, node)
      frames.push({
        key: `node-${nodeType}-${node.id}-${index}`,
        kind: 'node',
        x: node.x,
        y: node.y,
        width,
        height: resolveNodeHeight(nodeType, width, node),
        label: resolveNodeLabel(nodeType),
        text: resolveNodeText(nodeType, node),
        imageUrl: preview?.url,
        nodeType,
        nodeId: node.id,
      })
    })
  })
  return frames
}

export function collectBoardPreviewFrames(state: CanvasBoardSnapshot): BoardPreviewFrame[] {
  const imageFrames: BoardPreviewFrame[] = state.messages
    .filter(item => item.type === 'image' && !!item.url)
    .map((item) => {
      const width = item.width && item.width > 0 ? item.width : 300
      const ratio = item.height_ratio && item.height_ratio > 0 ? item.height_ratio : 1
      return {
        key: `image-${item.id}`,
        kind: 'image',
        x: item.x,
        y: item.y,
        width,
        height: width * clamp(ratio, 0.05, 12),
        imageUrl: item.url,
      }
    })

  return [...imageFrames, ...collectNodeFrames(state)]
}

function collectBoardPreviewConnections(
  state: CanvasBoardSnapshot,
  frames: readonly BoardPreviewFrame[],
): BoardPreviewConnection[] {
  const frameMap = new Map<string, BoardPreviewFrame>()
  frames.forEach((frame) => {
    if (!frame.nodeType || typeof frame.nodeId !== 'number') return
    frameMap.set(`${frame.nodeType}:${frame.nodeId}`, frame)
  })

  return state.connections.flatMap((connection, index) => {
    const fromFrame = frameMap.get(`${connection.fromNodeType}:${connection.fromNodeId}`)
    const toFrame = frameMap.get(`${connection.toNodeType}:${connection.toNodeId}`)
    if (!fromFrame || !toFrame) return []
    return [{
      key: `connection-${connection.id}-${index}`,
      tone: resolveCanvasConnectionTone(connection.fromPort),
      startX: fromFrame.x + fromFrame.width,
      startY: fromFrame.y + fromFrame.height / 2,
      endX: toFrame.x,
      endY: toFrame.y + toFrame.height / 2,
    }]
  })
}

function resolveConnectionStroke(tone: CanvasConnectionTone): string {
  if (tone === 'mask') return '#63d1bc'
  if (tone === 'prompt') return '#e8c276'
  return '#7aa3ff'
}

function createPreviewCanvas(): HTMLCanvasElement | null {
  if (typeof document === 'undefined') return null
  const canvas = document.createElement('canvas')
  canvas.width = PREVIEW_VIEWPORT_WIDTH
  canvas.height = PREVIEW_VIEWPORT_HEIGHT
  return canvas
}

function roundedRectPath(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  width: number,
  height: number,
  radius: number,
) {
  const actualRadius = Math.max(0, Math.min(radius, width / 2, height / 2))
  ctx.beginPath()
  ctx.moveTo(x + actualRadius, y)
  ctx.arcTo(x + width, y, x + width, y + height, actualRadius)
  ctx.arcTo(x + width, y + height, x, y + height, actualRadius)
  ctx.arcTo(x, y + height, x, y, actualRadius)
  ctx.arcTo(x, y, x + width, y, actualRadius)
  ctx.closePath()
}

function drawBackdropGlow(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  radius: number,
  color: string,
) {
  const glow = ctx.createRadialGradient(x, y, 0, x, y, radius)
  glow.addColorStop(0, color)
  glow.addColorStop(1, withAlpha('#FFFFFF', 0))
  ctx.fillStyle = glow
  ctx.beginPath()
  ctx.arc(x, y, radius, 0, Math.PI * 2)
  ctx.fill()
}

function drawDotBackground(ctx: CanvasRenderingContext2D) {
  ctx.fillStyle = PREVIEW_BACKGROUND
  ctx.fillRect(0, 0, PREVIEW_VIEWPORT_WIDTH, PREVIEW_VIEWPORT_HEIGHT)

  drawBackdropGlow(ctx, PREVIEW_VIEWPORT_WIDTH * 0.18, PREVIEW_VIEWPORT_HEIGHT * 0.2, 260, PREVIEW_STAGE_GLOW_PRIMARY)
  drawBackdropGlow(ctx, PREVIEW_VIEWPORT_WIDTH * 0.84, PREVIEW_VIEWPORT_HEIGHT * 0.82, 300, PREVIEW_STAGE_GLOW_SECONDARY)

  const gradient = ctx.createLinearGradient(0, 0, 0, PREVIEW_VIEWPORT_HEIGHT)
  gradient.addColorStop(0, '#fcfdff')
  gradient.addColorStop(1, '#f7f9fe')
  ctx.fillStyle = gradient
  ctx.fillRect(0, 0, PREVIEW_VIEWPORT_WIDTH, PREVIEW_VIEWPORT_HEIGHT)

  ctx.fillStyle = PREVIEW_DOT_COLOR
  for (let x = 14; x < PREVIEW_VIEWPORT_WIDTH; x += 24) {
    for (let y = 14; y < PREVIEW_VIEWPORT_HEIGHT; y += 24) {
      ctx.beginPath()
      ctx.arc(x, y, 1.25, 0, Math.PI * 2)
      ctx.fill()
    }
  }
}

async function loadPreviewImage(url: string): Promise<{ image: HTMLImageElement; revokeUrl?: string } | null> {
  const resolvedUrl = toAbsolutePreviewUrl(url)
  if (!resolvedUrl || typeof Image === 'undefined') return null
  let imageSrc = resolvedUrl
  let revokeUrl: string | undefined

  if (shouldProxyPreviewUrl(resolvedUrl)) {
    try {
      const response = await fetch(`${USER_DOWNLOAD_ENDPOINT}?url=${encodeURIComponent(resolvedUrl)}`, {
        credentials: 'include',
      })
      if (!response.ok) return null
      const blob = await response.blob()
      revokeUrl = URL.createObjectURL(blob)
      imageSrc = revokeUrl
    } catch {
      return null
    }
  }

  return new Promise((resolve) => {
    const image = new Image()
    image.decoding = 'async'
    image.onload = () => resolve({ image, revokeUrl })
    image.onerror = () => {
      if (revokeUrl) URL.revokeObjectURL(revokeUrl)
      resolve(null)
    }
    image.src = imageSrc
  })
}

async function buildPreviewImageMap(frames: readonly BoardPreviewFrame[]): Promise<{
  images: Map<string, HTMLImageElement>
  revoke: () => void
}> {
  const uniqueUrls = Array.from(
    new Set(
      frames
        .map(frame => frame.imageUrl?.trim() || '')
        .filter(Boolean),
    ),
  )
  const revokeUrls: string[] = []
  const imageEntries = await Promise.all(
    uniqueUrls.map(async (url) => {
      const loaded = await loadPreviewImage(url)
      if (!loaded) return null
      if (loaded.revokeUrl) revokeUrls.push(loaded.revokeUrl)
      return [url, loaded.image] as const
    }),
  )
  const result = new Map<string, HTMLImageElement>()
  imageEntries.forEach((entry) => {
    if (!entry) return
    result.set(entry[0], entry[1])
  })
  return {
    images: result,
    revoke: () => {
      revokeUrls.forEach(url => URL.revokeObjectURL(url))
    },
  }
}

function drawImageCover(
  ctx: CanvasRenderingContext2D,
  image: CanvasImageSource,
  x: number,
  y: number,
  width: number,
  height: number,
) {
  const imageWidth = (image as HTMLImageElement).naturalWidth || (image as HTMLImageElement).width || width
  const imageHeight = (image as HTMLImageElement).naturalHeight || (image as HTMLImageElement).height || height
  const scale = Math.max(width / imageWidth, height / imageHeight)
  const drawWidth = imageWidth * scale
  const drawHeight = imageHeight * scale
  const drawX = x + (width - drawWidth) / 2
  const drawY = y + (height - drawHeight) / 2
  ctx.drawImage(image, drawX, drawY, drawWidth, drawHeight)
}

function drawImageContain(
  ctx: CanvasRenderingContext2D,
  image: CanvasImageSource,
  x: number,
  y: number,
  width: number,
  height: number,
) {
  const imageWidth = (image as HTMLImageElement).naturalWidth || (image as HTMLImageElement).width || width
  const imageHeight = (image as HTMLImageElement).naturalHeight || (image as HTMLImageElement).height || height
  const scale = Math.min(width / imageWidth, height / imageHeight)
  const drawWidth = imageWidth * scale
  const drawHeight = imageHeight * scale
  const drawX = x + (width - drawWidth) / 2
  const drawY = y + (height - drawHeight) / 2
  ctx.drawImage(image, drawX, drawY, drawWidth, drawHeight)
}

function drawPreviewPlaceholder(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  width: number,
  height: number,
  label: string,
  accent = '#8a96ad',
) {
  const gradient = ctx.createLinearGradient(x, y, x + width, y + height)
  gradient.addColorStop(0, withAlpha(accent, 0.08))
  gradient.addColorStop(1, withAlpha(accent, 0.18))
  ctx.fillStyle = gradient
  ctx.fillRect(x, y, width, height)
  ctx.strokeStyle = withAlpha(accent, 0.2)
  ctx.lineWidth = 1
  ctx.strokeRect(x + 0.5, y + 0.5, Math.max(0, width - 1), Math.max(0, height - 1))
  ctx.fillStyle = withAlpha(accent, 0.78)
  const fontSize = Math.max(11, Math.min(24, width * 0.11))
  ctx.font = `700 ${fontSize}px system-ui`
  ctx.textAlign = 'center'
  ctx.textBaseline = 'middle'
  ctx.fillText(label, x + width / 2, y + height / 2)
}

function drawLabelChip(
  ctx: CanvasRenderingContext2D,
  label: string,
  x: number,
  y: number,
  accent: string,
  scale: number,
) {
  const fontSize = Math.max(10, Math.min(14, Math.round(10 * clamp(scale, 0.95, 1.25))))
  ctx.font = `700 ${fontSize}px system-ui`
  const textWidth = ctx.measureText(label).width
  const chipHeight = fontSize + 10
  const chipWidth = textWidth + 16
  ctx.fillStyle = withAlpha(accent, 0.12)
  roundedRectPath(ctx, x, y, chipWidth, chipHeight, chipHeight / 2)
  ctx.fill()
  ctx.strokeStyle = withAlpha(accent, 0.16)
  ctx.lineWidth = 1
  roundedRectPath(ctx, x, y, chipWidth, chipHeight, chipHeight / 2)
  ctx.stroke()
  ctx.fillStyle = withAlpha(accent, 0.95)
  ctx.textAlign = 'left'
  ctx.textBaseline = 'middle'
  ctx.fillText(label, x + 8, y + chipHeight / 2 + 0.5)
}

function drawTextLines(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  width: number,
  scale: number,
  text: string,
) {
  const lines = chunkText(text, Math.max(12, Math.floor(width / 18)), 4)
  ctx.fillStyle = '#7a849c'
  ctx.font = `${Math.max(11, Math.round(11 * scale))}px system-ui`
  ctx.textAlign = 'left'
  ctx.textBaseline = 'top'
  lines.forEach((line, index) => {
    ctx.fillText(line, x, y + index * 16 * scale)
  })
}

function drawConnection(
  ctx: CanvasRenderingContext2D,
  connection: BoardPreviewConnection,
  scale: number,
  offsetX: number,
  offsetY: number,
) {
  const startX = connection.startX * scale + offsetX
  const startY = connection.startY * scale + offsetY
  const endX = connection.endX * scale + offsetX
  const endY = connection.endY * scale + offsetY
  const curve = Math.max(36, Math.abs(endX - startX) * 0.42)
  const stroke = resolveConnectionStroke(connection.tone)
  ctx.beginPath()
  ctx.moveTo(startX, startY)
  ctx.bezierCurveTo(startX + curve, startY, endX - curve, endY, endX, endY)
  ctx.strokeStyle = withAlpha(stroke, 0.88)
  ctx.lineWidth = connection.tone === 'prompt' ? 2.2 : 2.6
  ctx.lineCap = 'round'
  ctx.lineJoin = 'round'
  ctx.globalAlpha = 0.92
  ctx.stroke()
  ctx.globalAlpha = 1
  ctx.fillStyle = withAlpha(stroke, 0.72)
  ctx.beginPath()
  ctx.arc(startX, startY, 2.2, 0, Math.PI * 2)
  ctx.fill()
  ctx.beginPath()
  ctx.arc(endX, endY, 2.2, 0, Math.PI * 2)
  ctx.fill()
}

function drawFrame(
  ctx: CanvasRenderingContext2D,
  frame: BoardPreviewFrame,
  scale: number,
  offsetX: number,
  offsetY: number,
  imageMap: Map<string, HTMLImageElement>,
) {
  const x = frame.x * scale + offsetX
  const y = frame.y * scale + offsetY
  const width = frame.width * scale
  const height = frame.height * scale
  const radius = Math.min(22, Math.max(12, width * 0.05))
  const image = frame.imageUrl ? imageMap.get(frame.imageUrl) ?? null : null
  const accent = frame.nodeType ? NODE_ACCENT_COLORS[frame.nodeType] : '#7a90bf'

  ctx.save()
  ctx.fillStyle = '#ffffff'
  roundedRectPath(ctx, x, y, width, height, radius)
  ctx.fill()
  ctx.restore()

  ctx.save()
  roundedRectPath(ctx, x, y, width, height, radius)
  ctx.clip()

  if (frame.kind === 'image') {
    const inset = Math.max(5, width * 0.024)
    const imageRadius = Math.max(8, radius - 4)
    if (image) {
      roundedRectPath(ctx, x + inset, y + inset, width - inset * 2, height - inset * 2, imageRadius)
      ctx.clip()
      drawImageCover(ctx, image, x + inset, y + inset, width - inset * 2, height - inset * 2)
    } else {
      drawPreviewPlaceholder(ctx, x + inset, y + inset, width - inset * 2, height - inset * 2, 'IMAGE', accent)
    }
  } else {
    ctx.fillStyle = '#ffffff'
    ctx.fillRect(x, y, width, height)
    const insetX = Math.max(10, width * 0.05)
    const insetY = Math.max(10, height * 0.06)
    const chipY = y + insetY
    const chipX = x + insetX
    drawLabelChip(ctx, frame.label || 'Node', chipX, chipY, accent, scale)

    const bodyX = x + insetX
    const bodyY = chipY + Math.max(24, 24 * clamp(scale, 0.9, 1.15))
    const bodyWidth = Math.max(12, width - insetX * 2)
    const bodyHeight = Math.max(12, height - (bodyY - y) - insetY)
    const bodyRadius = Math.max(9, radius - 5)

    ctx.fillStyle = withAlpha(accent, image ? 0.08 : 0.06)
    roundedRectPath(ctx, bodyX, bodyY, bodyWidth, bodyHeight, bodyRadius)
    ctx.fill()
    ctx.strokeStyle = withAlpha(accent, 0.14)
    ctx.lineWidth = 1
    roundedRectPath(ctx, bodyX, bodyY, bodyWidth, bodyHeight, bodyRadius)
    ctx.stroke()

    if (image) {
      ctx.save()
      roundedRectPath(ctx, bodyX, bodyY, bodyWidth, bodyHeight, bodyRadius)
      ctx.clip()
      ctx.fillStyle = '#f6f9ff'
      ctx.fillRect(bodyX, bodyY, bodyWidth, bodyHeight)
      drawImageContain(ctx, image, bodyX + 6, bodyY + 6, Math.max(0, bodyWidth - 12), Math.max(0, bodyHeight - 12))
      ctx.restore()
      if (frame.text) {
        drawTextLines(ctx, bodyX + 10, Math.max(bodyY + bodyHeight - 22, bodyY + 10), bodyWidth - 20, scale, frame.text)
      }
    } else if (frame.text) {
      drawTextLines(ctx, bodyX + 10, bodyY + 10, bodyWidth - 20, scale, frame.text)
    } else {
      drawPreviewPlaceholder(ctx, bodyX, bodyY, bodyWidth, bodyHeight, frame.label || 'Node', accent)
    }
  }

  ctx.restore()

  ctx.strokeStyle = '#dce3f3'
  ctx.lineWidth = Math.max(1.3, scale)
  roundedRectPath(ctx, x, y, width, height, radius)
  ctx.stroke()
}

export async function renderBoardPreviewDataUrl(state: CanvasBoardSnapshot | null): Promise<string> {
  if (!state) return ''
  const canvas = createPreviewCanvas()
  if (!canvas) return ''

  const frames = collectBoardPreviewFrames(state)
  const bounds = resolveBounds(frames)
  if (!bounds) return ''

  const ctx = canvas.getContext('2d')
  if (!ctx) return ''
  ctx.imageSmoothingEnabled = true
  ctx.imageSmoothingQuality = 'high'

  const connections = collectBoardPreviewConnections(state, frames)
  const { scale, offsetX, offsetY } = scaleBounds(bounds)
  const imageAssets = await buildPreviewImageMap(frames)

  drawDotBackground(ctx)
  connections.forEach(connection => drawConnection(ctx, connection, scale, offsetX, offsetY))
  frames.forEach(frame => drawFrame(ctx, frame, scale, offsetX, offsetY, imageAssets.images))

  try {
    const quality = frames.length > 12 ? 0.72 : 0.78
    return canvas.toDataURL('image/jpeg', quality)
  } finally {
    imageAssets.revoke()
  }
}
