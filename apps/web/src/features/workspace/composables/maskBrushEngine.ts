export const MASK_PAINT_RGB = {
  r: 122,
  g: 107,
  b: 213,
} as const

export const MASK_PAINT_COLOR = `rgba(${MASK_PAINT_RGB.r}, ${MASK_PAINT_RGB.g}, ${MASK_PAINT_RGB.b}, 1)`

const MIN_BRUSH_STAMP_SPACING = 0.75
const BRUSH_STAMP_PADDING = 2

type CachedBrushStamp = {
  canvas: HTMLCanvasElement
  radius: number
  offset: number
  spacing: number
}

export type MaskPaintBrushState = {
  brushSize: number
  brushHardness: number
  drawMode: 'paint' | 'erase'
}

const brushStampCache = new Map<string, CachedBrushStamp>()

export function drawMaskStrokeSegment(
  node: MaskPaintBrushState,
  canvas: HTMLCanvasElement,
  fromX: number,
  fromY: number,
  toX: number,
  toY: number,
) {
  const ctx = canvas.getContext('2d')
  if (!ctx) return
  const brushSize = resolveMaskBrushSize(node.brushSize)
  const hardness = resolveMaskBrushHardness(node.brushHardness)
  drawMaskStrokeWithStamp(ctx, node, fromX, fromY, toX, toY, brushSize, hardness)
}

function resolveMaskBrushSize(value: number) {
  return Math.max(4, Math.min(48, Number(value) || 18))
}

function resolveMaskBrushHardness(value: number) {
  return Math.max(0, Math.min(100, Number(value) || 0))
}

function drawMaskStrokeWithStamp(
  ctx: CanvasRenderingContext2D,
  node: MaskPaintBrushState,
  fromX: number,
  fromY: number,
  toX: number,
  toY: number,
  brushSize: number,
  hardness: number,
) {
  const stamp = getOrCreateBrushStamp(brushSize, hardness, node.drawMode)
  const distance = Math.hypot(toX - fromX, toY - fromY)
  const steps = distance > 0 ? Math.max(1, Math.ceil(distance / stamp.spacing)) : 0

  ctx.save()
  ctx.globalCompositeOperation = node.drawMode === 'erase' ? 'destination-out' : 'source-over'
  for (let index = 0; index <= steps; index += 1) {
    const t = steps > 0 ? index / steps : 0
    const x = fromX + (toX - fromX) * t
    const y = fromY + (toY - fromY) * t
    drawBrushStamp(ctx, stamp, x, y)
  }
  ctx.restore()
}

function getOrCreateBrushStamp(
  brushSize: number,
  hardness: number,
  drawMode: MaskPaintBrushState['drawMode'],
): CachedBrushStamp {
  const key = `${brushSize}:${hardness}:${drawMode}`
  const cached = brushStampCache.get(key)
  if (cached) return cached

  const radius = brushSize / 2
  const offset = radius + BRUSH_STAMP_PADDING
  const canvas = document.createElement('canvas')
  canvas.width = brushSize + BRUSH_STAMP_PADDING * 2
  canvas.height = brushSize + BRUSH_STAMP_PADDING * 2
  const ctx = canvas.getContext('2d')
  if (!ctx) {
    const fallback = {
      canvas,
      radius,
      offset,
      spacing: Math.max(MIN_BRUSH_STAMP_SPACING, radius * 0.25),
    }
    brushStampCache.set(key, fallback)
    return fallback
  }

  const clampedHardness = Math.max(0, Math.min(100, hardness))
  const hardnessRatio = clampedHardness / 100
  const innerRadius = radius * hardnessRatio
  const cx = offset
  const cy = offset

  ctx.clearRect(0, 0, canvas.width, canvas.height)
  if (clampedHardness >= 100) {
    ctx.fillStyle = drawMode === 'erase' ? '#000000' : MASK_PAINT_COLOR
  } else {
    const gradient = ctx.createRadialGradient(cx, cy, innerRadius, cx, cy, radius)
    if (drawMode === 'erase') {
      gradient.addColorStop(0, 'rgba(0, 0, 0, 1)')
      gradient.addColorStop(1, 'rgba(0, 0, 0, 0)')
    } else {
      gradient.addColorStop(0, MASK_PAINT_COLOR)
      gradient.addColorStop(1, `rgba(${MASK_PAINT_RGB.r}, ${MASK_PAINT_RGB.g}, ${MASK_PAINT_RGB.b}, 0)`)
    }
    ctx.fillStyle = gradient
  }
  ctx.beginPath()
  ctx.arc(cx, cy, radius, 0, Math.PI * 2)
  ctx.fill()

  const spacing = Math.max(MIN_BRUSH_STAMP_SPACING, radius * (0.18 + (1 - hardnessRatio) * 0.22))
  const created = { canvas, radius, offset, spacing }
  brushStampCache.set(key, created)
  return created
}

function drawBrushStamp(
  ctx: CanvasRenderingContext2D,
  stamp: CachedBrushStamp,
  x: number,
  y: number,
) {
  ctx.drawImage(stamp.canvas, x - stamp.offset, y - stamp.offset)
}
