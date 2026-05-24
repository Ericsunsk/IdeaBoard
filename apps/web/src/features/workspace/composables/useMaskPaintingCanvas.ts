const MASK_PAINT_RGB = {
  r: 122,
  g: 107,
  b: 213,
} as const

const MASK_PAINT_COLOR = `rgba(${MASK_PAINT_RGB.r}, ${MASK_PAINT_RGB.g}, ${MASK_PAINT_RGB.b}, 1)`
const MASK_PAINT_ALPHA = 0.5
const MASK_EMPTY_ALPHA_THRESHOLD = 0

export type MaskCanvasPoint = {
  x: number
  y: number
}

export type MaskPaintBrushState = {
  brushSize: number
  drawMode: 'paint' | 'erase'
}

export type MaskCommitResult = {
  executionMaskDataUrl: string
  maskDataUrl: string
}

export function getMaskCanvasPoint(
  clientX: number,
  clientY: number,
  canvas: HTMLCanvasElement,
): MaskCanvasPoint {
  const rect = canvas.getBoundingClientRect()
  if (!rect.width || !rect.height) return { x: 0, y: 0 }
  const x = ((clientX - rect.left) / rect.width) * canvas.width
  const y = ((clientY - rect.top) / rect.height) * canvas.height
  return {
    x: Math.max(0, Math.min(canvas.width, x)),
    y: Math.max(0, Math.min(canvas.height, y)),
  }
}

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
  ctx.save()
  ctx.lineCap = 'round'
  ctx.lineJoin = 'round'
  ctx.lineWidth = Math.max(4, Math.min(48, Number(node.brushSize) || 18))
  if (node.drawMode === 'erase') {
    ctx.globalCompositeOperation = 'destination-out'
    ctx.strokeStyle = '#000000'
  } else {
    ctx.globalCompositeOperation = 'source-over'
    ctx.strokeStyle = MASK_PAINT_COLOR
  }
  ctx.beginPath()
  ctx.moveTo(fromX, fromY)
  ctx.lineTo(toX, toY)
  ctx.stroke()
  ctx.restore()
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

function extractMaskIntensity(
  data: Uint8ClampedArray,
  width: number,
  height: number,
): Float32Array {
  const total = width * height
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
  const values = extractMaskIntensity(source.data, canvas.width, canvas.height)
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
