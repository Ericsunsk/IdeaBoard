import type { ImageMaskDrawMode } from '../model.ts'
import getStroke from 'perfect-freehand'
import { clampMaskBrushSize, MASK_PAINTING_COLOR } from '../config/constants'

export type MaskPaintBrushState = {
  brushSize: number
  drawMode: ImageMaskDrawMode
}

export type MaskStrokePoint = {
  x: number
  y: number
  pressure: number
}

export type MaskStrokeRenderOptions = MaskPaintBrushState & {
  last: boolean
}

export type MaskStrokeEngine = {
  draw: (
    canvas: HTMLCanvasElement,
    points: readonly MaskStrokePoint[],
    options: MaskStrokeRenderOptions,
  ) => void
}

function drawMaskStrokeOutline(
  canvas: HTMLCanvasElement,
  outline: readonly [number, number][],
  drawMode: ImageMaskDrawMode,
) {
  const ctx = canvas.getContext('2d', { willReadFrequently: true })
  if (!ctx || outline.length === 0) return
  const firstPoint = outline[0]
  if (!firstPoint) return

  ctx.save()
  ctx.globalCompositeOperation = drawMode === 'erase' ? 'destination-out' : 'source-over'
  ctx.fillStyle = MASK_PAINTING_COLOR
  ctx.beginPath()
  ctx.moveTo(firstPoint[0], firstPoint[1])
  for (const [x, y] of outline.slice(1)) {
    ctx.lineTo(x, y)
  }
  ctx.closePath()
  ctx.fill()
  ctx.restore()
}

export const maskStrokeEngine: MaskStrokeEngine = {
  draw(canvas, points, options) {
    const outline = getStroke(
      points.map(({ x, y, pressure }) => [x, y, pressure]),
      {
        size: clampMaskBrushSize(options.brushSize),
        thinning: 0,
        smoothing: 0.5,
        streamline: 0.35,
        simulatePressure: false,
        last: options.last,
      },
    )
    drawMaskStrokeOutline(canvas, outline, options.drawMode)
  },
}
