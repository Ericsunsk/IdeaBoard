import type { ImageMaskDrawMode } from '../types'
import { clampMaskBrushSize, MASK_EDITOR_PAINT_COLOR } from '../config/maskEditorConstants'

export type MaskPaintBrushState = {
  brushSize: number
  drawMode: ImageMaskDrawMode
}

export function drawMaskStrokeSegment(
  brush: MaskPaintBrushState,
  canvas: HTMLCanvasElement,
  fromX: number,
  fromY: number,
  toX: number,
  toY: number,
) {
  const ctx = canvas.getContext('2d', { willReadFrequently: true })
  if (!ctx) return

  ctx.save()
  ctx.globalCompositeOperation = brush.drawMode === 'erase' ? 'destination-out' : 'source-over'
  ctx.strokeStyle = MASK_EDITOR_PAINT_COLOR
  ctx.lineCap = 'round'
  ctx.lineJoin = 'round'
  ctx.lineWidth = clampMaskBrushSize(brush.brushSize)
  ctx.beginPath()
  ctx.moveTo(fromX, fromY)
  ctx.lineTo(toX, toY)
  ctx.stroke()
  ctx.restore()
}
