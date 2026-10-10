import { computed, onBeforeUnmount, ref, watch, type Ref } from 'vue'
import { usePointerDragSession } from '../../../infrastructure/browser/interaction/usePointerDragSession.ts'
import { INPAINT_INTERACTION_KEY } from '../core/model.ts'
import type {
  ImageMaskDrawMode,
  ImageInpaintCommittedData,
  InpaintInteractionKey,
} from '../core/model.ts'
import { drawMaskRasterPreviewToCanvas } from './maskPaintingCanvas'
import {
  clampMaskBrushSize,
  INPAINT_DEFAULT_BRUSH_SIZE,
  INPAINT_MAX_PENDING_STROKES_PER_NODE,
  INPAINT_MAX_POINTS_PER_STROKE,
  INPAINT_STROKE_MIN_MOVE_DISTANCE_PX,
} from '../core/constants'
import type {
  MaskPainter,
  MaskPainterClientPoint,
  MaskPainterOptions,
  MaskPainterViewport,
} from './fabric/index.ts'
import { loadFabricMaskPainter } from './fabric/index.ts'
import type { RetouchImageRect } from '../../retouch'

type MaskPainterFactory = (
  host: HTMLCanvasElement,
  options: MaskPainterOptions,
) => MaskPainter | Promise<MaskPainter>

type PendingInpaintStroke = {
  drawMode: ImageMaskDrawMode
  points: MaskPainterClientPoint[]
  viewport: MaskPainterViewport
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
  /**
   * Injectable painter factory. Production defaults to the lazy Fabric
   * loader (dynamic import, own chunk); specs inject a fake painter so
   * happy-dom never constructs a real Fabric canvas.
   */
  createMaskPainter?: MaskPainterFactory
}

export function useImageInpaintOverlay(options: UseImageInpaintOverlayOptions) {
  const createMaskPainter = options.createMaskPainter ?? loadFabricMaskPainter
  const drawMode = ref<ImageMaskDrawMode>('paint')
  const brushSize = ref(INPAINT_DEFAULT_BRUSH_SIZE)
  const canUndoMask = ref(false)
  const canRedoMask = ref(false)
  const canvasRefs = new Map<string, HTMLCanvasElement>()
  const paintersByNodeId = new Map<string, MaskPainter>()
  const pendingStrokesByNodeId = new Map<string, PendingInpaintStroke[]>()
  const committedMaskDataMap = new Map<string, ImageInpaintCommittedData>()
  /**
   * Per-node painter creation generation. Every `createPainterForNode` call
   * bumps the counter and captures it; the async attach disposes the painter
   * when the captured generation is stale (element reused or removed while
   * Fabric loaded), so at most the newest creation ever attaches.
   */
  const painterCreationGenerationByNodeId = new Map<string, number>()
  /**
   * Per-node committed-preview generation. Bumped whenever committed data is
   * discarded or the host element changes; the `Image.onload` in
   * `drawCommittedMask` re-checks it (plus data identity) so a stale load
   * never paints over a cleared state or a reused element.
   */
  const committedPreviewGenerationByNodeId = new Map<string, number>()

  function nextPainterCreationGeneration(nodeId: string): number {
    const next = (painterCreationGenerationByNodeId.get(nodeId) ?? 0) + 1
    painterCreationGenerationByNodeId.set(nodeId, next)
    return next
  }

  function invalidatePainterCreations(nodeId: string) {
    nextPainterCreationGeneration(nodeId)
  }

  function bumpCommittedPreviewGeneration(nodeId: string) {
    committedPreviewGenerationByNodeId.set(
      nodeId,
      (committedPreviewGenerationByNodeId.get(nodeId) ?? 0) + 1,
    )
  }

  let paintingNodeId: string | null = null
  let paintingPointerId: number | null = null
  let paintingClientPoints: MaskPainterClientPoint[] = []
  let paintingViewport: MaskPainterViewport | null = null
  let paintingDrawMode: ImageMaskDrawMode = 'paint'
  let liveStrokeNodeId: string | null = null
  let interactionActive = false

  const pointerDragSession = usePointerDragSession<void>({
    onMove: ({ event }) => onPointerMove(event),
    onEnd: () => finishPainting(false),
    onCancel: () => finishPainting(true),
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

  function disposePainterForNode(nodeId: string) {
    const painter = paintersByNodeId.get(nodeId)
    if (!painter) return
    paintersByNodeId.delete(nodeId)
    painter.dispose()
  }

  function updateHistoryAvailability(nodeId = inpaintNodeId.value) {
    const painter = nodeId ? paintersByNodeId.get(nodeId) : undefined
    canUndoMask.value = painter?.canUndo() ?? false
    canRedoMask.value = painter?.canRedo() ?? false
  }

  function applyStroke(nodeId: string, stroke: PendingInpaintStroke) {
    const painter = paintersByNodeId.get(nodeId)
    if (!painter) {
      // Fabric still loading: buffer the stroke instead of dropping it; it
      // flushes once the painter for this host resolves. Capped: keep the
      // newest strokes, drop the oldest, so a slow load cannot grow memory
      // without bound.
      if (canvasRefs.has(nodeId)) {
        const pending = pendingStrokesByNodeId.get(nodeId) ?? []
        pending.push(stroke)
        if (pending.length > INPAINT_MAX_PENDING_STROKES_PER_NODE) {
          pending.splice(0, pending.length - INPAINT_MAX_PENDING_STROKES_PER_NODE)
        }
        pendingStrokesByNodeId.set(nodeId, pending)
      }
      updateHistoryAvailability(nodeId)
      return
    }
    painter.setBrushSize(brushSize.value)
    if (stroke.drawMode === 'erase') painter.eraseStroke(stroke.points, stroke.viewport)
    else painter.paintStroke(stroke.points, stroke.viewport)
    updateHistoryAvailability(nodeId)
  }

  function flushPendingStrokes(nodeId: string) {
    const pending = pendingStrokesByNodeId.get(nodeId)
    if (!pending || pending.length === 0) return
    pendingStrokesByNodeId.delete(nodeId)
    for (const stroke of pending) applyStroke(nodeId, stroke)
  }

  function attachPainterForNode(
    nodeId: string,
    canvas: HTMLCanvasElement,
    painter: MaskPainter,
    restorePreview: boolean,
  ) {
    if (canvasRefs.get(nodeId) !== canvas) {
      painter.dispose()
      return
    }
    // Defensive: the creation generation already guarantees at most the
    // newest creation attaches, but never leak an overwritten painter.
    const existing = paintersByNodeId.get(nodeId)
    if (existing && existing !== painter) existing.dispose()
    paintersByNodeId.set(nodeId, painter)
    // Baseline preview first, then strokes painted while Fabric loaded.
    if (restorePreview) drawCommittedMask(nodeId)
    flushPendingStrokes(nodeId)
    updateHistoryAvailability(nodeId)
  }

  function createPainterForNode(
    nodeId: string,
    canvas: HTMLCanvasElement,
    restorePreview: boolean,
  ) {
    const generation = nextPainterCreationGeneration(nodeId)
    let created: MaskPainter | Promise<MaskPainter>
    try {
      created = createMaskPainter(canvas, {
        width: canvas.width,
        height: canvas.height,
        brushSize: brushSize.value,
      })
    } catch {
      // Fabric stays lazy-optional: a failed load leaves history unavailable
      // and commit returns false, which the chrome surfaces as a warning.
      updateHistoryAvailability(nodeId)
      return
    }
    if (created instanceof Promise) {
      void created.then(
        (painter) => {
          // Element reused or removed while Fabric loaded: the stale
          // creation must never attach (it would overwrite the newest
          // painter and leak); dispose it instead.
          if (painterCreationGenerationByNodeId.get(nodeId) !== generation) {
            painter.dispose()
            return
          }
          attachPainterForNode(nodeId, canvas, painter, restorePreview)
        },
        () => updateHistoryAvailability(nodeId),
      )
      return
    }
    attachPainterForNode(nodeId, canvas, created, restorePreview)
  }

  function getCommittedMaskData(nodeId: string) {
    return committedMaskDataMap.get(nodeId) ?? null
  }

  function drawCommittedMask(nodeId: string) {
    const committed = committedMaskDataMap.get(nodeId)
    const painter = paintersByNodeId.get(nodeId)
    if (painter) {
      // Object-safe baseline: the seam attaches the committed preview as the
      // Fabric background so later stroke renders preserve it. A direct pixel
      // write here would be wiped by the next Fabric render (T3 object fix).
      painter.setBaselineImage(committed?.maskDataUrl ?? null)
      return
    }
    const canvas = canvasRefs.get(nodeId)
    if (!canvas) return
    const ctx = canvas.getContext('2d', { willReadFrequently: true })
    if (!ctx) return
    ctx.clearRect(0, 0, canvas.width, canvas.height)
    if (!committed?.maskDataUrl) return
    const canvasAtStart = canvas
    const dataUrlAtStart = committed.maskDataUrl
    const generationAtStart = committedPreviewGenerationByNodeId.get(nodeId) ?? 0
    const image = new Image()
    image.onload = () => {
      // Stale-load guard: the committed data may have been cleared (or the
      // node switched/element reused) while the bitmap loaded. The token
      // plus data identity re-check drops the draw in that case.
      if (committedPreviewGenerationByNodeId.get(nodeId) !== generationAtStart) return
      const latest = canvasRefs.get(nodeId)
      if (!latest || latest !== canvasAtStart) return
      if (committedMaskDataMap.get(nodeId)?.maskDataUrl !== dataUrlAtStart) return
      // The painter is still absent (loading or failed): a pixel preview is
      // safe because no Fabric render can wipe it yet. `attachPainterForNode`
      // re-baselines through the seam once the painter resolves.
      if (paintersByNodeId.has(nodeId)) return
      drawMaskRasterPreviewToCanvas(image, latest)
      updateHistoryAvailability(nodeId)
    }
    image.src = committed.maskDataUrl
  }

  function finishPainting(cancelled = false) {
    const nodeId = paintingNodeId
    const stroke: PendingInpaintStroke | null =
      nodeId && paintingClientPoints.length > 0 && paintingViewport
        ? {
            drawMode: paintingDrawMode,
            points: paintingClientPoints,
            viewport: paintingViewport,
          }
        : null
    const liveNodeId = liveStrokeNodeId
    paintingNodeId = null
    paintingPointerId = null
    paintingClientPoints = []
    paintingViewport = null
    liveStrokeNodeId = null
    stopInteraction()
    if (!nodeId) return
    if (liveNodeId === nodeId) {
      // The drag rendered incrementally through the live session: ending it
      // records exactly one history entry (cancelling discards the preview).
      // The buffered points must not be replayed as a second one-shot.
      const painter = paintersByNodeId.get(nodeId)
      if (cancelled) painter?.cancelStroke()
      else painter?.endStroke()
      updateHistoryAvailability(nodeId)
      return
    }
    // No live session (the painter was missing during the drag): strokes
    // materialize once per drag session as a one-shot release.
    // The Fabric seam turns a single tap into a dot internally.
    if (!stroke) {
      updateHistoryAvailability(nodeId)
      return
    }
    applyStroke(nodeId, stroke)
  }

  function stopPainting() {
    const stoppedDragSession = pointerDragSession.endDrag()
    // `endDrag` without an event clears listeners silently, so finalize here:
    // an aborted drag (save/remount/node switch mid-drag) discards its
    // partial live preview, matching the old release-on-stroke drop.
    if (stoppedDragSession) finishPainting(true)
    else finishPainting(false)
  }

  function saveMaskDraftForNode(nodeId: string): boolean {
    stopPainting()
    const painter = paintersByNodeId.get(nodeId)
    if (!painter) return false
    const committed = painter.commit()
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
    if (!paintingNodeId) return
    if (paintingPointerId != null && event.pointerId !== paintingPointerId) return
    // Raw client samples only: projection and pressure handling belong to
    // the Fabric seam (constant-width brushes ignore pressure by design).
    const point = { x: event.clientX, y: event.clientY }
    const previous = paintingClientPoints[paintingClientPoints.length - 1]
    if (previous) {
      // Distance decimation: sub-threshold jitter is dropped from both the
      // buffer and the live preview. The floor sits below the minimum brush
      // width, so rendering is visually identical while long drags stay
      // bounded.
      const deltaX = point.x - previous.x
      const deltaY = point.y - previous.y
      const minDistance = INPAINT_STROKE_MIN_MOVE_DISTANCE_PX
      if (deltaX * deltaX + deltaY * deltaY < minDistance * minDistance) return
    }
    if (paintingClientPoints.length >= INPAINT_MAX_POINTS_PER_STROKE) {
      // Sample cap: thin the buffered shape instead of truncating the tail.
      // The live preview already rendered every accepted sample; the buffer
      // only feeds the one-shot fallback when no live session exists.
      paintingClientPoints = paintingClientPoints.filter((_, index) => index % 2 === 0)
    }
    paintingClientPoints.push(point)
    if (liveStrokeNodeId === paintingNodeId) {
      // Incremental live preview: the seam renders this sample immediately
      // without replaying the buffered stroke, so the release path below
      // must not apply these points a second time.
      paintersByNodeId.get(paintingNodeId)?.appendStrokePoints([point])
    }
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
    // Capture the overlay layout once per stroke; the seam projects every
    // point of the stroke against this viewport (no per-move re-read).
    const rect = canvas.getBoundingClientRect()
    paintingNodeId = editorState.nodeId
    paintingPointerId = event.pointerId
    paintingDrawMode = drawMode.value
    paintingViewport = { left: rect.left, top: rect.top, width: rect.width, height: rect.height }
    paintingClientPoints = [{ x: event.clientX, y: event.clientY }]
    startInteraction()
    const painter = paintersByNodeId.get(editorState.nodeId)
    if (painter) {
      // Live incremental session when the painter is ready; otherwise the
      // drag only buffers and the release path applies it as one shot.
      // The down sample opens the session so taps (no moves) still dot.
      painter.setBrushSize(brushSize.value)
      painter.beginStroke(paintingDrawMode, paintingViewport)
      liveStrokeNodeId = editorState.nodeId
      painter.appendStrokePoints([{ x: event.clientX, y: event.clientY }])
    }
    pointerDragSession.beginDrag(event, undefined, { target: canvas })
  }

  function setMaskCanvasRef(nodeId: string, element: Element | null) {
    const canvas = element instanceof HTMLCanvasElement ? element : null
    if (!canvas) {
      if (paintingNodeId === nodeId) stopPainting()
      // Invalidate in-flight creations: their async attach must dispose
      // instead of painting onto a removed (possibly later reused) element.
      invalidatePainterCreations(nodeId)
      bumpCommittedPreviewGeneration(nodeId)
      disposePainterForNode(nodeId)
      canvasRefs.delete(nodeId)
      pendingStrokesByNodeId.delete(nodeId)
      return
    }
    if (paintingNodeId === nodeId) stopPainting()
    disposePainterForNode(nodeId)
    // The new creation (bumped inside `createPainterForNode`) supersedes any
    // in-flight one; a host swap also invalidates pending preview loads.
    bumpCommittedPreviewGeneration(nodeId)
    canvasRefs.set(nodeId, canvas)
    pendingStrokesByNodeId.delete(nodeId)
    updateHistoryAvailability(nodeId)
    void createPainterForNode(nodeId, canvas, true)
  }

  function clearMask() {
    const nodeId = inpaintNodeId.value
    if (!nodeId) return
    pendingStrokesByNodeId.delete(nodeId)
    paintersByNodeId.get(nodeId)?.clear()
    updateHistoryAvailability(nodeId)
  }

  function undoMask() {
    const nodeId = inpaintNodeId.value
    if (!nodeId) return
    const pending = pendingStrokesByNodeId.get(nodeId)
    if (pending && pending.length > 0) {
      // Unflushed strokes never reached the painter: drop the newest instead.
      pending.pop()
      if (pending.length === 0) pendingStrokesByNodeId.delete(nodeId)
      updateHistoryAvailability(nodeId)
      return
    }
    paintersByNodeId.get(nodeId)?.undo()
    updateHistoryAvailability(nodeId)
  }

  function redoMask() {
    const nodeId = inpaintNodeId.value
    if (!nodeId) return
    if (pendingStrokesByNodeId.has(nodeId)) {
      updateHistoryAvailability(nodeId)
      return
    }
    paintersByNodeId.get(nodeId)?.redo()
    updateHistoryAvailability(nodeId)
  }

  function commitMaskDraft(): boolean {
    return saveMaskDraft()
  }

  function clearCommittedMaskData(nodeId: string) {
    committedMaskDataMap.delete(nodeId)
    // Invalidate pending preview loads and painter creations: a bitmap or a
    // Fabric chunk resolving after this point belongs to the discarded state.
    bumpCommittedPreviewGeneration(nodeId)
    invalidatePainterCreations(nodeId)
    // Reset seam history so discarded edits cannot resurface via undo; host
    // pixels stay untouched and no committed preview is restored.
    pendingStrokesByNodeId.delete(nodeId)
    const canvas = canvasRefs.get(nodeId)
    disposePainterForNode(nodeId)
    if (canvas) void createPainterForNode(nodeId, canvas, false)
    updateHistoryAvailability()
  }

  function updateBrushSize(value: number) {
    brushSize.value = clampMaskBrushSize(value)
  }

  watch(inpaintNodeId, (nodeId, previousNodeId) => {
    stopPainting()
    // Switching away invalidates the previous node's pending preview load;
    // switching back re-triggers `drawCommittedMask` for a fresh load.
    if (previousNodeId && previousNodeId !== nodeId) bumpCommittedPreviewGeneration(previousNodeId)
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
    for (const nodeId of paintersByNodeId.keys()) disposePainterForNode(nodeId)
    for (const nodeId of canvasRefs.keys()) invalidatePainterCreations(nodeId)
    canvasRefs.clear()
    pendingStrokesByNodeId.clear()
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
