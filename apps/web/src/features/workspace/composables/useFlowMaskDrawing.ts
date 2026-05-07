import { nextTick, onUnmounted } from 'vue'
import type { MaskNodeItem } from '../types/canvasNodes'

const MASK_PAINT_COLOR = 'rgba(122, 107, 213, 1)'
const MASK_PAINT_ALPHA = 0.5

type UseFlowMaskDrawingOptions = {
  getMaskNodeSnapshot: (nodeId: string) => MaskNodeItem | null
  updateSnapshotNode: (nodeId: string, patcher: (snapshotNode: Record<string, any>) => Record<string, any>) => void
}

function getMaskPreviewDataUrl(node: Pick<MaskNodeItem, 'maskDataUrl' | 'executionMaskDataUrl'>): string {
  return node.maskDataUrl || node.executionMaskDataUrl || ''
}

export function useFlowMaskDrawing(options: UseFlowMaskDrawingOptions) {
  const maskCanvasRefs = new Map<string, HTMLCanvasElement>()
  const maskRasterCache = new Map<string, HTMLImageElement>()
  let paintingMaskNodeId: string | null = null
  let paintingMaskCanvas: HTMLCanvasElement | null = null
  let paintingPointerId: number | null = null
  let paintingLastX = 0
  let paintingLastY = 0

  function clearMaskCanvas(nodeId: string) {
    const canvas = maskCanvasRefs.get(nodeId)
    const ctx = canvas?.getContext('2d')
    if (canvas && ctx) {
      ctx.clearRect(0, 0, canvas.width, canvas.height)
    }
  }

  function drawMaskRasterPreviewToCanvas(image: CanvasImageSource, targetCanvas: HTMLCanvasElement) {
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
      imageData.data[base] = 122
      imageData.data[base + 1] = 107
      imageData.data[base + 2] = 213
      imageData.data[base + 3] = hasMask ? 255 : 0
    }

    targetCtx.clearRect(0, 0, width, height)
    targetCtx.putImageData(imageData, 0, 0)
  }

  function renderMaskNodeMask(nodeId: string) {
    const canvas = maskCanvasRefs.get(nodeId)
    const node = options.getMaskNodeSnapshot(nodeId)
    if (!canvas || !node) return
    const ctx = canvas.getContext('2d')
    if (!ctx) return
    ctx.clearRect(0, 0, canvas.width, canvas.height)
    const previewMaskDataUrl = getMaskPreviewDataUrl(node)
    if (!previewMaskDataUrl) return
    const cached = maskRasterCache.get(nodeId)
    if (cached && cached.src === previewMaskDataUrl && cached.complete) {
      drawMaskRasterPreviewToCanvas(cached, canvas)
      return
    }
    const image = new Image()
    image.onload = () => {
      const latestCanvas = maskCanvasRefs.get(nodeId)
      const latestNode = options.getMaskNodeSnapshot(nodeId)
      if (!latestCanvas || !latestNode || getMaskPreviewDataUrl(latestNode) !== previewMaskDataUrl) return
      const latestCtx = latestCanvas.getContext('2d')
      if (!latestCtx) return
      latestCtx.clearRect(0, 0, latestCanvas.width, latestCanvas.height)
      drawMaskRasterPreviewToCanvas(image, latestCanvas)
    }
    image.src = previewMaskDataUrl
    maskRasterCache.set(nodeId, image)
  }

  function getMaskCanvasPoint(clientX: number, clientY: number, canvas: HTMLCanvasElement) {
    const rect = canvas.getBoundingClientRect()
    if (!rect.width || !rect.height) return { x: 0, y: 0 }
    const x = ((clientX - rect.left) / rect.width) * canvas.width
    const y = ((clientY - rect.top) / rect.height) * canvas.height
    return {
      x: Math.max(0, Math.min(canvas.width, x)),
      y: Math.max(0, Math.min(canvas.height, y)),
    }
  }

  function drawMaskStrokeSegment(node: MaskNodeItem, canvas: HTMLCanvasElement, fromX: number, fromY: number, toX: number, toY: number) {
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

  function extractMaskIntensity(data: Uint8ClampedArray, width: number, height: number): Float32Array {
    const total = width * height
    const values = new Float32Array(total)
    for (let i = 0; i < total; i += 1) {
      const base = i * 4
      const alpha = data[base + 3] / 255
      values[i] = alpha > 0 ? alpha : 0
    }
    return values
  }

  function paintExecutionMaskValuesToCanvas(targetCanvas: HTMLCanvasElement, values: Float32Array, width: number, height: number) {
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

  function paintPreviewMaskValuesToCanvas(targetCanvas: HTMLCanvasElement, values: Float32Array, width: number, height: number) {
    const ctx = targetCanvas.getContext('2d')
    if (!ctx) return
    const imageData = ctx.createImageData(width, height)
    for (let i = 0; i < values.length; i += 1) {
      const base = i * 4
      const alpha = Math.max(0, Math.min(1, values[i])) * MASK_PAINT_ALPHA
      imageData.data[base] = 122
      imageData.data[base + 1] = 107
      imageData.data[base + 2] = 213
      imageData.data[base + 3] = Math.round(alpha * 255)
    }
    ctx.clearRect(0, 0, width, height)
    ctx.putImageData(imageData, 0, 0)
  }

  function commitMaskNodeMask(nodeId: string) {
    const canvas = maskCanvasRefs.get(nodeId)
    const node = options.getMaskNodeSnapshot(nodeId)
    if (!canvas || !node) return
    const ctx = canvas.getContext('2d')
    if (!ctx) return
    const source = ctx.getImageData(0, 0, canvas.width, canvas.height)
    const values = extractMaskIntensity(source.data, canvas.width, canvas.height)
    const executionCanvas = document.createElement('canvas')
    executionCanvas.width = canvas.width
    executionCanvas.height = canvas.height
    paintExecutionMaskValuesToCanvas(executionCanvas, values, canvas.width, canvas.height)
    const previewCanvas = document.createElement('canvas')
    previewCanvas.width = canvas.width
    previewCanvas.height = canvas.height
    paintPreviewMaskValuesToCanvas(previewCanvas, values, canvas.width, canvas.height)
    const executionMaskDataUrl = executionCanvas.toDataURL('image/png')
    const maskDataUrl = previewCanvas.toDataURL('image/png')
    options.updateSnapshotNode(nodeId, prev => ({
      ...prev,
      executionMaskDataUrl,
      maskDataUrl,
    }))
  }

  function onMaskPaintPointerMove(event: PointerEvent) {
    if (!paintingMaskNodeId || !paintingMaskCanvas) return
    if (paintingPointerId != null && event.pointerId !== paintingPointerId) return
    const node = options.getMaskNodeSnapshot(paintingMaskNodeId)
    if (!node) return
    const point = getMaskCanvasPoint(event.clientX, event.clientY, paintingMaskCanvas)
    drawMaskStrokeSegment(node, paintingMaskCanvas, paintingLastX, paintingLastY, point.x, point.y)
    paintingLastX = point.x
    paintingLastY = point.y
  }

  function onMaskPaintPointerUp(event: PointerEvent) {
    if (paintingPointerId != null && event.pointerId !== paintingPointerId) return
    stopMaskPainting(true)
  }

  function onMaskPaintPointerCancel(event: PointerEvent) {
    if (paintingPointerId != null && event.pointerId !== paintingPointerId) return
    stopMaskPainting(true)
  }

  function stopMaskPainting(commit = true) {
    window.removeEventListener('pointermove', onMaskPaintPointerMove, true)
    window.removeEventListener('pointerup', onMaskPaintPointerUp, true)
    window.removeEventListener('pointercancel', onMaskPaintPointerCancel, true)
    if (paintingMaskCanvas && paintingPointerId != null && paintingMaskCanvas.hasPointerCapture(paintingPointerId)) {
      paintingMaskCanvas.releasePointerCapture(paintingPointerId)
    }
    if (commit && paintingMaskNodeId) {
      commitMaskNodeMask(paintingMaskNodeId)
    }
    paintingPointerId = null
    paintingMaskNodeId = null
    paintingMaskCanvas = null
  }

  function onFlowMaskCanvasMouseDown(nodeId: string, event: PointerEvent) {
    if (event.pointerType === 'mouse' && event.button !== 0) return
    event.preventDefault()
    event.stopPropagation()
    const node = options.getMaskNodeSnapshot(nodeId)
    const canvas = maskCanvasRefs.get(nodeId)
    if (!node || !canvas) return
    stopMaskPainting(false)
    const point = getMaskCanvasPoint(event.clientX, event.clientY, canvas)
    paintingMaskNodeId = nodeId
    paintingMaskCanvas = canvas
    paintingPointerId = event.pointerId
    if (!canvas.hasPointerCapture(event.pointerId)) {
      canvas.setPointerCapture(event.pointerId)
    }
    paintingLastX = point.x
    paintingLastY = point.y
    drawMaskStrokeSegment(node, canvas, point.x, point.y, point.x + 0.001, point.y + 0.001)
    window.addEventListener('pointermove', onMaskPaintPointerMove, true)
    window.addEventListener('pointerup', onMaskPaintPointerUp, true)
    window.addEventListener('pointercancel', onMaskPaintPointerCancel, true)
  }

  function onFlowMaskSetCanvasRef(nodeId: string, element: Element | null) {
    const canvas = element instanceof HTMLCanvasElement ? element : null
    if (!canvas) {
      maskCanvasRefs.delete(nodeId)
      maskRasterCache.delete(nodeId)
      if (paintingMaskNodeId === nodeId) {
        stopMaskPainting(false)
      }
      return
    }
    maskCanvasRefs.set(nodeId, canvas)
    void nextTick(() => renderMaskNodeMask(nodeId))
  }

  function removeMaskRuntimeForNode(nodeId: string) {
    if (paintingMaskNodeId === nodeId) {
      stopMaskPainting(false)
    }
    maskCanvasRefs.delete(nodeId)
    maskRasterCache.delete(nodeId)
  }

  onUnmounted(() => {
    stopMaskPainting(false)
    maskCanvasRefs.clear()
    maskRasterCache.clear()
  })

  return {
    onFlowMaskSetCanvasRef,
    renderMaskNodeMask,
    onFlowMaskCanvasMouseDown,
    stopMaskPainting,
    clearMaskCanvas,
    removeMaskRuntimeForNode,
  }
}
