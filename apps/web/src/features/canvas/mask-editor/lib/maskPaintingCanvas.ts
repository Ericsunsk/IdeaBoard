import {
  MASK_EDITOR_EMPTY_ALPHA_THRESHOLD,
  MASK_EDITOR_DEFAULT_FEATHER_RADIUS,
  MASK_EDITOR_PAINT_RGB,
  MASK_EDITOR_PREVIEW_ALPHA,
} from '../config/maskEditorConstants'

export type MaskCommitResult = {
  executionMaskDataUrl: string
  maskDataUrl: string
}

type MaskRgbColor = {
  r: number
  g: number
  b: number
}

const MASK_EXECUTION_RGB = {
  r: 255,
  g: 255,
  b: 255,
} as const

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

function copyCanvasToTemporaryCanvas(sourceCanvas: HTMLCanvasElement): HTMLCanvasElement | null {
  const width = sourceCanvas.width
  const height = sourceCanvas.height
  if (!width || !height) return null
  const copyCanvas = createMaskCanvas(width, height)
  const copyCtx = copyCanvas.getContext('2d')
  if (!copyCtx) {
    releaseTemporaryCanvas(copyCanvas)
    return null
  }
  copyCtx.drawImage(sourceCanvas, 0, 0, width, height)
  return copyCanvas
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

export function drawMaskRasterPreviewToCanvas(
  image: CanvasImageSource,
  targetCanvas: HTMLCanvasElement,
) {
  const width = targetCanvas.width
  const height = targetCanvas.height
  if (!width || !height) return
  const tempCanvas = createMaskCanvas(width, height)
  const tempCtx = tempCanvas.getContext('2d')
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
    const hasMask = source.data[base + 3] > 0
    imageData.data[base] = MASK_EDITOR_PAINT_RGB.r
    imageData.data[base + 1] = MASK_EDITOR_PAINT_RGB.g
    imageData.data[base + 2] = MASK_EDITOR_PAINT_RGB.b
    imageData.data[base + 3] = hasMask ? 255 : 0
  }

  targetCtx.clearRect(0, 0, width, height)
  targetCtx.putImageData(imageData, 0, 0)
  releaseTemporaryCanvas(tempCanvas)
}

export function invertMaskPreviewCanvas(canvas: HTMLCanvasElement) {
  const width = canvas.width
  const height = canvas.height
  if (!width || !height) return
  const ctx = canvas.getContext('2d')
  if (!ctx) return
  const sourceCanvas = copyCanvasToTemporaryCanvas(canvas)
  if (!sourceCanvas) return

  ctx.save()
  ctx.clearRect(0, 0, width, height)
  ctx.globalCompositeOperation = 'source-over'
  ctx.fillStyle = maskColorToRgba(MASK_EDITOR_PAINT_RGB)
  ctx.fillRect(0, 0, width, height)
  ctx.globalCompositeOperation = 'destination-out'
  ctx.drawImage(sourceCanvas, 0, 0, width, height)
  ctx.restore()
  releaseTemporaryCanvas(sourceCanvas)
}

function blurAlphaHorizontal(
  source: Uint8ClampedArray,
  target: Uint8ClampedArray,
  width: number,
  height: number,
  radius: number,
) {
  const windowSize = radius * 2 + 1
  for (let y = 0; y < height; y += 1) {
    const rowOffset = y * width
    let sum = 0
    for (let k = -radius; k <= radius; k += 1) {
      const x = Math.min(width - 1, Math.max(0, k))
      sum += source[rowOffset + x]
    }
    for (let x = 0; x < width; x += 1) {
      target[rowOffset + x] = Math.round(sum / windowSize)
      const removeX = Math.max(0, x - radius)
      const addX = Math.min(width - 1, x + radius + 1)
      sum += source[rowOffset + addX] - source[rowOffset + removeX]
    }
  }
}

function blurAlphaVertical(
  source: Uint8ClampedArray,
  target: Uint8ClampedArray,
  width: number,
  height: number,
  radius: number,
) {
  const windowSize = radius * 2 + 1
  for (let x = 0; x < width; x += 1) {
    let sum = 0
    for (let k = -radius; k <= radius; k += 1) {
      const y = Math.min(height - 1, Math.max(0, k))
      sum += source[y * width + x]
    }
    for (let y = 0; y < height; y += 1) {
      target[y * width + x] = Math.round(sum / windowSize)
      const removeY = Math.max(0, y - radius)
      const addY = Math.min(height - 1, y + radius + 1)
      sum += source[addY * width + x] - source[removeY * width + x]
    }
  }
}

export function featherMaskPreviewCanvas(
  canvas: HTMLCanvasElement,
  radius = MASK_EDITOR_DEFAULT_FEATHER_RADIUS,
): boolean {
  const width = canvas.width
  const height = canvas.height
  if (!width || !height) return false
  const ctx = canvas.getContext('2d')
  if (!ctx) return false
  const blurRadius = Math.floor(radius)
  if (blurRadius <= 0) return false
  const imageData = ctx.getImageData(0, 0, width, height)
  const source = imageData.data
  const total = width * height
  const alphaSource = new Uint8ClampedArray(total)
  let hasMask = false
  for (let i = 0; i < total; i += 1) {
    const alpha = source[i * 4 + 3]
    alphaSource[i] = alpha
    if (alpha > 0) hasMask = true
  }
  if (!hasMask) return false

  const alphaTemp = new Uint8ClampedArray(total)
  const alphaBlurred = new Uint8ClampedArray(total)
  blurAlphaHorizontal(alphaSource, alphaTemp, width, height, blurRadius)
  blurAlphaVertical(alphaTemp, alphaBlurred, width, height, blurRadius)

  let changed = false
  const blurredImageData = ctx.createImageData(width, height)
  for (let i = 0; i < total; i += 1) {
    const base = i * 4
    const nextAlpha = alphaBlurred[i]
    if (alphaSource[i] !== nextAlpha) changed = true
    blurredImageData.data[base + 3] = nextAlpha
  }
  if (!changed) return false
  ctx.putImageData(blurredImageData, 0, 0)
  tintCurrentAlpha(ctx, width, height, MASK_EDITOR_PAINT_RGB)
  return true
}

function hasMaskPaint(data: Uint8ClampedArray): boolean {
  const alphaThreshold = MASK_EDITOR_EMPTY_ALPHA_THRESHOLD * 255
  for (let idx = 3; idx < data.length; idx += 4) {
    if (data[idx] > alphaThreshold) return true
  }
  return false
}

export function buildMaskCommitResult(canvas: HTMLCanvasElement): MaskCommitResult | null {
  const ctx = canvas.getContext('2d')
  if (!ctx) return null
  const source = ctx.getImageData(0, 0, canvas.width, canvas.height)
  if (!hasMaskPaint(source.data)) return null
  const executionCanvas = createMaskCanvas(canvas.width, canvas.height)
  drawTintedMaskToCanvas(executionCanvas, canvas, MASK_EXECUTION_RGB)
  const previewCanvas = createMaskCanvas(canvas.width, canvas.height)
  drawTintedMaskToCanvas(previewCanvas, canvas, MASK_EDITOR_PAINT_RGB, MASK_EDITOR_PREVIEW_ALPHA)
  const executionMaskDataUrl = executionCanvas.toDataURL('image/png')
  const maskDataUrl = previewCanvas.toDataURL('image/png')
  releaseTemporaryCanvas(executionCanvas)
  releaseTemporaryCanvas(previewCanvas)
  return {
    executionMaskDataUrl,
    maskDataUrl,
  }
}
