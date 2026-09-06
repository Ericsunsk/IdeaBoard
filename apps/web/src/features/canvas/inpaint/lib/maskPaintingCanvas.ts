import {
  MASK_PAINTING_EMPTY_ALPHA_THRESHOLD,
  MASK_PAINTING_RGB,
  MASK_PAINTING_PREVIEW_ALPHA,
} from '../config/inpaintConstants'

export type MaskCommitResult = {
  executionMaskDataUrl: string
  maskDataUrl: string
}

type MaskRgbColor = {
  r: number
  g: number
  b: number
}

function createMaskCanvas(width: number, height: number) {
  const canvas = document.createElement('canvas')
  canvas.width = width
  canvas.height = height
  return canvas
}

function maskColorToRgba(color: MaskRgbColor, alpha = 1) {
  return `rgba(${color.r}, ${color.g}, ${color.b}, ${alpha})`
}

function releaseTemporaryCanvas(canvas: HTMLCanvasElement) {
  canvas.width = 0
  canvas.height = 0
}

function tintCurrentAlpha(
  ctx: CanvasRenderingContext2D,
  width: number,
  height: number,
  color: MaskRgbColor,
  alpha = 1,
) {
  ctx.save()
  ctx.globalCompositeOperation = 'source-in'
  ctx.fillStyle = maskColorToRgba(color, alpha)
  ctx.fillRect(0, 0, width, height)
  ctx.restore()
}

function drawTintedMaskToCanvas(
  targetCanvas: HTMLCanvasElement,
  sourceCanvas: HTMLCanvasElement,
  color: MaskRgbColor,
  alpha = 1,
) {
  const width = targetCanvas.width
  const height = targetCanvas.height
  if (!width || !height) return
  const ctx = targetCanvas.getContext('2d')
  if (!ctx) return
  ctx.clearRect(0, 0, width, height)
  ctx.drawImage(sourceCanvas, 0, 0, width, height)
  tintCurrentAlpha(ctx, width, height, color, alpha)
}

function drawExecutionMaskToCanvas(targetCanvas: HTMLCanvasElement, source: ImageData) {
  const ctx = targetCanvas.getContext('2d')
  if (!ctx) return
  const imageData = ctx.createImageData(source.width, source.height)
  const total = source.width * source.height
  for (let index = 0; index < total; index += 1) {
    const base = index * 4
    const maskValue = source.data[base + 3] ?? 0
    imageData.data[base] = maskValue
    imageData.data[base + 1] = maskValue
    imageData.data[base + 2] = maskValue
    imageData.data[base + 3] = 255
  }
  ctx.putImageData(imageData, 0, 0)
}

export function drawMaskRasterPreviewToCanvas(
  image: CanvasImageSource,
  targetCanvas: HTMLCanvasElement,
) {
  const width = targetCanvas.width
  const height = targetCanvas.height
  if (!width || !height) return
  const tempCanvas = createMaskCanvas(width, height)
  const tempCtx = tempCanvas.getContext('2d', { willReadFrequently: true })
  const targetCtx = targetCanvas.getContext('2d')
  if (!tempCtx || !targetCtx) {
    releaseTemporaryCanvas(tempCanvas)
    return
  }

  tempCtx.clearRect(0, 0, width, height)
  tempCtx.drawImage(image, 0, 0, width, height)
  const source = tempCtx.getImageData(0, 0, width, height)
  const imageData = targetCtx.createImageData(width, height)
  const total = width * height

  for (let i = 0; i < total; i += 1) {
    const base = i * 4
    const hasMask = (source.data[base + 3] ?? 0) > 0
    imageData.data[base] = MASK_PAINTING_RGB.r
    imageData.data[base + 1] = MASK_PAINTING_RGB.g
    imageData.data[base + 2] = MASK_PAINTING_RGB.b
    imageData.data[base + 3] = hasMask ? 255 : 0
  }

  targetCtx.clearRect(0, 0, width, height)
  targetCtx.putImageData(imageData, 0, 0)
  releaseTemporaryCanvas(tempCanvas)
}

function hasMaskPaint(data: Uint8ClampedArray): boolean {
  const alphaThreshold = MASK_PAINTING_EMPTY_ALPHA_THRESHOLD * 255
  for (let idx = 3; idx < data.length; idx += 4) {
    if ((data[idx] ?? 0) > alphaThreshold) return true
  }
  return false
}

export function buildMaskCommitResult(canvas: HTMLCanvasElement): MaskCommitResult | null {
  const ctx = canvas.getContext('2d', { willReadFrequently: true })
  if (!ctx) return null
  const source = ctx.getImageData(0, 0, canvas.width, canvas.height)
  if (!hasMaskPaint(source.data)) return null
  const executionCanvas = createMaskCanvas(canvas.width, canvas.height)
  drawExecutionMaskToCanvas(executionCanvas, source)
  const previewCanvas = createMaskCanvas(canvas.width, canvas.height)
  drawTintedMaskToCanvas(previewCanvas, canvas, MASK_PAINTING_RGB, MASK_PAINTING_PREVIEW_ALPHA)
  const executionMaskDataUrl = executionCanvas.toDataURL('image/png')
  const maskDataUrl = previewCanvas.toDataURL('image/png')
  releaseTemporaryCanvas(executionCanvas)
  releaseTemporaryCanvas(previewCanvas)
  return {
    executionMaskDataUrl,
    maskDataUrl,
  }
}
