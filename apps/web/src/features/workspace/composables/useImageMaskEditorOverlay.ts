import { computed, onBeforeUnmount, ref, watch, type Ref } from 'vue'
import type { Node } from '@vue-flow/core'
import type { CanvasInteractionKey } from './useFlowCanvasInteractionState'
import {
  buildMaskCommitResult,
  drawMaskRasterPreviewToCanvas,
  featherMaskPreviewCanvas,
  invertMaskPreviewCanvas,
} from './useMaskPaintingCanvas'
import type { MaskPaintBrushShape } from './maskBrushEngine'
import { drawMaskStrokeSegment } from './maskBrushEngine'
import { containImageRect } from '../utils/canvasExtendGeometry'
import { getMaskCanvasPoint } from '../utils/maskCanvasGeometry'
import { isEditableEventTarget } from '../utils/domEventTargets'

type ImageDisplayPayload = {
  url: string
  ratio: number
}

export type ImageMaskDrawMode = 'paint' | 'erase'
export type ImageMaskBrushShape = MaskPaintBrushShape

export type ImageMaskEditorCommittedData = {
  executionMaskDataUrl: string
  maskDataUrl: string
}

const MAX_FEATHER_RADIUS = 16

function strengthToFeatherRadius(strength: number) {
  const clamped = Math.max(0, Math.min(100, Math.round(strength)))
  if (clamped <= 0) return 0
  return Math.max(1, Math.round((clamped / 100) * MAX_FEATHER_RADIUS))
}

type ImageMaskEditorState = {
  nodeId: string
  imageRect: { left: number; top: number; width: number; height: number }
}

type MaskHistoryEntry = {
  width: number
  height: number
  imageData: ImageData
}

type UseImageMaskEditorOverlayOptions = {
  nodes: Ref<Node[]>
  activeTool: Ref<string>
  getImageNodeDisplayPayload: (nodeId: string) => ImageDisplayPayload | null
  getImageStageElement: (nodeId: string) => HTMLElement | null
  onBeforePointerCancel?: () => void
  startCanvasInteraction?: (key: CanvasInteractionKey) => void
  stopCanvasInteraction?: (key: CanvasInteractionKey) => void
}

export function useImageMaskEditorOverlay(options: UseImageMaskEditorOverlayOptions) {
  const state = ref<ImageMaskEditorState | null>(null)
  const drawMode = ref<ImageMaskDrawMode>('paint')
  const brushSize = ref(18)
  const brushHardness = ref(70)
  const brushShape = ref<ImageMaskBrushShape>('round')
  const canUndoMask = ref(false)
  const canRedoMask = ref(false)
  const canvasRefs = new Map<string, HTMLCanvasElement>()
  const committedMaskDataMap = new Map<string, ImageMaskEditorCommittedData>()
  const undoStacksByNodeId = new Map<string, MaskHistoryEntry[]>()
  const redoStacksByNodeId = new Map<string, MaskHistoryEntry[]>()

  let paintingNodeId: string | null = null
  let paintingCanvas: HTMLCanvasElement | null = null
  let paintingPointerId: number | null = null
  let paintingLastX = 0
  let paintingLastY = 0
  let paintingBeforeEntry: MaskHistoryEntry | null = null
  let interactionActive = false

  const MASK_HISTORY_LIMIT = 40

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

  function captureMaskHistoryEntry(canvas: HTMLCanvasElement): MaskHistoryEntry | null {
    const ctx = canvas.getContext('2d')
    if (!ctx || !canvas.width || !canvas.height) return null
    return {
      width: canvas.width,
      height: canvas.height,
      imageData: ctx.getImageData(0, 0, canvas.width, canvas.height),
    }
  }

  function restoreMaskHistoryEntry(canvas: HTMLCanvasElement, entry: MaskHistoryEntry) {
    const ctx = canvas.getContext('2d')
    if (!ctx) return
    ctx.clearRect(0, 0, canvas.width, canvas.height)
    if (canvas.width === entry.width && canvas.height === entry.height) {
      ctx.putImageData(entry.imageData, 0, 0)
      return
    }
    const sourceCanvas = document.createElement('canvas')
    sourceCanvas.width = entry.width
    sourceCanvas.height = entry.height
    const sourceCtx = sourceCanvas.getContext('2d')
    if (!sourceCtx) return
    sourceCtx.putImageData(entry.imageData, 0, 0)
    ctx.drawImage(sourceCanvas, 0, 0, canvas.width, canvas.height)
  }

  function readUndoStack(nodeId: string) {
    return undoStacksByNodeId.get(nodeId) ?? []
  }

  function readRedoStack(nodeId: string) {
    return redoStacksByNodeId.get(nodeId) ?? []
  }

  function areMaskHistoryEntriesEqual(a: MaskHistoryEntry | null, b: MaskHistoryEntry | null) {
    if (!a || !b) return false
    if (a.width !== b.width || a.height !== b.height) return false
    const aData = a.imageData.data
    const bData = b.imageData.data
    if (aData.length !== bData.length) return false
    for (let idx = 0; idx < aData.length; idx += 1) {
      if (aData[idx] !== bData[idx]) return false
    }
    return true
  }

  function updateHistoryAvailability(nodeId = state.value?.nodeId ?? null) {
    canUndoMask.value = nodeId ? readUndoStack(nodeId).length > 0 : false
    canRedoMask.value = nodeId ? readRedoStack(nodeId).length > 0 : false
  }

  function pushUndoHistory(nodeId: string, entry: MaskHistoryEntry | null) {
    if (!entry) return false
    const undoStack = readUndoStack(nodeId).slice()
    undoStack.push(entry)
    undoStacksByNodeId.set(nodeId, undoStack.slice(-MASK_HISTORY_LIMIT))
    redoStacksByNodeId.set(nodeId, [])
    updateHistoryAvailability(nodeId)
    return true
  }

  function commitUndoHistoryIfChanged(
    nodeId: string,
    before: MaskHistoryEntry | null,
    canvas: HTMLCanvasElement,
  ) {
    const after = captureMaskHistoryEntry(canvas)
    if (areMaskHistoryEntriesEqual(before, after)) {
      updateHistoryAvailability(nodeId)
      return false
    }
    return pushUndoHistory(nodeId, before)
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
      updateHistoryAvailability(nodeId)
    }
    image.src = committed.maskDataUrl
  }

  function stopPainting() {
    const nodeId = paintingNodeId
    const canvas = paintingCanvas
    window.removeEventListener('pointermove', onPointerMove, true)
    window.removeEventListener('pointerup', onPointerUp, true)
    window.removeEventListener('pointercancel', onPointerCancel, true)
    if (canvas) {
      canvas.removeEventListener('lostpointercapture', onLostPointerCapture)
    }
    if (canvas && paintingPointerId != null && canvas.hasPointerCapture(paintingPointerId)) {
      canvas.releasePointerCapture(paintingPointerId)
    }
    stopInteraction()
    if (nodeId && canvas && paintingBeforeEntry) {
      commitUndoHistoryIfChanged(nodeId, paintingBeforeEntry, canvas)
    } else if (nodeId) {
      updateHistoryAvailability(nodeId)
    }
    paintingNodeId = null
    paintingCanvas = null
    paintingPointerId = null
    paintingBeforeEntry = null
  }

  function saveMaskDraft(): boolean {
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
    return true
  }

  function cancelMaskEditorMode() {
    saveMaskDraft()
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
    updateHistoryAvailability(nodeId)
    return true
  }

  function onPointerMove(event: PointerEvent) {
    if (!paintingNodeId || !paintingCanvas) return
    if (paintingPointerId != null && event.pointerId !== paintingPointerId) return
    const point = getMaskCanvasPoint(event.clientX, event.clientY, paintingCanvas)
    drawMaskStrokeSegment(
      {
        brushSize: brushSize.value,
        brushHardness: brushHardness.value,
        brushShape: brushShape.value,
        drawMode: drawMode.value,
      },
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
    paintingBeforeEntry = captureMaskHistoryEntry(canvas)
    startInteraction()
    if (!canvas.hasPointerCapture(event.pointerId)) {
      canvas.setPointerCapture(event.pointerId)
    }
    paintingLastX = point.x
    paintingLastY = point.y
    drawMaskStrokeSegment(
      {
        brushSize: brushSize.value,
        brushHardness: brushHardness.value,
        brushShape: brushShape.value,
        drawMode: drawMode.value,
      },
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
    updateHistoryAvailability(nodeId)
  }

  function clearMask() {
    const nodeId = state.value?.nodeId
    if (!nodeId) return
    const canvas = canvasRefs.get(nodeId)
    const ctx = canvas?.getContext('2d')
    if (!canvas || !ctx) return
    const before = captureMaskHistoryEntry(canvas)
    ctx.clearRect(0, 0, canvas.width, canvas.height)
    commitUndoHistoryIfChanged(nodeId, before, canvas)
  }

  function invertMask() {
    const nodeId = state.value?.nodeId
    if (!nodeId) return
    const canvas = canvasRefs.get(nodeId)
    if (!canvas) return
    const before = captureMaskHistoryEntry(canvas)
    invertMaskPreviewCanvas(canvas)
    commitUndoHistoryIfChanged(nodeId, before, canvas)
  }

  function featherMask(strength = 35) {
    const nodeId = state.value?.nodeId
    if (!nodeId) return
    const canvas = canvasRefs.get(nodeId)
    if (!canvas) return
    const before = captureMaskHistoryEntry(canvas)
    const changed = featherMaskPreviewCanvas(canvas, strengthToFeatherRadius(strength))
    if (!changed) {
      updateHistoryAvailability(nodeId)
      return
    }
    commitUndoHistoryIfChanged(nodeId, before, canvas)
  }

  function undoMask() {
    const nodeId = state.value?.nodeId
    if (!nodeId) return
    const canvas = canvasRefs.get(nodeId)
    if (!canvas) return
    const undoStack = readUndoStack(nodeId).slice()
    const previous = undoStack.pop()
    if (!previous) return
    const current = captureMaskHistoryEntry(canvas)
    if (current) {
      const redoStack = readRedoStack(nodeId).slice()
      redoStack.push(current)
      redoStacksByNodeId.set(nodeId, redoStack.slice(-MASK_HISTORY_LIMIT))
    }
    undoStacksByNodeId.set(nodeId, undoStack)
    restoreMaskHistoryEntry(canvas, previous)
    updateHistoryAvailability(nodeId)
  }

  function redoMask() {
    const nodeId = state.value?.nodeId
    if (!nodeId) return
    const canvas = canvasRefs.get(nodeId)
    if (!canvas) return
    const redoStack = readRedoStack(nodeId).slice()
    const next = redoStack.pop()
    if (!next) return
    const current = captureMaskHistoryEntry(canvas)
    if (current) {
      const undoStack = readUndoStack(nodeId).slice()
      undoStack.push(current)
      undoStacksByNodeId.set(nodeId, undoStack.slice(-MASK_HISTORY_LIMIT))
    }
    redoStacksByNodeId.set(nodeId, redoStack)
    restoreMaskHistoryEntry(canvas, next)
    updateHistoryAvailability(nodeId)
  }

  function commitMaskEditorMode(): boolean {
    const saved = saveMaskDraft()
    state.value = null
    updateHistoryAvailability(null)
    return saved
  }

  function clearCommittedMaskData(nodeId: string) {
    committedMaskDataMap.delete(nodeId)
    undoStacksByNodeId.delete(nodeId)
    redoStacksByNodeId.delete(nodeId)
    updateHistoryAvailability()
  }

  function updateBrushSize(value: number) {
    brushSize.value = Math.max(4, Math.min(48, Number(value) || 18))
  }

  function updateBrushHardness(value: number) {
    brushHardness.value = Math.max(0, Math.min(100, Number(value) || 0))
  }

  function updateBrushShape(value: ImageMaskBrushShape) {
    brushShape.value = value === 'square' ? 'square' : 'round'
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
      updateHistoryAvailability(null)
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
    undoStacksByNodeId.clear()
    redoStacksByNodeId.clear()
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
    brushHardness,
    brushShape,
    canUndoMask,
    canRedoMask,
    enterMaskEditorMode,
    cancelMaskEditorMode,
    commitMaskEditorMode,
    clearMask,
    invertMask,
    featherMask,
    undoMask,
    redoMask,
    clearCommittedMaskData,
    setMaskCanvasRef,
    onCanvasPointerDown,
    setDrawMode: (mode: ImageMaskDrawMode) => {
      drawMode.value = mode
    },
    setBrushSize: updateBrushSize,
    setBrushHardness: updateBrushHardness,
    setBrushShape: updateBrushShape,
    getCommittedMaskData,
  }
}
