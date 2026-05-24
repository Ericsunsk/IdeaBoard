import { MASK_PAINT_RGB } from './maskBrushEngine'

const MASK_PAINT_ALPHA = 0.5
const MASK_EMPTY_ALPHA_THRESHOLD = 0

export type MaskCommitResult = {
  executionMaskDataUrl: string
  maskDataUrl: string
}

export function drawMaskRasterPreviewToCanvas(
  image: CanvasImageSource,
  targetCanvas: HTMLCanvasElement,
) {
  const width = targetCanvas.width
  const height = targetCanvas.height
  if (!width || !height) return
  const tempCanvas = document.createElement('canvas')
  tempCanvas.width = width
  tempCanvas.height = height
  const tempCtx = tempCanvas.getContext('2d')
  const targetCtx = targetCanvas.getContext('2d')
  if (!tempCtx || !targetCtx) return

  tempCtx.clearRect(0, 0, width, height)
  tempCtx.drawImage(image, 0, 0, width, height)
  const source = tempCtx.getImageData(0, 0, width, height)
  const imageData = targetCtx.createImageData(width, height)
  const total = width * height

  for (let i = 0; i < total; i += 1) {
    const base = i * 4
    const hasMask = source.data[base + 3] > 0
    imageData.data[base] = MASK_PAINT_RGB.r
    imageData.data[base + 1] = MASK_PAINT_RGB.g
    imageData.data[base + 2] = MASK_PAINT_RGB.b
    imageData.data[base + 3] = hasMask ? 255 : 0
  }

  targetCtx.clearRect(0, 0, width, height)
  targetCtx.putImageData(imageData, 0, 0)
}

export function invertMaskPreviewCanvas(canvas: HTMLCanvasElement) {
  const ctx = canvas.getContext('2d')
  if (!ctx) return
  const imageData = ctx.getImageData(0, 0, canvas.width, canvas.height)
  const data = imageData.data
  for (let idx = 0; idx < data.length; idx += 4) {
    data[idx] = MASK_PAINT_RGB.r
    data[idx + 1] = MASK_PAINT_RGB.g
    data[idx + 2] = MASK_PAINT_RGB.b
    data[idx + 3] = 255 - data[idx + 3]
  }
  ctx.putImageData(imageData, 0, 0)
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
  radius = 6,
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
  for (let i = 0; i < total; i += 1) {
    const base = i * 4
    source[base] = MASK_PAINT_RGB.r
    source[base + 1] = MASK_PAINT_RGB.g
    source[base + 2] = MASK_PAINT_RGB.b
    const nextAlpha = alphaBlurred[i]
    if (source[base + 3] !== nextAlpha) changed = true
    source[base + 3] = nextAlpha
  }
  if (!changed) return false
  ctx.putImageData(imageData, 0, 0)
  return true
}

function extractMaskIntensity(
  data: Uint8ClampedArray,
): Float32Array {
  const total = data.length / 4
  const values = new Float32Array(total)
  for (let i = 0; i < total; i += 1) {
    const base = i * 4
    const alpha = data[base + 3] / 255
    values[i] = alpha > 0 ? alpha : 0
  }
  return values
}

function hasMaskPaint(values: Float32Array): boolean {
  for (const value of values) {
    if (value > MASK_EMPTY_ALPHA_THRESHOLD) return true
  }
  return false
}

function paintExecutionMaskValuesToCanvas(
  targetCanvas: HTMLCanvasElement,
  values: Float32Array,
  width: number,
  height: number,
) {
  const ctx = targetCanvas.getContext('2d')
  if (!ctx) return
  const imageData = ctx.createImageData(width, height)
  for (let i = 0; i < values.length; i += 1) {
    const base = i * 4
    const alpha = Math.max(0, Math.min(1, values[i]))
    imageData.data[base] = 255
    imageData.data[base + 1] = 255
    imageData.data[base + 2] = 255
    imageData.data[base + 3] = Math.round(alpha * 255)
  }
  ctx.clearRect(0, 0, width, height)
  ctx.putImageData(imageData, 0, 0)
}

function paintPreviewMaskValuesToCanvas(
  targetCanvas: HTMLCanvasElement,
  values: Float32Array,
  width: number,
  height: number,
) {
  const ctx = targetCanvas.getContext('2d')
  if (!ctx) return
  const imageData = ctx.createImageData(width, height)
  for (let i = 0; i < values.length; i += 1) {
    const base = i * 4
    const alpha = Math.max(0, Math.min(1, values[i])) * MASK_PAINT_ALPHA
    imageData.data[base] = MASK_PAINT_RGB.r
    imageData.data[base + 1] = MASK_PAINT_RGB.g
    imageData.data[base + 2] = MASK_PAINT_RGB.b
    imageData.data[base + 3] = Math.round(alpha * 255)
  }
  ctx.clearRect(0, 0, width, height)
  ctx.putImageData(imageData, 0, 0)
}

export function buildMaskCommitResult(canvas: HTMLCanvasElement): MaskCommitResult | null {
  const ctx = canvas.getContext('2d')
  if (!ctx) return null
  const source = ctx.getImageData(0, 0, canvas.width, canvas.height)
  const values = extractMaskIntensity(source.data)
  if (!hasMaskPaint(values)) return null
  const executionCanvas = document.createElement('canvas')
  executionCanvas.width = canvas.width
  executionCanvas.height = canvas.height
  paintExecutionMaskValuesToCanvas(executionCanvas, values, canvas.width, canvas.height)
  const previewCanvas = document.createElement('canvas')
  previewCanvas.width = canvas.width
  previewCanvas.height = canvas.height
  paintPreviewMaskValuesToCanvas(previewCanvas, values, canvas.width, canvas.height)
  return {
    executionMaskDataUrl: executionCanvas.toDataURL('image/png'),
    maskDataUrl: previewCanvas.toDataURL('image/png'),
  }
}
