import { computed, onBeforeUnmount, ref, watch, type Ref } from 'vue'
import { usePointerDragSession } from '../utils/interaction/usePointerDragSession'
import { INPAINT_INTERACTION_KEY } from './types'
import type { ImageMaskDrawMode, ImageInpaintCommittedData, InpaintInteractionKey } from './types'
import { buildMaskCommitResult, drawMaskRasterPreviewToCanvas } from './lib/maskPaintingCanvas'
import {
  maskStrokeEngine,
  type MaskStrokePoint,
  type MaskStrokeRenderOptions,
} from './lib/maskBrushEngine'
import {
  createMaskCanvasPointerProjection,
  type MaskCanvasPointerProjection,
} from './lib/maskCanvasGeometry'
import {
  clampMaskBrushSize,
  INPAINT_DEFAULT_BRUSH_SIZE,
  INPAINT_HISTORY_LIMIT,
} from './config/constants'
import type { RetouchImageRect } from '../retouch'

type MaskHistoryEntry = {
  width: number
  height: number
  imageData: ImageData
}

type ImageInpaintModeState = {
  nodeId: string
  mode: 'inpaint'
}

type UseImageInpaintOverlayOptions = {
  inpaintEditorState: Ref<ImageInpaintModeState | null>
  imageRect: Ref<RetouchImageRect | null>
  startCanvasInteraction?: (key: InpaintInteractionKey) => void
  stopCanvasInteraction?: (key: InpaintInteractionKey) => void
}

export function useImageInpaintOverlay(options: UseImageInpaintOverlayOptions) {
  const drawMode = ref<ImageMaskDrawMode>('paint')
  const brushSize = ref(INPAINT_DEFAULT_BRUSH_SIZE)
  const canUndoMask = ref(false)
  const canRedoMask = ref(false)
  const canvasRefs = new Map<string, HTMLCanvasElement>()
  const committedMaskDataMap = new Map<string, ImageInpaintCommittedData>()
  const undoStacksByNodeId = new Map<string, MaskHistoryEntry[]>()
  const redoStacksByNodeId = new Map<string, MaskHistoryEntry[]>()

  let paintingNodeId: string | null = null
  let paintingCanvas: HTMLCanvasElement | null = null
  let paintingPointerId: number | null = null
  let paintingPoints: MaskStrokePoint[] = []
  let paintingStrokeOptions: Omit<MaskStrokeRenderOptions, 'last'> | null = null
  let paintingBeforeEntry: MaskHistoryEntry | null = null
  let paintingPointProjection: MaskCanvasPointerProjection | null = null
  let interactionActive = false

  const pointerDragSession = usePointerDragSession<void>({
    onMove: ({ event }) => onPointerMove(event),
    onEnd: finishPainting,
    onCancel: finishPainting,
  })

  const inpaintNodeId = computed(() => options.inpaintEditorState.value?.nodeId ?? null)
  const inpaintOverlayStyle = computed<Record<string, string> | null>(() => {
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
    options.startCanvasInteraction?.(INPAINT_INTERACTION_KEY)
  }

  function stopInteraction() {
    if (!interactionActive) return
    interactionActive = false
    options.stopCanvasInteraction?.(INPAINT_INTERACTION_KEY)
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

  function updateHistoryAvailability(nodeId = inpaintNodeId.value) {
    canUndoMask.value = nodeId ? readUndoStack(nodeId).length > 0 : false
    canRedoMask.value = nodeId ? readRedoStack(nodeId).length > 0 : false
  }

  function pushUndoHistory(nodeId: string, entry: MaskHistoryEntry | null) {
    if (!entry) return false
    const undoStack = readUndoStack(nodeId).slice()
    undoStack.push(entry)
    undoStacksByNodeId.set(nodeId, undoStack.slice(-INPAINT_HISTORY_LIMIT))
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
    if (canvas && paintingStrokeOptions && paintingPoints.length > 0) {
      maskStrokeEngine.draw(canvas, paintingPoints, {
        ...paintingStrokeOptions,
        last: true,
      })
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
    paintingPoints = []
    paintingStrokeOptions = null
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
    const nodeId = inpaintNodeId.value
    return nodeId ? saveMaskDraftForNode(nodeId) : false
  }

  function onPointerMove(event: PointerEvent) {
    if (!paintingNodeId || !paintingCanvas) return
    if (paintingPointerId != null && event.pointerId !== paintingPointerId) return
    if (!paintingPointProjection) return
    const point = paintingPointProjection(event.clientX, event.clientY)
    paintingPoints.push({
      x: point.x,
      y: point.y,
      pressure: event.pressure > 0 ? event.pressure : 0.5,
    })
    if (!paintingStrokeOptions) return
    maskStrokeEngine.draw(paintingCanvas, paintingPoints, {
      ...paintingStrokeOptions,
      last: false,
    })
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
    paintingPoints = [
      {
        x: point.x,
        y: point.y,
        pressure: event.pressure > 0 ? event.pressure : 0.5,
      },
    ]
    paintingStrokeOptions = {
      brushSize: brushSize.value,
      drawMode: drawMode.value,
    }
    paintingBeforeEntry = captureMaskHistoryEntry(canvas)
    paintingPointProjection = pointProjection
    startInteraction()
    pointerDragSession.beginDrag(event, undefined, { target: canvas })
    maskStrokeEngine.draw(canvas, paintingPoints, {
      ...paintingStrokeOptions,
      last: false,
    })
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
    const nodeId = inpaintNodeId.value
    if (!nodeId) return
    const canvas = canvasRefs.get(nodeId)
    const ctx = canvas?.getContext('2d', { willReadFrequently: true })
    if (!canvas || !ctx) return
    const before = captureMaskHistoryEntry(canvas)
    ctx.clearRect(0, 0, canvas.width, canvas.height)
    commitUndoHistoryIfChanged(nodeId, before, canvas)
  }

  function undoMask() {
    const nodeId = inpaintNodeId.value
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
      redoStacksByNodeId.set(nodeId, redoStack.slice(-INPAINT_HISTORY_LIMIT))
    }
    undoStacksByNodeId.set(nodeId, undoStack)
    restoreMaskHistoryEntry(canvas, previous)
    updateHistoryAvailability(nodeId)
  }

  function redoMask() {
    const nodeId = inpaintNodeId.value
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
      undoStacksByNodeId.set(nodeId, undoStack.slice(-INPAINT_HISTORY_LIMIT))
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

  watch(inpaintNodeId, (nodeId) => {
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
    inpaintNodeId,
    inpaintOverlayStyle,
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
