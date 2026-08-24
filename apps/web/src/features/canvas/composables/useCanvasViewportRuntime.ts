import { computed, getCurrentScope, onScopeDispose, type Ref } from 'vue'
import {
  beginCanvasFrameProbe,
  isCanvasFrameProbeEnabled,
  type CanvasFrameProbeSession,
} from '../utils/canvasFrameProbe'
import { waitForNextRenderFrame } from '../utils/animationFrame'
import {
  createCanvasViewportPersistence,
  type CanvasViewportPersistence,
  type CanvasViewportPersistenceIdentity,
} from '../utils/canvasViewportPersistence'

export type CanvasFlowViewport = {
  x: number
  y: number
  zoom: number
}

export type CanvasFlowMovePayload = {
  flowTransform?: {
    x?: number
    y?: number
    zoom?: number
  }
}

export type CanvasViewportRuntimeInteraction = 'flow-viewport' | 'viewport-pan' | 'node-drag'

export type CanvasMiniMapCenterRequestPayload = {
  position: {
    x: number
    y: number
  }
}

export type CanvasMiniMapDragDelta = {
  x: number
  y: number
}

type ReadonlyValueRef<T> = {
  readonly value: T
}

export type UseCanvasViewportRuntimeOptions = {
  nodes: ReadonlyValueRef<readonly unknown[]>
  edges: ReadonlyValueRef<readonly unknown[]>
  currentBoardId: ReadonlyValueRef<string | null>
  getUserUid: () => string
  viewport: Readonly<Ref<CanvasFlowViewport>>
  getViewport: () => CanvasFlowViewport
  setViewport: (
    viewport: CanvasFlowViewport,
    options?: { duration?: number },
  ) => Promise<unknown> | unknown
  setCenter: (
    x: number,
    y: number,
    options?: { duration?: number; zoom?: number },
  ) => Promise<unknown> | unknown
  zoomIn: () => Promise<unknown> | unknown
  zoomOut: () => Promise<unknown> | unknown
  panBy: (delta: { x: number; y: number }) => boolean
  loadInitialOrRouteBoard: () => Promise<void>
  closeBoardHeaderEditing: () => void
  startCanvasInteraction: (key: CanvasViewportRuntimeInteraction) => void
  stopCanvasInteraction: (key: CanvasViewportRuntimeInteraction) => void
  viewportPersistence?: CanvasViewportPersistence
}

function viewportKey(viewport: CanvasFlowViewport) {
  return `${viewport.x}:${viewport.y}:${viewport.zoom}`
}

export function useCanvasViewportRuntime(options: UseCanvasViewportRuntimeOptions) {
  let activeViewportFrameProbe: CanvasFrameProbeSession | null = null
  let activeNodeDragFrameProbe: CanvasFrameProbeSession | null = null
  let isMiniMapDragActive = false
  let didMiniMapDragMove = false
  let pendingMiniMapDragDelta: CanvasMiniMapDragDelta = { x: 0, y: 0 }
  let pendingMiniMapDragFrameId: number | null = null
  let pendingMiniMapDragEnd = false
  let miniMapDragSessionId = 0
  let miniMapDragTargetViewport: CanvasFlowViewport | null = null
  let isMiniMapViewportUpdateInFlight = false
  let lastRequestedMiniMapViewportKey: string | null = null
  let lastSuccessfulMiniMapViewportKey: string | null = null
  let programmaticViewportCommitDepth = 0
  let viewportMoveStartZoom: number | null = null

  const viewportPersistence =
    options.viewportPersistence ??
    createCanvasViewportPersistence({
      getIdentity: getViewportStorageIdentity,
      getViewport: options.getViewport,
      setViewport: options.setViewport,
    })

  const zoomLevel = computed(() => {
    const zoom = options.viewport.value.zoom
    return Math.max(10, Math.round((Number.isFinite(zoom) && zoom > 0 ? zoom : 1) * 100))
  })

  function getCanvasFrameProbeMetadata() {
    const viewport = options.viewport.value
    return {
      boardId: options.currentBoardId.value,
      nodes: options.nodes.value.length,
      edges: options.edges.value.length,
      viewportX: Math.round(viewport.x),
      viewportY: Math.round(viewport.y),
      zoomPct: Math.round(viewport.zoom * 100),
    }
  }

  function getViewportStorageIdentity(): CanvasViewportPersistenceIdentity {
    return {
      userUid: options.getUserUid(),
      boardId: options.currentBoardId.value,
    }
  }

  async function restoreViewportForCurrentBoard() {
    programmaticViewportCommitDepth += 1
    try {
      await viewportPersistence.restore()
    } finally {
      programmaticViewportCommitDepth -= 1
    }
  }

  async function runAndPersistViewportCommand(run: () => Promise<unknown> | unknown) {
    programmaticViewportCommitDepth += 1
    try {
      const result = await run()
      if (result === false) return

      viewportPersistence.scheduleDelayedPersist()
    } finally {
      programmaticViewportCommitDepth -= 1
    }
  }

  function onZoomIn() {
    void runAndPersistViewportCommand(options.zoomIn)
  }

  function onZoomOut() {
    void runAndPersistViewportCommand(options.zoomOut)
  }

  function onPanViewport(delta: { x: number; y: number }) {
    programmaticViewportCommitDepth += 1
    try {
      if (!options.panBy(delta)) return
      viewportPersistence.scheduleDelayedPersist()
    } finally {
      programmaticViewportCommitDepth -= 1
    }
  }

  function onMiniMapCenterRequested(payload: CanvasMiniMapCenterRequestPayload) {
    const zoom = options.getViewport().zoom
    void runAndPersistViewportCommand(() =>
      options.setCenter(payload.position.x, payload.position.y, { duration: 0, zoom }),
    )
  }

  function onMiniMapDragStart() {
    miniMapDragSessionId += 1
    clearPendingMiniMapDragFrame()
    pendingMiniMapDragDelta = { x: 0, y: 0 }
    pendingMiniMapDragEnd = false
    miniMapDragTargetViewport = options.getViewport()
    lastRequestedMiniMapViewportKey = null
    lastSuccessfulMiniMapViewportKey = null
    didMiniMapDragMove = false
    isMiniMapDragActive = true
    onFlowMoveStart()
  }

  function onMiniMapDragMove(delta: CanvasMiniMapDragDelta) {
    if (!isMiniMapDragActive) return
    if (![delta.x, delta.y].every(Number.isFinite)) return
    if (!delta.x && !delta.y) return

    pendingMiniMapDragDelta = {
      x: pendingMiniMapDragDelta.x + delta.x,
      y: pendingMiniMapDragDelta.y + delta.y,
    }
    scheduleMiniMapDragFrame()
  }

  function onMiniMapDragEnd() {
    if (!isMiniMapDragActive) return

    flushPendingMiniMapDragDelta()
    isMiniMapDragActive = false
    pendingMiniMapDragEnd = true

    finishMiniMapDragEndIfIdle()
  }

  function scheduleMiniMapDragFrame() {
    if (pendingMiniMapDragFrameId !== null) return
    pendingMiniMapDragFrameId = window.requestAnimationFrame(() => {
      pendingMiniMapDragFrameId = null
      flushPendingMiniMapDragDelta()
    })
  }

  function clearPendingMiniMapDragFrame() {
    if (pendingMiniMapDragFrameId === null) return
    window.cancelAnimationFrame(pendingMiniMapDragFrameId)
    pendingMiniMapDragFrameId = null
  }

  function flushPendingMiniMapDragDelta() {
    if (!pendingMiniMapDragDelta.x && !pendingMiniMapDragDelta.y) return

    const baseViewport = miniMapDragTargetViewport ?? options.getViewport()
    const zoom = baseViewport.zoom
    if (!Number.isFinite(zoom) || zoom <= 0) {
      pendingMiniMapDragDelta = { x: 0, y: 0 }
      return
    }

    const delta = pendingMiniMapDragDelta
    pendingMiniMapDragDelta = { x: 0, y: 0 }

    didMiniMapDragMove = true
    miniMapDragTargetViewport = {
      x: baseViewport.x - delta.x * zoom,
      y: baseViewport.y - delta.y * zoom,
      zoom,
    }
    beginViewportFrameProbe({
      x: miniMapDragTargetViewport.x,
      y: miniMapDragTargetViewport.y,
    })
    requestMiniMapViewportUpdate()
  }

  function requestMiniMapViewportUpdate() {
    if (isMiniMapViewportUpdateInFlight) return

    const targetViewport = miniMapDragTargetViewport
    if (!targetViewport) {
      finishMiniMapDragEndIfIdle()
      return
    }

    const targetViewportKey = viewportKey(targetViewport)
    if (targetViewportKey === lastRequestedMiniMapViewportKey) {
      finishMiniMapDragEndIfIdle()
      return
    }

    const sessionId = miniMapDragSessionId
    lastRequestedMiniMapViewportKey = targetViewportKey
    isMiniMapViewportUpdateInFlight = true
    programmaticViewportCommitDepth += 1

    void Promise.resolve()
      .then(() => options.setViewport(targetViewport, { duration: 0 }))
      .then(
        () => {
          if (sessionId === miniMapDragSessionId) {
            lastSuccessfulMiniMapViewportKey = targetViewportKey
          }
        },
        (error) => {
          console.warn('[canvas] minimap viewport update failed', error)
        },
      )
      .finally(() => {
        programmaticViewportCommitDepth -= 1
        isMiniMapViewportUpdateInFlight = false
        if (sessionId !== miniMapDragSessionId) {
          if (didMiniMapDragMove) requestMiniMapViewportUpdate()
          return
        }

        const latestTarget = miniMapDragTargetViewport
        if (latestTarget && viewportKey(latestTarget) !== lastRequestedMiniMapViewportKey) {
          requestMiniMapViewportUpdate()
          return
        }

        finishMiniMapDragEndIfIdle()
      })
  }

  function finishMiniMapDragEndIfIdle() {
    if (!pendingMiniMapDragEnd || isMiniMapViewportUpdateInFlight) return

    if (!didMiniMapDragMove) {
      pendingMiniMapDragEnd = false
      onFlowMoveEnd({ flowTransform: options.getViewport() })
      miniMapDragTargetViewport = null
      return
    }

    const latestTarget = miniMapDragTargetViewport
    if (latestTarget && viewportKey(latestTarget) !== lastRequestedMiniMapViewportKey) {
      requestMiniMapViewportUpdate()
      return
    }

    pendingMiniMapDragEnd = false
    if (
      didMiniMapDragMove &&
      latestTarget &&
      lastSuccessfulMiniMapViewportKey === viewportKey(latestTarget)
    ) {
      viewportPersistence.persistNow()
    }
    onFlowMoveEnd({ flowTransform: options.getViewport() })
    miniMapDragTargetViewport = null
  }

  function onViewportChangeEnd(viewport = options.viewport.value) {
    if (programmaticViewportCommitDepth > 0 || isMiniMapDragActive) {
      return
    }
    viewportPersistence.persistViewportEnd(viewport)
  }

  function observeViewportMove(transform: CanvasFlowMovePayload['flowTransform']) {
    if (!transform) return
    beginViewportFrameProbe(transform)
  }

  function beginViewportFrameProbe(transform: CanvasFlowMovePayload['flowTransform']) {
    if (activeViewportFrameProbe) return
    const nextZoom =
      typeof transform?.zoom === 'number' && Number.isFinite(transform.zoom)
        ? transform.zoom
        : options.viewport.value.zoom
    const baseZoom = viewportMoveStartZoom ?? options.viewport.value.zoom
    const scene = Math.abs(nextZoom - baseZoom) > 0.0001 ? 'zoom' : 'pan'
    if (scene === 'pan' && !isMiniMapDragActive) {
      options.startCanvasInteraction('viewport-pan')
    }
    activeViewportFrameProbe = beginCanvasFrameProbe(scene, () => getCanvasFrameProbeMetadata())
  }

  function endViewportFrameProbe(reason: string) {
    activeViewportFrameProbe?.end(() => ({
      ...getCanvasFrameProbeMetadata(),
      reason,
    }))
    activeViewportFrameProbe = null
    viewportMoveStartZoom = null
    options.stopCanvasInteraction('viewport-pan')
  }

  async function loadCanvasBoardAndProbe() {
    if (!isCanvasFrameProbeEnabled()) {
      await options.loadInitialOrRouteBoard()
      await restoreViewportForCurrentBoard()
      return
    }

    const frameProbe = beginCanvasFrameProbe('load-hydrate', () => getCanvasFrameProbeMetadata())
    try {
      await options.loadInitialOrRouteBoard()
      await restoreViewportForCurrentBoard()
      frameProbe.mark('graph-committed', () => getCanvasFrameProbeMetadata())
      await waitForNextRenderFrame()
      frameProbe.end(() => getCanvasFrameProbeMetadata())
    } catch (error) {
      frameProbe.cleanup()
      throw error
    }
  }

  function onFlowMoveStart() {
    viewportMoveStartZoom = options.viewport.value.zoom
    options.closeBoardHeaderEditing()
    options.startCanvasInteraction('flow-viewport')
  }

  function onFlowMove(payload: CanvasFlowMovePayload) {
    observeViewportMove(payload.flowTransform)
  }

  function onFlowMoveEnd(payload: CanvasFlowMovePayload) {
    try {
      const transform = payload.flowTransform
      if (transform) beginViewportFrameProbe(transform)
    } finally {
      endViewportFrameProbe('move-end')
      options.stopCanvasInteraction('flow-viewport')
    }
  }

  function onNodeDragStart() {
    options.startCanvasInteraction('node-drag')
    activeNodeDragFrameProbe?.cleanup()
    activeNodeDragFrameProbe = beginCanvasFrameProbe('node-drag', () =>
      getCanvasFrameProbeMetadata(),
    )
  }

  function onNodeDragStop() {
    activeNodeDragFrameProbe?.end(() => ({
      ...getCanvasFrameProbeMetadata(),
      reason: 'node-drag-stop',
    }))
    activeNodeDragFrameProbe = null
    options.stopCanvasInteraction('node-drag')
  }

  function cleanupFrameProbes() {
    clearPendingMiniMapDragFrame()
    pendingMiniMapDragEnd = false
    isMiniMapDragActive = false
    isMiniMapViewportUpdateInFlight = false
    viewportPersistence.dispose()
    activeViewportFrameProbe?.cleanup()
    activeViewportFrameProbe = null
    viewportMoveStartZoom = null
    options.stopCanvasInteraction('viewport-pan')
    activeNodeDragFrameProbe?.cleanup()
    activeNodeDragFrameProbe = null
  }

  if (getCurrentScope()) {
    onScopeDispose(cleanupFrameProbes)
  }

  return {
    zoomLevel,
    onZoomIn,
    onZoomOut,
    onPanViewport,
    onMiniMapCenterRequested,
    onMiniMapDragStart,
    onMiniMapDragMove,
    onMiniMapDragEnd,
    onViewportChangeEnd,
    onFlowMoveStart,
    onFlowMove,
    onFlowMoveEnd,
    onNodeDragStart,
    onNodeDragStop,
    flushViewportPersistence: viewportPersistence.flush,
    loadCanvasBoardAndProbe,
  }
}
