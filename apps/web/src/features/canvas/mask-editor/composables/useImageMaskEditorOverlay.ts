import { computed, onBeforeUnmount, ref, watch, type Ref } from 'vue'
import { usePointerDragSession } from '../../composables/usePointerDragSession'
import { MASK_EDITOR_INTERACTION_KEY } from '../types'
import type {
  ImageMaskDrawMode,
  ImageMaskEditorCommittedData,
  MaskEditorInteractionKey,
} from '../types'
import {
  buildMaskCommitResult,
  drawMaskRasterPreviewToCanvas,
} from '../lib/maskPaintingCanvas'
import { drawMaskStrokeSegment } from '../lib/maskBrushEngine'
import {
  createMaskCanvasPointerProjection,
  type MaskCanvasPointerProjection,
} from '../lib/maskCanvasGeometry'
import {
  clampMaskBrushSize,
  MASK_EDITOR_DEFAULT_BRUSH_SIZE,
  MASK_EDITOR_HISTORY_LIMIT,
} from '../config/maskEditorConstants'
import type { ImageEditorImageRect } from '../../image-editor'

type MaskHistoryEntry = {
  width: number
  height: number
  imageData: ImageData
}

type ImageMaskEditorModeState = {
  nodeId: string
  mode: 'inpaint'
}

type UseImageMaskEditorOverlayOptions = {
  inpaintEditorState: Ref<ImageMaskEditorModeState | null>
  imageRect: Ref<ImageEditorImageRect | null>
  startCanvasInteraction?: (key: MaskEditorInteractionKey) => void
  stopCanvasInteraction?: (key: MaskEditorInteractionKey) => void
}

export function useImageMaskEditorOverlay(options: UseImageMaskEditorOverlayOptions) {
  const drawMode = ref<ImageMaskDrawMode>('paint')
  const brushSize = ref(MASK_EDITOR_DEFAULT_BRUSH_SIZE)
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
  let paintingPointProjection: MaskCanvasPointerProjection | null = null
  let interactionActive = false

  const pointerDragSession = usePointerDragSession<void>({
    onMove: ({ event }) => onPointerMove(event),
    onEnd: finishPainting,
    onCancel: finishPainting,
  })

  const maskEditorNodeId = computed(() => options.inpaintEditorState.value?.nodeId ?? null)
  const maskEditorOverlayStyle = computed<Record<string, string> | null>(() => {
    const rect = options.imageRect.value
    if (!rect) return null
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
    options.startCanvasInteraction?.(MASK_EDITOR_INTERACTION_KEY)
  }

  function stopInteraction() {
    if (!interactionActive) return
    interactionActive = false
    options.stopCanvasInteraction?.(MASK_EDITOR_INTERACTION_KEY)
  }

  function captureMaskHistoryEntry(canvas: HTMLCanvasElement): MaskHistoryEntry | null {
    const ctx = canvas.getContext('2d', { willReadFrequently: true })
    if (!ctx || !canvas.width || !canvas.height) return null
    return {
      width: canvas.width,
      height: canvas.height,
      imageData: ctx.getImageData(0, 0, canvas.width, canvas.height),
    }
  }

  function restoreMaskHistoryEntry(canvas: HTMLCanvasElement, entry: MaskHistoryEntry) {
    const ctx = canvas.getContext('2d', { willReadFrequently: true })
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

  function updateHistoryAvailability(nodeId = maskEditorNodeId.value) {
    canUndoMask.value = nodeId ? readUndoStack(nodeId).length > 0 : false
    canRedoMask.value = nodeId ? readRedoStack(nodeId).length > 0 : false
  }

  function pushUndoHistory(nodeId: string, entry: MaskHistoryEntry | null) {
    if (!entry) return false
    const undoStack = readUndoStack(nodeId).slice()
    undoStack.push(entry)
    undoStacksByNodeId.set(nodeId, undoStack.slice(-MASK_EDITOR_HISTORY_LIMIT))
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
    const ctx = canvas.getContext('2d', { willReadFrequently: true })
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

  function finishPainting() {
    const nodeId = paintingNodeId
    const canvas = paintingCanvas
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
    paintingPointProjection = null
  }

  function stopPainting() {
    const stoppedDragSession = pointerDragSession.endDrag()
    if (!stoppedDragSession) finishPainting()
  }

  function saveMaskDraftForNode(nodeId: string): boolean {
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

  function saveMaskDraft(): boolean {
    const nodeId = maskEditorNodeId.value
    return nodeId ? saveMaskDraftForNode(nodeId) : false
  }

  function onPointerMove(event: PointerEvent) {
    if (!paintingNodeId || !paintingCanvas) return
    if (paintingPointerId != null && event.pointerId !== paintingPointerId) return
    if (!paintingPointProjection) return
    const point = paintingPointProjection(event.clientX, event.clientY)
    drawMaskStrokeSegment(
      {
        brushSize: brushSize.value,
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

  function onCanvasPointerDown(event: PointerEvent) {
    const editorState = options.inpaintEditorState.value
    if (!editorState) return
    if (event.pointerType === 'mouse' && event.button !== 0) return
    const canvas = canvasRefs.get(editorState.nodeId)
    if (!canvas) return
    event.preventDefault()
    event.stopPropagation()
    stopPainting()
    const pointProjection = createMaskCanvasPointerProjection(canvas)
    const point = pointProjection(event.clientX, event.clientY)
    paintingNodeId = editorState.nodeId
    paintingCanvas = canvas
    paintingPointerId = event.pointerId
    paintingBeforeEntry = captureMaskHistoryEntry(canvas)
    paintingPointProjection = pointProjection
    startInteraction()
    pointerDragSession.beginDrag(event, undefined, { target: canvas })
    paintingLastX = point.x
    paintingLastY = point.y
    drawMaskStrokeSegment(
      {
        brushSize: brushSize.value,
        drawMode: drawMode.value,
      },
      canvas,
      point.x,
      point.y,
      point.x + 0.001,
      point.y + 0.001,
    )
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
    const nodeId = maskEditorNodeId.value
    if (!nodeId) return
    const canvas = canvasRefs.get(nodeId)
    const ctx = canvas?.getContext('2d', { willReadFrequently: true })
    if (!canvas || !ctx) return
    const before = captureMaskHistoryEntry(canvas)
    ctx.clearRect(0, 0, canvas.width, canvas.height)
    commitUndoHistoryIfChanged(nodeId, before, canvas)
  }

  function undoMask() {
    const nodeId = maskEditorNodeId.value
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
      redoStacksByNodeId.set(nodeId, redoStack.slice(-MASK_EDITOR_HISTORY_LIMIT))
    }
    undoStacksByNodeId.set(nodeId, undoStack)
    restoreMaskHistoryEntry(canvas, previous)
    updateHistoryAvailability(nodeId)
  }

  function redoMask() {
    const nodeId = maskEditorNodeId.value
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
      undoStacksByNodeId.set(nodeId, undoStack.slice(-MASK_EDITOR_HISTORY_LIMIT))
    }
    redoStacksByNodeId.set(nodeId, redoStack)
    restoreMaskHistoryEntry(canvas, next)
    updateHistoryAvailability(nodeId)
  }

  function commitMaskDraft(): boolean {
    const saved = saveMaskDraft()
    if (!saved) return false
    return true
  }

  function clearCommittedMaskData(nodeId: string) {
    committedMaskDataMap.delete(nodeId)
    undoStacksByNodeId.delete(nodeId)
    redoStacksByNodeId.delete(nodeId)
    updateHistoryAvailability()
  }

  function updateBrushSize(value: number) {
    brushSize.value = clampMaskBrushSize(value)
  }

  watch(maskEditorNodeId, (nodeId) => {
    stopPainting()
    if (!nodeId) {
      updateHistoryAvailability(null)
      return
    }
    drawMode.value = 'paint'
    drawCommittedMask(nodeId)
    updateHistoryAvailability(nodeId)
  })

  onBeforeUnmount(() => {
    stopPainting()
    canvasRefs.clear()
    undoStacksByNodeId.clear()
    redoStacksByNodeId.clear()
  })

  return {
    maskEditorNodeId,
    maskEditorOverlayStyle,
    drawMode,
    brushSize,
    canUndoMask,
    canRedoMask,
    saveMaskDraft,
    commitMaskDraft,
    clearMask,
    undoMask,
    redoMask,
    clearCommittedMaskData,
    setMaskCanvasRef,
    onCanvasPointerDown,
    setDrawMode: (mode: ImageMaskDrawMode) => {
      drawMode.value = mode
    },
    setBrushSize: updateBrushSize,
    getCommittedMaskData,
  }
}
