import { computed, onBeforeUnmount, ref, watch, type Ref } from 'vue'
import type { Node } from '@vue-flow/core'
import type { CanvasInteractionKey } from './useFlowCanvasInteractionState'
import {
  buildMaskCommitResult,
  drawMaskRasterPreviewToCanvas,
  drawMaskStrokeSegment,
  getMaskCanvasPoint,
} from './useMaskPaintingCanvas'
import { containImageRect } from '../utils/canvasExtendGeometry'
import { isEditableEventTarget } from '../utils/domEventTargets'

type ImageDisplayPayload = {
  url: string
  ratio: number
}

export type ImageMaskDrawMode = 'paint' | 'erase'

export type ImageMaskEditorCommittedData = {
  executionMaskDataUrl: string
  maskDataUrl: string
}

type ImageMaskEditorState = {
  nodeId: string
  imageRect: { left: number; top: number; width: number; height: number }
}

type UseImageMaskEditorOverlayOptions = {
  nodes: Ref<Node[]>
  activeTool: Ref<string>
  getImageNodeDisplayPayload: (nodeId: string) => ImageDisplayPayload | null
  getImageStageElement: (nodeId: string) => HTMLElement | null
  onBeforePointerCancel?: () => void
  onCommit?: (payload: { nodeId: string; committed: ImageMaskEditorCommittedData }) => void
  startCanvasInteraction?: (key: CanvasInteractionKey) => void
  stopCanvasInteraction?: (key: CanvasInteractionKey) => void
}

export function useImageMaskEditorOverlay(options: UseImageMaskEditorOverlayOptions) {
  const state = ref<ImageMaskEditorState | null>(null)
  const drawMode = ref<ImageMaskDrawMode>('paint')
  const brushSize = ref(18)
  const canvasRefs = new Map<string, HTMLCanvasElement>()
  const committedMaskDataMap = new Map<string, ImageMaskEditorCommittedData>()

  let paintingNodeId: string | null = null
  let paintingCanvas: HTMLCanvasElement | null = null
  let paintingPointerId: number | null = null
  let paintingLastX = 0
  let paintingLastY = 0
  let interactionActive = false

  const isMaskEditorMode = computed(() => !!state.value)
  const maskEditorNodeId = computed(() => state.value?.nodeId ?? null)
  const activeEditorNodeKind = computed(() => {
    const nodeId = state.value?.nodeId
    if (!nodeId) return null
    return options.nodes.value.find((item) => item.id === nodeId)?.data?.kind ?? null
  })
  const maskEditorOverlayStyle = computed<Record<string, string> | null>(() => {
    if (!state.value) return null
    const rect = state.value.imageRect
    return {
      left: `${rect.left}px`,
      top: `${rect.top}px`,
      width: `${rect.width}px`,
      height: `${rect.height}px`,
    }
  })

  function startInteraction() {
    if (interactionActive) return
    interactionActive = true
    options.startCanvasInteraction?.('image-mask-editor')
  }

  function stopInteraction() {
    if (!interactionActive) return
    interactionActive = false
    options.stopCanvasInteraction?.('image-mask-editor')
  }

  function getCommittedMaskData(nodeId: string) {
    return committedMaskDataMap.get(nodeId) ?? null
  }

  function drawCommittedMask(nodeId: string) {
    const canvas = canvasRefs.get(nodeId)
    const committed = committedMaskDataMap.get(nodeId)
    if (!canvas) return
    const ctx = canvas.getContext('2d')
    if (!ctx) return
    ctx.clearRect(0, 0, canvas.width, canvas.height)
    if (!committed?.maskDataUrl) return
    const image = new Image()
    image.onload = () => {
      const latest = canvasRefs.get(nodeId)
      if (!latest) return
      drawMaskRasterPreviewToCanvas(image, latest)
    }
    image.src = committed.maskDataUrl
  }

  function stopPainting() {
    window.removeEventListener('pointermove', onPointerMove, true)
    window.removeEventListener('pointerup', onPointerUp, true)
    window.removeEventListener('pointercancel', onPointerCancel, true)
    if (paintingCanvas) {
      paintingCanvas.removeEventListener('lostpointercapture', onLostPointerCapture)
    }
    if (paintingCanvas && paintingPointerId != null && paintingCanvas.hasPointerCapture(paintingPointerId)) {
      paintingCanvas.releasePointerCapture(paintingPointerId)
    }
    stopInteraction()
    paintingNodeId = null
    paintingCanvas = null
    paintingPointerId = null
  }

  function cancelMaskEditorMode() {
    stopPainting()
    state.value = null
  }

  function enterMaskEditorMode(nodeId: string): boolean {
    const display = options.getImageNodeDisplayPayload(nodeId)
    if (!display?.url) return false
    const stage = options.getImageStageElement(nodeId)
    if (!stage) return false
    const containRect = containImageRect(stage.clientWidth, stage.clientHeight, display.ratio)
    if (!containRect) return false
    stopPainting()
    state.value = {
      nodeId,
      imageRect: containRect,
    }
    drawMode.value = 'paint'
    drawCommittedMask(nodeId)
    return true
  }

  function onPointerMove(event: PointerEvent) {
    if (!paintingNodeId || !paintingCanvas) return
    if (paintingPointerId != null && event.pointerId !== paintingPointerId) return
    const point = getMaskCanvasPoint(event.clientX, event.clientY, paintingCanvas)
    drawMaskStrokeSegment(
      { brushSize: brushSize.value, drawMode: drawMode.value },
      paintingCanvas,
      paintingLastX,
      paintingLastY,
      point.x,
      point.y,
    )
    paintingLastX = point.x
    paintingLastY = point.y
  }

  function onPointerUp(event: PointerEvent) {
    if (paintingPointerId != null && event.pointerId !== paintingPointerId) return
    stopPainting()
  }

  function onPointerCancel(event: PointerEvent) {
    if (paintingPointerId != null && event.pointerId !== paintingPointerId) return
    stopPainting()
  }

  function onLostPointerCapture(event: PointerEvent) {
    if (paintingPointerId != null && event.pointerId !== paintingPointerId) return
    stopPainting()
  }

  function onCanvasPointerDown(event: PointerEvent) {
    if (!state.value) return
    if (event.pointerType === 'mouse' && event.button !== 0) return
    const canvas = canvasRefs.get(state.value.nodeId)
    if (!canvas) return
    event.preventDefault()
    event.stopPropagation()
    stopPainting()
    const point = getMaskCanvasPoint(event.clientX, event.clientY, canvas)
    paintingNodeId = state.value.nodeId
    paintingCanvas = canvas
    paintingPointerId = event.pointerId
    startInteraction()
    if (!canvas.hasPointerCapture(event.pointerId)) {
      canvas.setPointerCapture(event.pointerId)
    }
    paintingLastX = point.x
    paintingLastY = point.y
    drawMaskStrokeSegment(
      { brushSize: brushSize.value, drawMode: drawMode.value },
      canvas,
      point.x,
      point.y,
      point.x + 0.001,
      point.y + 0.001,
    )
    window.addEventListener('pointermove', onPointerMove, true)
    window.addEventListener('pointerup', onPointerUp, true)
    window.addEventListener('pointercancel', onPointerCancel, true)
    canvas.addEventListener('lostpointercapture', onLostPointerCapture)
  }

  function setMaskCanvasRef(nodeId: string, element: Element | null) {
    const canvas = element instanceof HTMLCanvasElement ? element : null
    if (!canvas) {
      canvasRefs.delete(nodeId)
      if (paintingNodeId === nodeId) stopPainting()
      return
    }
    canvasRefs.set(nodeId, canvas)
    drawCommittedMask(nodeId)
  }

  function clearMask() {
    const nodeId = state.value?.nodeId
    if (!nodeId) return
    const canvas = canvasRefs.get(nodeId)
    const ctx = canvas?.getContext('2d')
    if (!canvas || !ctx) return
    ctx.clearRect(0, 0, canvas.width, canvas.height)
  }

  function commitMaskEditorMode(): boolean {
    const nodeId = state.value?.nodeId
    if (!nodeId) return false
    stopPainting()
    const canvas = canvasRefs.get(nodeId)
    if (!canvas) return false
    const committed = buildMaskCommitResult(canvas)
    if (!committed) {
      committedMaskDataMap.delete(nodeId)
      return false
    }
    committedMaskDataMap.set(nodeId, committed)
    options.onCommit?.({ nodeId, committed })
    cancelMaskEditorMode()
    return true
  }

  function updateBrushSize(value: number) {
    brushSize.value = Math.max(4, Math.min(48, Number(value) || 18))
  }

  function onGlobalPointerDown(event: PointerEvent) {
    if (!state.value || event.button !== 0) return
    const target = event.target as HTMLElement | null
    if (!target) return
    const inEditor = !!target.closest('[data-image-mask-editor]')
    if (inEditor) return
    options.onBeforePointerCancel?.()
    cancelMaskEditorMode()
  }

  function onGlobalKeydown(event: KeyboardEvent) {
    if (!state.value) return
    if (isEditableEventTarget(event.target)) return
    if (event.key === 'Escape') {
      event.preventDefault()
      cancelMaskEditorMode()
    }
  }

  watch(
    () => options.activeTool.value,
    () => {
      if (options.activeTool.value !== 'select' && state.value) {
        cancelMaskEditorMode()
      }
    },
  )

  watch(activeEditorNodeKind, (kind) => {
    if (!state.value) return
    if (kind !== 'image') cancelMaskEditorMode()
  })

  if (typeof window !== 'undefined') {
    window.addEventListener('pointerdown', onGlobalPointerDown, true)
    window.addEventListener('keydown', onGlobalKeydown, true)
  }

  onBeforeUnmount(() => {
    stopPainting()
    canvasRefs.clear()
    if (typeof window !== 'undefined') {
      window.removeEventListener('pointerdown', onGlobalPointerDown, true)
      window.removeEventListener('keydown', onGlobalKeydown, true)
    }
  })

  return {
    isMaskEditorMode,
    maskEditorNodeId,
    maskEditorOverlayStyle,
    drawMode,
    brushSize,
    enterMaskEditorMode,
    cancelMaskEditorMode,
    commitMaskEditorMode,
    clearMask,
    setMaskCanvasRef,
    onCanvasPointerDown,
    setDrawMode: (mode: ImageMaskDrawMode) => {
      drawMode.value = mode
    },
    setBrushSize: updateBrushSize,
    getCommittedMaskData,
  }
}
