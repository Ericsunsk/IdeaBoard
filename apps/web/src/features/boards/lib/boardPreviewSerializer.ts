import type {
  CanvasBoardSnapshot,
  CanvasConnectionNodeType,
} from '../../workspace/types/canvasNodes'
import {
  DEFAULT_FUSION_BLEND_POINT,
  FUSION_SLOT_LAYOUT,
  getFusionLineAnchors,
  getFusionSlotAnchors,
  normalizeFusionBlendPoint,
} from '../../workspace/utils/fusionBlend'
import { resolveImageNodeStageHeight } from '../../workspace/utils/imageNodeSizing'

export interface BoardPreviewFrame {
  key: string
  kind: 'image' | 'node'
  x: number
  y: number
  width: number
  height: number
  bodyOffsetY: number
  bodyHeight: number
  imageUrl?: string
  inputImageUrls?: string[]
  blendPoint?: { x: number; y: number }
  nodeType?: CanvasConnectionNodeType
  nodeId?: number
}

interface BoardPreviewConnection {
  key: string
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
  title?: unknown
  outputImages?: unknown
  sourceImageUrl?: unknown
  sourceImageRatio?: unknown
  executionMaskDataUrl?: unknown
  maskDataUrl?: unknown
  inputImages?: unknown
  blendPoint?: unknown
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
  { key: 'editNodes', nodeType: 'edit' },
  { key: 'fusionNodes', nodeType: 'fusion' },
  { key: 'enhanceNodes', nodeType: 'enhance' },
  { key: 'wearNodes', nodeType: 'wear' },
] as const satisfies ReadonlyArray<{
  key: keyof CanvasBoardSnapshot
  nodeType: CanvasConnectionNodeType
}>

const PREVIEW_VIEWPORT_WIDTH = 760
const PREVIEW_VIEWPORT_HEIGHT = 490
const PREVIEW_PADDING = 36
const PREVIEW_HIGH_DETAIL_QUALITY = 0.78
const PREVIEW_DENSE_DETAIL_QUALITY = 0.72
const PREVIEW_BACKGROUND = '#ffffff'
const PREVIEW_DOT_COLOR = 'rgba(226, 228, 232, 0.86)'
const PREVIEW_NODE_BORDER = '#e2e8f0'
const PREVIEW_NODE_TEXT = '#334155'
const PREVIEW_EDGE_STROKE = '#9aa4b2'
const USER_DOWNLOAD_ENDPOINT = '/api/user/download'

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
    ratio:
      isFiniteNumber(node.sourceImageRatio) && node.sourceImageRatio > 0
        ? node.sourceImageRatio
        : 1,
  }
}

function getMaskPreview(node: NodeLike): NodeImagePreview | null {
  if (typeof node.executionMaskDataUrl === 'string' && node.executionMaskDataUrl) {
    return {
      url: node.executionMaskDataUrl,
      ratio:
        isFiniteNumber(node.sourceImageRatio) && node.sourceImageRatio > 0
          ? node.sourceImageRatio
          : 1,
    }
  }
  if (typeof node.maskDataUrl === 'string' && node.maskDataUrl) {
    return {
      url: node.maskDataUrl,
      ratio:
        isFiniteNumber(node.sourceImageRatio) && node.sourceImageRatio > 0
          ? node.sourceImageRatio
          : 1,
    }
  }
  return getSourcePreview(node)
}

function resolveNodePreview(
  nodeType: CanvasConnectionNodeType,
  node: NodeLike,
): NodeImagePreview | null {
  if (nodeType === 'mask') return getMaskPreview(node)
  return getLatestOutputPreview(node) ?? getSourcePreview(node)
}

function getFusionInputPreviewUrls(node: NodeLike): string[] {
  if (!Array.isArray(node.inputImages)) return []
  return node.inputImages
    .map((raw) => {
      if (!isRecord(raw)) return null
      const url = typeof raw.url === 'string' ? raw.url.trim() : ''
      return url || null
    })
    .filter((url): url is string => !!url)
}

function resolveNodeBodyHeight(
  nodeType: CanvasConnectionNodeType,
  width: number,
  node: NodeLike,
): number {
  const preview = resolveNodePreview(nodeType, node)
  if (nodeType === 'image') {
    return resolveImageNodeStageHeight(width, preview?.ratio ?? 1, !!preview)
  }
  if (nodeType === 'prompt') return width
  if (nodeType === 'fusion') return width
  if (nodeType === 'mask') {
    return preview ? Math.max(120, Math.min(360, Math.round(width * preview.ratio))) : 172
  }
  return preview ? Math.max(112, Math.min(180, Math.round(width * 0.56))) : 132
}

function resolveNodeHeight(
  nodeType: CanvasConnectionNodeType,
  width: number,
  node: NodeLike,
): number {
  return resolveNodeBodyHeight(nodeType, width, node)
}

function resolveBounds(frames: readonly BoardPreviewFrame[]): BoardPreviewBounds | null {
  if (!frames.length) return null
  const minX = Math.min(...frames.map((frame) => frame.x))
  const minY = Math.min(...frames.map((frame) => frame.y))
  const maxX = Math.max(...frames.map((frame) => frame.x + frame.width))
  const maxY = Math.max(...frames.map((frame) => frame.y + frame.height))
  if (
    !Number.isFinite(minX) ||
    !Number.isFinite(minY) ||
    !Number.isFinite(maxX) ||
    !Number.isFinite(maxY)
  ) {
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

function scaleBounds(bounds: BoardPreviewBounds) {
  const availableWidth = PREVIEW_VIEWPORT_WIDTH - PREVIEW_PADDING * 2
  const availableHeight = PREVIEW_VIEWPORT_HEIGHT - PREVIEW_PADDING * 2
  const scale = clamp(
    Math.min(availableWidth / bounds.width, availableHeight / bounds.height),
    0.08,
    2.4,
  )
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
      const fusionInputUrls = nodeType === 'fusion' ? getFusionInputPreviewUrls(node) : []
      frames.push({
        key: `node-${nodeType}-${node.id}-${index}`,
        kind: 'node',
        x: node.x,
        y: node.y,
        width,
        height: resolveNodeHeight(nodeType, width, node),
        bodyOffsetY: 0,
        bodyHeight: resolveNodeBodyHeight(nodeType, width, node),
        imageUrl: nodeType === 'fusion' ? undefined : preview?.url,
        inputImageUrls: fusionInputUrls,
        blendPoint: nodeType === 'fusion' ? normalizeFusionBlendPoint(node.blendPoint) : undefined,
        nodeType,
        nodeId: node.id,
      })
    })
  })
  return frames
}

export function collectBoardPreviewFrames(state: CanvasBoardSnapshot): BoardPreviewFrame[] {
  return collectNodeFrames(state)
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
    return [
      {
        key: `connection-${connection.id}-${index}`,
        startX: fromFrame.x + fromFrame.width,
        startY: fromFrame.y + fromFrame.bodyOffsetY + fromFrame.bodyHeight / 2,
        endX: toFrame.x,
        endY: toFrame.y + toFrame.bodyOffsetY + toFrame.bodyHeight / 2,
      },
    ]
  })
}

function createPreviewCanvas(): HTMLCanvasElement | null {
  if (typeof document === 'undefined') return null
  const canvas = document.createElement('canvas')
  canvas.width = PREVIEW_VIEWPORT_WIDTH
  canvas.height = PREVIEW_VIEWPORT_HEIGHT
  return canvas
}

function encodePreviewCanvas(canvas: HTMLCanvasElement, dense: boolean): string {
  const quality = dense ? PREVIEW_DENSE_DETAIL_QUALITY : PREVIEW_HIGH_DETAIL_QUALITY
  const candidates = ['image/avif', 'image/webp', 'image/jpeg'] as const

  for (const mimeType of candidates) {
    const dataUrl = canvas.toDataURL(mimeType, quality)
    if (dataUrl.startsWith(`data:${mimeType}`)) return dataUrl
  }

  return canvas.toDataURL('image/jpeg', quality)
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

function drawDotBackground(ctx: CanvasRenderingContext2D) {
  ctx.fillStyle = PREVIEW_BACKGROUND
  ctx.fillRect(0, 0, PREVIEW_VIEWPORT_WIDTH, PREVIEW_VIEWPORT_HEIGHT)

  const gradient = ctx.createLinearGradient(0, 0, 0, PREVIEW_VIEWPORT_HEIGHT)
  gradient.addColorStop(0, '#ffffff')
  gradient.addColorStop(1, '#f8fafc')
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

async function loadPreviewImage(
  url: string,
): Promise<{ image: HTMLImageElement; revokeUrl?: string } | null> {
  const resolvedUrl = toAbsolutePreviewUrl(url)
  if (!resolvedUrl || typeof Image === 'undefined') return null
  let imageSrc = resolvedUrl
  let revokeUrl: string | undefined

  if (shouldProxyPreviewUrl(resolvedUrl)) {
    try {
      const response = await fetch(
        `${USER_DOWNLOAD_ENDPOINT}?url=${encodeURIComponent(resolvedUrl)}`,
        {
          credentials: 'include',
        },
      )
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
        .flatMap((frame) => [
          frame.imageUrl?.trim() || '',
          ...(frame.inputImageUrls?.map((url) => url.trim()) ?? []),
        ])
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
      revokeUrls.forEach((url) => URL.revokeObjectURL(url))
    },
  }
}

function drawImageContain(
  ctx: CanvasRenderingContext2D,
  image: CanvasImageSource,
  x: number,
  y: number,
  width: number,
  height: number,
) {
  const imageWidth =
    (image as HTMLImageElement).naturalWidth || (image as HTMLImageElement).width || width
  const imageHeight =
    (image as HTMLImageElement).naturalHeight || (image as HTMLImageElement).height || height
  const scale = Math.min(width / imageWidth, height / imageHeight)
  const drawWidth = imageWidth * scale
  const drawHeight = imageHeight * scale
  const drawX = x + (width - drawWidth) / 2
  const drawY = y + (height - drawHeight) / 2
  ctx.drawImage(image, drawX, drawY, drawWidth, drawHeight)
}

function drawNodeImageStage(
  ctx: CanvasRenderingContext2D,
  image: CanvasImageSource,
  x: number,
  y: number,
  width: number,
  height: number,
) {
  const stageRadius = Math.max(6, Math.min(12, Math.min(width, height) * 0.12))
  ctx.fillStyle = '#f7f9ff'
  roundedRectPath(ctx, x, y, width, height, stageRadius)
  ctx.fill()
  ctx.strokeStyle = PREVIEW_NODE_BORDER
  ctx.lineWidth = 0.9
  roundedRectPath(ctx, x, y, width, height, stageRadius)
  ctx.stroke()

  ctx.save()
  roundedRectPath(
    ctx,
    x + 2,
    y + 2,
    Math.max(0, width - 4),
    Math.max(0, height - 4),
    Math.max(4, stageRadius - 3),
  )
  ctx.clip()
  drawImageContain(ctx, image, x + 2, y + 2, Math.max(0, width - 4), Math.max(0, height - 4))
  ctx.restore()
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
  ctx.beginPath()
  ctx.moveTo(startX, startY)
  ctx.bezierCurveTo(startX + curve, startY, endX - curve, endY, endX, endY)
  ctx.strokeStyle = withAlpha(PREVIEW_EDGE_STROKE, 0.5)
  ctx.lineWidth = 1.1
  ctx.lineCap = 'round'
  ctx.lineJoin = 'round'
  ctx.stroke()
}

function drawEmptyImageGlyph(ctx: CanvasRenderingContext2D, x: number, y: number, size: number) {
  const strokeWidth = Math.max(0.7, size * 0.011)
  const left = x - size / 2
  const top = y - size / 2
  ctx.strokeStyle = '#a8b3be'
  ctx.lineWidth = strokeWidth
  ctx.lineCap = 'round'
  ctx.lineJoin = 'round'
  roundedRectPath(ctx, left + size * 0.18, top + size * 0.18, size * 0.64, size * 0.64, size * 0.1)
  ctx.stroke()
  ctx.beginPath()
  ctx.arc(left + size * 0.62, top + size * 0.36, size * 0.05, 0, Math.PI * 2)
  ctx.stroke()
  ctx.beginPath()
  ctx.moveTo(left + size * 0.22, top + size * 0.66)
  ctx.lineTo(left + size * 0.4, top + size * 0.48)
  ctx.lineTo(left + size * 0.56, top + size * 0.64)
  ctx.lineTo(left + size * 0.68, top + size * 0.54)
  ctx.lineTo(left + size * 0.8, top + size * 0.68)
  ctx.stroke()
}

function drawPromptPreviewLines(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  width: number,
  height: number,
  scale: number,
) {
  const padding = Math.max(8, 12 * scale)
  const lineHeight = Math.max(8, 14 * scale)
  const lineCount = Math.max(3, Math.min(6, Math.floor((height - padding * 2) / lineHeight)))
  ctx.fillStyle = withAlpha(PREVIEW_NODE_TEXT, 0.24)
  for (let i = 0; i < lineCount; i += 1) {
    const lineWidth = width * (i === lineCount - 1 ? 0.42 : i % 2 === 0 ? 0.76 : 0.62)
    roundedRectPath(
      ctx,
      x + padding,
      y + padding + i * lineHeight,
      Math.max(8, lineWidth - padding * 2),
      Math.max(2, 3 * scale),
      999,
    )
    ctx.fill()
  }
}

function drawExecutionPreviewBody(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  width: number,
  height: number,
  scale: number,
) {
  const padding = Math.max(7, 10 * scale)
  const cardHeight = Math.max(26, Math.min(height * 0.42, 54 * scale))
  ctx.fillStyle = '#f8fafc'
  roundedRectPath(
    ctx,
    x + padding,
    y + padding,
    width - padding * 2,
    cardHeight,
    Math.max(8, 12 * scale),
  )
  ctx.fill()
  ctx.strokeStyle = '#e8ebf3'
  ctx.lineWidth = Math.max(0.7, 1 * scale)
  roundedRectPath(
    ctx,
    x + padding,
    y + padding,
    width - padding * 2,
    cardHeight,
    Math.max(8, 12 * scale),
  )
  ctx.stroke()

  ctx.fillStyle = withAlpha(PREVIEW_NODE_TEXT, 0.22)
  const rowY = y + padding + cardHeight + Math.max(10, 14 * scale)
  roundedRectPath(ctx, x + padding, rowY, width - padding * 2, Math.max(3, 5 * scale), 999)
  ctx.fill()
  roundedRectPath(
    ctx,
    x + padding,
    rowY + Math.max(12, 16 * scale),
    (width - padding * 2) * 0.68,
    Math.max(3, 5 * scale),
    999,
  )
  ctx.fill()
}

function drawFusionNodePreview(
  ctx: CanvasRenderingContext2D,
  frame: BoardPreviewFrame,
  x: number,
  y: number,
  width: number,
  height: number,
  scale: number,
  imageMap: Map<string, HTMLImageElement>,
) {
  const stageInset = Math.max(7, Math.min(14, Math.min(width, height) * 0.08))
  const stageX = x + stageInset
  const stageY = y + stageInset
  const stageWidth = Math.max(0, width - stageInset * 2)
  const stageHeight = Math.max(0, height - stageInset * 2)

  const inputUrls = frame.inputImageUrls ?? []
  const visualCount = Math.max(2, Math.min(FUSION_SLOT_LAYOUT.maxSlots, inputUrls.length || 0))
  const anchors = getFusionSlotAnchors(visualCount, FUSION_SLOT_LAYOUT.fallbackStageWidth)
  const lineAnchors = getFusionLineAnchors(visualCount, FUSION_SLOT_LAYOUT.fallbackStageWidth)
  const blendPoint = frame.blendPoint ?? DEFAULT_FUSION_BLEND_POINT

  const slotSize = Math.max(
    18,
    Math.min(stageWidth, stageHeight) * (visualCount >= 4 ? 0.24 : visualCount === 3 ? 0.27 : 0.3),
  )
  const slotRadius = Math.max(4, slotSize * 0.16)

  lineAnchors.forEach((anchor) => {
    const startX = stageX + blendPoint.x * stageWidth
    const startY = stageY + blendPoint.y * stageHeight
    const endX = stageX + anchor.x * stageWidth
    const endY = stageY + anchor.y * stageHeight
    ctx.beginPath()
    ctx.moveTo(startX, startY)
    ctx.lineTo(endX, endY)
    ctx.strokeStyle = withAlpha('#b9a8f7', 0.5)
    ctx.lineWidth = Math.max(1, scale * 1.2)
    ctx.setLineDash([Math.max(3, scale * 4), Math.max(3, scale * 3)])
    ctx.lineCap = 'round'
    ctx.stroke()
  })
  ctx.setLineDash([])

  anchors.forEach((anchor, index) => {
    const slotCenterX = stageX + anchor.x * stageWidth
    const slotCenterY = stageY + anchor.y * stageHeight
    const slotX = slotCenterX - slotSize / 2
    const slotY = slotCenterY - slotSize / 2
    const inputImage = inputUrls[index] ? imageMap.get(inputUrls[index]!) : null
    ctx.fillStyle = '#ffffff'
    roundedRectPath(ctx, slotX, slotY, slotSize, slotSize, slotRadius)
    ctx.fill()
    ctx.strokeStyle = '#e3e8f4'
    ctx.lineWidth = Math.max(0.8, scale)
    roundedRectPath(ctx, slotX, slotY, slotSize, slotSize, slotRadius)
    ctx.stroke()
    if (inputImage) {
      ctx.save()
      roundedRectPath(
        ctx,
        slotX + 1.5,
        slotY + 1.5,
        Math.max(0, slotSize - 3),
        Math.max(0, slotSize - 3),
        Math.max(3, slotRadius - 2),
      )
      ctx.clip()
      drawImageContain(
        ctx,
        inputImage,
        slotX + 1.5,
        slotY + 1.5,
        Math.max(0, slotSize - 3),
        Math.max(0, slotSize - 3),
      )
      ctx.restore()
      return
    }
    drawEmptyImageGlyph(ctx, slotCenterX, slotCenterY, slotSize * 0.7)
  })

  const centerX = stageX + blendPoint.x * stageWidth
  const centerY = stageY + blendPoint.y * stageHeight
  const centerRadius = Math.max(3.5, Math.min(8, Math.min(stageWidth, stageHeight) * 0.045))
  ctx.fillStyle = '#9278e6'
  ctx.beginPath()
  ctx.arc(centerX, centerY, centerRadius, 0, Math.PI * 2)
  ctx.fill()
  ctx.strokeStyle = '#ffffff'
  ctx.lineWidth = Math.max(0.9, scale)
  ctx.stroke()
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
  const bodyY = y + frame.bodyOffsetY * scale
  const bodyHeight = frame.bodyHeight * scale
  const radius = Math.min(8, Math.max(5, width * 0.025))
  const image = frame.imageUrl ? (imageMap.get(frame.imageUrl) ?? null) : null

  ctx.save()
  roundedRectPath(ctx, x, bodyY, width, bodyHeight, radius)
  ctx.clip()
  ctx.fillStyle = '#ffffff'
  ctx.fillRect(x, bodyY, width, bodyHeight)

  if (frame.nodeType === 'image') {
    if (image) {
      drawImageContain(ctx, image, x, bodyY, width, bodyHeight)
    } else {
      drawEmptyImageGlyph(
        ctx,
        x + width / 2,
        bodyY + bodyHeight / 2,
        Math.min(width, bodyHeight) * 0.62,
      )
    }
  } else if (frame.nodeType === 'prompt') {
    drawPromptPreviewLines(ctx, x, bodyY, width, bodyHeight, scale)
  } else if (frame.nodeType === 'fusion') {
    drawFusionNodePreview(ctx, frame, x, bodyY, width, bodyHeight, scale, imageMap)
  } else if (image) {
    const inset = Math.max(7, Math.min(13, bodyHeight * 0.11))
    drawNodeImageStage(
      ctx,
      image,
      x + inset,
      bodyY + inset,
      Math.max(0, width - inset * 2),
      Math.max(0, bodyHeight - inset * 2),
    )
  } else {
    drawExecutionPreviewBody(ctx, x, bodyY, width, bodyHeight, scale)
  }

  ctx.restore()

  ctx.strokeStyle = PREVIEW_NODE_BORDER
  ctx.lineWidth = 1.5
  roundedRectPath(ctx, x, bodyY, width, bodyHeight, radius)
  ctx.stroke()
}

export async function renderBoardPreviewDataUrl(
  state: CanvasBoardSnapshot | null,
): Promise<string> {
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
  connections.forEach((connection) => drawConnection(ctx, connection, scale, offsetX, offsetY))
  frames.forEach((frame) => drawFrame(ctx, frame, scale, offsetX, offsetY, imageAssets.images))

  try {
    return encodePreviewCanvas(canvas, frames.length > 12)
  } finally {
    imageAssets.revoke()
  }
}
