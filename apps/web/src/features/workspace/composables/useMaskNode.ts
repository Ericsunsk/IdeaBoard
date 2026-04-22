import { nextTick, onUnmounted, type ComponentPublicInstance, type Ref } from 'vue'
import type { CanvasImageItem, MaskNodeItem } from '../types/canvasNodes'

interface UploadResponse {
  code: number
  msg?: string
  data?: {
    url?: string
  }
}

interface MaskNodeMessageApi {
  warning(content: string): unknown
  success(content: string): unknown
  error(content: string): unknown
}

interface DragPayload {
  url: string
  ratio: number
}

interface UseMaskNodeOptions {
  maskNodes: Ref<MaskNodeItem[]>
  activeImageActionId: Ref<number | null>
  showStyleStrengthControl: Ref<boolean>
  message: MaskNodeMessageApi
  uploadImage: (file: File) => Promise<UploadResponse>
  loadImageRatioFromFile: (file: File) => Promise<number | null>
  loadImageRatioFromUrl: (url: string) => Promise<number | null>
  loadImageSizeFromUrl: (url: string) => Promise<{ width: number; height: number } | null>
  activateMaskNode: (id: number) => void
  closeNodeContextMenu: () => void
  readNodeImageDropPayload: (event: DragEvent) => DragPayload | null
}

const maskPaintColor = 'rgba(122, 107, 213, 1)'
const maskPaintAlpha = 0.5
const maskColorR = 122
const maskColorG = 107
const maskColorB = 213
const MASK_NODE_LOG_PREFIX = '[MaskNode]'

export function clampMaskRefineAmount(value: number): number {
  if (!Number.isFinite(value)) return 0
  return Math.max(-100, Math.min(100, Math.round(value)))
}

export function useMaskNode(options: UseMaskNodeOptions) {
  const maskCanvasRefs = new Map<number, HTMLCanvasElement>()
  const maskRasterCache = new Map<number, HTMLImageElement>()

  let paintingMaskNode: MaskNodeItem | null = null
  let paintingMaskCanvas: HTMLCanvasElement | null = null
  let paintingLastX = 0
  let paintingLastY = 0

  function getMaskCanvasWidth(node: MaskNodeItem): number {
    return Math.max(120, Math.round(node.width - 24))
  }

  function getMaskStageHeight(node: MaskNodeItem): number {
    if (!node.sourceImageUrl) return 210
    const stageWidth = getMaskCanvasWidth(node)
    const ratio = Number.isFinite(node.sourceImageRatio) && node.sourceImageRatio > 0
      ? node.sourceImageRatio
      : 1
    const dynamicHeight = Math.round(stageWidth * ratio)
    return Math.max(120, Math.min(420, dynamicHeight))
  }

  async function syncMaskNodeImageRatioByUrl(node: MaskNodeItem, url: string) {
    const ratio = await options.loadImageRatioFromUrl(url)
    if (!ratio || node.sourceImageUrl !== url) return
    node.sourceImageRatio = ratio
  }

  function logMaskNodeError(context: string, error: unknown, detail?: Record<string, unknown>) {
    console.error(`${MASK_NODE_LOG_PREFIX} ${context}`, {
      ...detail,
      error,
    })
  }

  function setMaskCanvasRef(id: number, el: Element | ComponentPublicInstance | null) {
    const canvasEl =
      el instanceof HTMLCanvasElement
        ? el
        : el && '$el' in el && (el as ComponentPublicInstance).$el instanceof HTMLCanvasElement
          ? (el as ComponentPublicInstance).$el
          : null

    if (canvasEl) {
      maskCanvasRefs.set(id, canvasEl)
      const node = options.maskNodes.value.find(item => item.id === id)
      if (node) {
        void nextTick(() => renderMaskNodeMask(node))
      }
      return
    }

    maskCanvasRefs.delete(id)
  }

  function renderMaskNodeMask(node: MaskNodeItem) {
    const canvas = maskCanvasRefs.get(node.id)
    if (!canvas) return
    const ctx = canvas.getContext('2d')
    if (!ctx) return
    ctx.clearRect(0, 0, canvas.width, canvas.height)
    if (!node.maskDataUrl) return

    const cached = maskRasterCache.get(node.id)
    if (cached && cached.src === node.maskDataUrl && cached.complete) {
      drawMaskRasterPreviewToCanvas(cached, canvas)
      return
    }

    const maskUrl = node.maskDataUrl
    const raster = new Image()
    raster.onload = () => {
      if (node.maskDataUrl !== maskUrl) return
      const latestCanvas = maskCanvasRefs.get(node.id)
      if (!latestCanvas) return
      drawMaskRasterPreviewToCanvas(raster, latestCanvas)
    }
    raster.src = maskUrl
    maskRasterCache.set(node.id, raster)
  }

  function paintMaskValuesToCanvas(
    targetCanvas: HTMLCanvasElement,
    values: Float32Array,
    width: number,
    height: number,
    alphaScale: number,
    forceOpaque = false,
  ) {
    const tempCanvas = document.createElement('canvas')
    tempCanvas.width = width
    tempCanvas.height = height
    const tempCtx = tempCanvas.getContext('2d')
    const targetCtx = targetCanvas.getContext('2d')
    if (!tempCtx || !targetCtx) return

    const imageData = tempCtx.createImageData(width, height)
    const { data } = imageData
    for (let i = 0; i < values.length; i += 1) {
      const base = i * 4
      const normalized = Math.max(0, Math.min(1, values[i]))
      const alpha = forceOpaque
        ? (normalized > 0.001 ? 1 : 0)
        : normalized * alphaScale
      data[base] = maskColorR
      data[base + 1] = maskColorG
      data[base + 2] = maskColorB
      data[base + 3] = Math.round(alpha * 255)
    }
    tempCtx.putImageData(imageData, 0, 0)
    targetCtx.clearRect(0, 0, targetCanvas.width, targetCanvas.height)
    targetCtx.drawImage(tempCanvas, 0, 0, targetCanvas.width, targetCanvas.height)
  }

  function drawMaskRasterPreviewToCanvas(raster: CanvasImageSource, targetCanvas: HTMLCanvasElement) {
    const width =
      raster instanceof HTMLImageElement
        ? raster.naturalWidth || raster.width
        : raster instanceof HTMLCanvasElement
          ? raster.width
          : 0
    const height =
      raster instanceof HTMLImageElement
        ? raster.naturalHeight || raster.height
        : raster instanceof HTMLCanvasElement
          ? raster.height
          : 0
    if (!width || !height) return

    const sourceCanvas = document.createElement('canvas')
    sourceCanvas.width = width
    sourceCanvas.height = height
    const sourceCtx = sourceCanvas.getContext('2d')
    if (!sourceCtx) return
    sourceCtx.drawImage(raster, 0, 0, width, height)
    const source = sourceCtx.getImageData(0, 0, width, height)
    const values = extractMaskIntensity(source.data, width, height)
    paintMaskValuesToCanvas(targetCanvas, values, width, height, 1, true)
  }

  function getMaskCanvasPoint(clientX: number, clientY: number, canvas: HTMLCanvasElement): { x: number; y: number } {
    const rect = canvas.getBoundingClientRect()
    if (!rect.width || !rect.height) return { x: 0, y: 0 }
    const x = ((clientX - rect.left) / rect.width) * canvas.width
    const y = ((clientY - rect.top) / rect.height) * canvas.height
    return {
      x: Math.max(0, Math.min(canvas.width, x)),
      y: Math.max(0, Math.min(canvas.height, y)),
    }
  }

  function drawMaskStrokeSegment(
    node: MaskNodeItem,
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
      ctx.strokeStyle = maskPaintColor
    }
    ctx.beginPath()
    ctx.moveTo(fromX, fromY)
    ctx.lineTo(toX, toY)
    ctx.stroke()
    ctx.restore()
  }

  function commitMaskNodeMask(node: MaskNodeItem) {
    const canvas = maskCanvasRefs.get(node.id)
    if (!canvas) return
    try {
      const ctx = canvas.getContext('2d')
      if (!ctx) return
      const source = ctx.getImageData(0, 0, canvas.width, canvas.height)
      const values = extractMaskIntensity(source.data, canvas.width, canvas.height)
      const exportCanvas = document.createElement('canvas')
      exportCanvas.width = canvas.width
      exportCanvas.height = canvas.height
      paintMaskValuesToCanvas(exportCanvas, values, canvas.width, canvas.height, maskPaintAlpha)
      node.maskDataUrl = exportCanvas.toDataURL('image/png')
      const raster = new Image()
      raster.src = node.maskDataUrl
      maskRasterCache.set(node.id, raster)
    } catch (error: unknown) {
      logMaskNodeError('Failed to commit mask canvas to data URL.', error, {
        nodeId: node.id,
        canvasWidth: canvas.width,
        canvasHeight: canvas.height,
      })
    }
  }

  function stopMaskPainting(commit = true) {
    window.removeEventListener('mousemove', onMaskPaintMouseMove, true)
    window.removeEventListener('mouseup', onMaskPaintMouseUp, true)
    if (paintingMaskNode && commit) {
      commitMaskNodeMask(paintingMaskNode)
    }
    paintingMaskNode = null
    paintingMaskCanvas = null
  }

  function onMaskPaintMouseMove(event: MouseEvent) {
    if (!paintingMaskNode || !paintingMaskCanvas) return
    const point = getMaskCanvasPoint(event.clientX, event.clientY, paintingMaskCanvas)
    drawMaskStrokeSegment(
      paintingMaskNode,
      paintingMaskCanvas,
      paintingLastX,
      paintingLastY,
      point.x,
      point.y,
    )
    paintingLastX = point.x
    paintingLastY = point.y
  }

  function onMaskPaintMouseUp() {
    stopMaskPainting(true)
  }

  function drawingInitMaskPoint(node: MaskNodeItem, canvas: HTMLCanvasElement, x: number, y: number) {
    paintingMaskNode = node
    paintingMaskCanvas = canvas
    paintingLastX = x
    paintingLastY = y
    drawMaskStrokeSegment(node, canvas, x, y, x + 0.001, y + 0.001)
    window.addEventListener('mousemove', onMaskPaintMouseMove, true)
    window.addEventListener('mouseup', onMaskPaintMouseUp, true)
  }

  function onMaskCanvasMouseDown(event: MouseEvent, node: MaskNodeItem) {
    if (event.button !== 0) return
    const liveNode = options.maskNodes.value.find(item => item.id === node.id) ?? node
    const canvas = maskCanvasRefs.get(liveNode.id)
    if (!canvas) {
      if (!liveNode.sourceImageUrl) {
        openMaskNodeFilePicker(liveNode)
      }
      return
    }
    options.activateMaskNode(liveNode.id)
    options.closeNodeContextMenu()
    options.activeImageActionId.value = null
    options.showStyleStrengthControl.value = false
    stopMaskPainting(false)
    const point = getMaskCanvasPoint(event.clientX, event.clientY, canvas)
    drawingInitMaskPoint(liveNode, canvas, point.x, point.y)
  }

  async function dataUrlToFile(dataUrl: string, fileName: string): Promise<File> {
    const response = await fetch(dataUrl)
    const blob = await response.blob()
    return new File([blob], fileName, {
      type: blob.type || 'image/png',
    })
  }

  async function buildFullMaskDataUrl(imageUrl: string): Promise<string | null> {
    const size = await options.loadImageSizeFromUrl(imageUrl)
    if (!size) return null
    const limit = 2048
    const scale = Math.min(1, limit / Math.max(size.width, size.height))
    const width = Math.max(1, Math.round(size.width * scale))
    const height = Math.max(1, Math.round(size.height * scale))
    const canvas = document.createElement('canvas')
    canvas.width = width
    canvas.height = height
    const ctx = canvas.getContext('2d')
    if (!ctx) return null
    ctx.fillStyle = `rgba(${maskColorR}, ${maskColorG}, ${maskColorB}, ${maskPaintAlpha})`
    ctx.fillRect(0, 0, width, height)
    return canvas.toDataURL('image/png')
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
      if (alpha > 0) {
        values[i] = alpha
        continue
      }
      const luminance =
        (0.299 * data[base] + 0.587 * data[base + 1] + 0.114 * data[base + 2]) / 255
      values[i] = luminance
    }
    return values
  }

  function runMaskMorphology(
    source: Float32Array,
    width: number,
    height: number,
    mode: 'dilate' | 'erode',
    rounds: number,
  ): Float32Array {
    if (rounds <= 0) return source
    let input = source
    for (let r = 0; r < rounds; r += 1) {
      const output = new Float32Array(input.length)
      for (let y = 0; y < height; y += 1) {
        for (let x = 0; x < width; x += 1) {
          let next = mode === 'dilate' ? 0 : 1
          for (let ky = -1; ky <= 1; ky += 1) {
            const sy = y + ky
            if (sy < 0 || sy >= height) continue
            for (let kx = -1; kx <= 1; kx += 1) {
              const sx = x + kx
              if (sx < 0 || sx >= width) continue
              const val = input[sy * width + sx]
              if (mode === 'dilate') next = Math.max(next, val)
              else next = Math.min(next, val)
            }
          }
          output[y * width + x] = next
        }
      }
      input = output
    }
    return input
  }

  function runMaskFeather(
    source: Float32Array,
    width: number,
    height: number,
    passes: number,
  ): Float32Array {
    if (passes <= 0) return source
    let input = source
    for (let p = 0; p < passes; p += 1) {
      const output = new Float32Array(input.length)
      for (let y = 0; y < height; y += 1) {
        for (let x = 0; x < width; x += 1) {
          let sum = 0
          let count = 0
          for (let ky = -1; ky <= 1; ky += 1) {
            const sy = y + ky
            if (sy < 0 || sy >= height) continue
            for (let kx = -1; kx <= 1; kx += 1) {
              const sx = x + kx
              if (sx < 0 || sx >= width) continue
              sum += input[sy * width + sx]
              count += 1
            }
          }
          output[y * width + x] = count > 0 ? sum / count : 0
        }
      }
      input = output
    }
    return input
  }

  async function refineMaskDataUrl(maskDataUrl: string, refineAmount = 0, invertMask = false): Promise<string> {
    if (!maskDataUrl) return ''
    try {
      const image = new Image()
      await new Promise<void>((resolve, reject) => {
        image.onload = () => resolve()
        image.onerror = () => reject(new Error('Failed to decode mask image'))
        image.src = maskDataUrl
      })
      const width = image.naturalWidth || image.width
      const height = image.naturalHeight || image.height
      if (!width || !height) return maskDataUrl

      const sourceCanvas = document.createElement('canvas')
      sourceCanvas.width = width
      sourceCanvas.height = height
      const sourceCtx = sourceCanvas.getContext('2d')
      if (!sourceCtx) return maskDataUrl
      sourceCtx.drawImage(image, 0, 0, width, height)
      const source = sourceCtx.getImageData(0, 0, width, height)

      let maskValues = extractMaskIntensity(source.data, width, height)
      if (invertMask) {
        for (let i = 0; i < maskValues.length; i += 1) {
          maskValues[i] = 1 - maskValues[i]
        }
      }

      const amount = clampMaskRefineAmount(refineAmount)
      const rounds = Math.round(Math.abs(amount) / 12)
      if (rounds > 0) {
        maskValues = runMaskMorphology(maskValues, width, height, amount > 0 ? 'dilate' : 'erode', rounds)
      }
      if (Math.abs(amount) > 0) {
        maskValues = runMaskFeather(maskValues, width, height, 1)
      }

      const outputCanvas = document.createElement('canvas')
      outputCanvas.width = width
      outputCanvas.height = height
      const outputCtx = outputCanvas.getContext('2d')
      if (!outputCtx) return maskDataUrl
      const output = outputCtx.createImageData(width, height)
      const { data } = output
      for (let i = 0; i < maskValues.length; i += 1) {
        const base = i * 4
        const value = Math.max(0, Math.min(1, maskValues[i]))
        data[base] = maskColorR
        data[base + 1] = maskColorG
        data[base + 2] = maskColorB
        data[base + 3] = Math.round(value * maskPaintAlpha * 255)
      }
      outputCtx.putImageData(output, 0, 0)
      return outputCanvas.toDataURL('image/png')
    } catch (error: unknown) {
      logMaskNodeError('Failed to refine mask data URL.', error, {
        refineAmount,
        invertMask,
      })
      return maskDataUrl
    }
  }

  async function uploadMaskNodeFile(node: MaskNodeItem, file: File) {
    if (!file.type.startsWith('image/')) {
      options.message.warning('请上传图片文件')
      return
    }
    try {
      const ratioFromFile = await options.loadImageRatioFromFile(file)
      const res = await options.uploadImage(file)
      if (res.code === 0 && res.data?.url) {
        node.sourceImageUrl = res.data.url
        node.maskDataUrl = ''
        if (ratioFromFile) node.sourceImageRatio = ratioFromFile
        else void syncMaskNodeImageRatioByUrl(node, res.data.url)
        void nextTick(() => renderMaskNodeMask(node))
        options.message.success('节点图片已更新')
        return
      }
      options.message.error(res.msg || '上传失败')
    } catch (error: unknown) {
      const maybeError = error as { msg?: string; message?: string } | null
      options.message.error(maybeError?.msg || maybeError?.message || '上传失败')
    }
  }

  function openMaskNodeFilePicker(node: MaskNodeItem) {
    const input = document.createElement('input')
    input.type = 'file'
    input.accept = 'image/*'
    input.onchange = () => {
      const file = input.files?.[0]
      if (!file) return
      void uploadMaskNodeFile(node, file)
    }
    input.click()
  }

  function clearMaskNodeImage(node: MaskNodeItem) {
    node.sourceImageUrl = ''
    node.sourceImageRatio = 1
    node.maskDataUrl = ''
    renderMaskNodeMask(node)
  }

  function clearMaskNodeMask(node: MaskNodeItem) {
    node.maskDataUrl = ''
    renderMaskNodeMask(node)
  }

  function onMaskStageDrop(event: DragEvent, node: MaskNodeItem) {
    const payload = options.readNodeImageDropPayload(event)
    if (payload) {
      node.sourceImageUrl = payload.url
      node.maskDataUrl = ''
      if (payload.ratio > 0) node.sourceImageRatio = payload.ratio
      else void syncMaskNodeImageRatioByUrl(node, payload.url)
      void nextTick(() => renderMaskNodeMask(node))
      return
    }
    const file = event.dataTransfer?.files?.[0]
    if (!file) return
    void uploadMaskNodeFile(node, file)
  }

  function importCanvasImageToMaskNodeAtPoint(clientX: number, clientY: number, item: CanvasImageItem): boolean {
    const target = document.elementFromPoint(clientX, clientY) as HTMLElement | null
    const stageEl = target?.closest('.mask-stage[data-mask-id]') as HTMLElement | null
    if (!stageEl) return false
    const maskId = Number(stageEl.dataset.maskId)
    if (!Number.isFinite(maskId)) return false
    const node = options.maskNodes.value.find(n => n.id === maskId)
    if (!node) return false
    node.sourceImageUrl = item.url
    node.maskDataUrl = ''
    if (item.height_ratio && item.height_ratio > 0) node.sourceImageRatio = item.height_ratio
    else void syncMaskNodeImageRatioByUrl(node, item.url)
    void nextTick(() => renderMaskNodeMask(node))
    return true
  }

  function removeMaskRuntimeFor(id: number) {
    maskCanvasRefs.delete(id)
    maskRasterCache.delete(id)
  }

  function resetMaskRuntime() {
    stopMaskPainting(false)
    maskCanvasRefs.clear()
    maskRasterCache.clear()
  }

  onUnmounted(() => {
    stopMaskPainting(false)
  })

  return {
    getMaskCanvasWidth,
    getMaskStageHeight,
    setMaskCanvasRef,
    renderMaskNodeMask,
    onMaskCanvasMouseDown,
    clampMaskRefineAmount,
    dataUrlToFile,
    buildFullMaskDataUrl,
    refineMaskDataUrl,
    openMaskNodeFilePicker,
    clearMaskNodeImage,
    clearMaskNodeMask,
    onMaskStageDrop,
    importCanvasImageToMaskNodeAtPoint,
    removeMaskRuntimeFor,
    resetMaskRuntime,
  }
}
