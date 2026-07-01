import { getCurrentScope, onScopeDispose } from 'vue'
import {
  beginCanvasFrameProbe,
  isCanvasFrameProbeEnabled,
  type CanvasFrameProbeSession,
} from '../utils/canvasFrameProbe'
import { waitForNextRenderFrame } from '../utils/animationFrame'

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

export type CanvasViewportSyncOptions = {
  persist?: boolean
  forcePersist?: boolean
}

export type CanvasViewportRuntimeInteraction = 'flow-viewport' | 'node-drag'

export type CanvasMiniMapCenterRequestPayload = {
  position: {
    x: number
    y: number
  }
}

type ReadonlyValueRef<T> = {
  readonly value: T
}

export type UseCanvasViewportRuntimeOptions = {
  nodes: ReadonlyValueRef<readonly unknown[]>
  edges: ReadonlyValueRef<readonly unknown[]>
  fullNodes: ReadonlyValueRef<readonly unknown[]>
  fullEdges: ReadonlyValueRef<readonly unknown[]>
  currentBoardId: ReadonlyValueRef<string | null>
  getViewport: () => CanvasFlowViewport
  setViewport: (viewport: CanvasFlowViewport) => Promise<unknown> | unknown
  setCenter: (
    x: number,
    y: number,
    options?: { duration?: number; zoom?: number },
  ) => Promise<unknown> | unknown
  zoomIn: () => Promise<unknown> | unknown
  zoomOut: () => Promise<unknown> | unknown
  applyFlowTransform: (viewport: CanvasFlowViewport) => boolean
  markBoardViewportChanged: () => void
  commitPendingBoardViewportChange: () => void
  loadInitialOrRouteBoard: () => Promise<void>
  closeBoardHeaderEditing: () => void
  startCanvasInteraction: (key: CanvasViewportRuntimeInteraction) => void
  stopCanvasInteraction: (key: CanvasViewportRuntimeInteraction) => void
  startNodeDragInteraction: () => void
  stopNodeDragInteraction: () => void
  refreshFlowHandleVisibility: () => void
}

export function useCanvasViewportRuntime(options: UseCanvasViewportRuntimeOptions) {
  let activeViewportFrameProbe: CanvasFrameProbeSession | null = null
  let activeNodeDragFrameProbe: CanvasFrameProbeSession | null = null

  function getCanvasFrameProbeMetadata() {
    const viewport = options.getViewport()
    return {
      boardId: options.currentBoardId.value,
      nodes: options.nodes.value.length,
      edges: options.edges.value.length,
      fullNodes: options.fullNodes.value.length,
      fullEdges: options.fullEdges.value.length,
      viewportX: Math.round(viewport.x),
      viewportY: Math.round(viewport.y),
      zoomPct: Math.round(viewport.zoom * 100),
    }
  }

  function resolveFlowViewportFromTransform(
    transform: CanvasFlowMovePayload['flowTransform'] | CanvasFlowViewport | undefined,
  ): CanvasFlowViewport {
    const viewport = options.getViewport()
    return {
      x: typeof transform?.x === 'number' ? transform.x : viewport.x,
      y: typeof transform?.y === 'number' ? transform.y : viewport.y,
      zoom: typeof transform?.zoom === 'number' ? transform.zoom : viewport.zoom,
    }
  }

  function syncCanvasViewport(
    transform: CanvasFlowMovePayload['flowTransform'] | CanvasFlowViewport | undefined,
    syncOptions: CanvasViewportSyncOptions = {},
  ) {
    const viewport = resolveFlowViewportFromTransform(transform)
    const didChange = options.applyFlowTransform(viewport)

    if (syncOptions.persist && (didChange || syncOptions.forcePersist)) {
      options.markBoardViewportChanged()
    }

    return didChange
  }

  async function runAndPersistViewportCommand(
    run: () => Promise<unknown> | unknown,
    resolveViewport: () => CanvasFlowViewport = options.getViewport,
  ) {
    const result = await run()
    if (result === false) return

    syncCanvasViewport(resolveViewport(), {
      persist: true,
      forcePersist: true,
    })
    options.commitPendingBoardViewportChange()
  }

  function onZoomIn() {
    void runAndPersistViewportCommand(options.zoomIn)
  }

  function onZoomOut() {
    void runAndPersistViewportCommand(options.zoomOut)
  }

  function onPanViewport(delta: { x: number; y: number }) {
    const viewport = options.getViewport()
    const nextViewport = {
      x: viewport.x + delta.x,
      y: viewport.y + delta.y,
      zoom: viewport.zoom,
    }

    void runAndPersistViewportCommand(() => options.setViewport(nextViewport), () => {
      return nextViewport
    })
  }

  function onMiniMapCenterRequested(payload: CanvasMiniMapCenterRequestPayload) {
    const zoom = options.getViewport().zoom
    void runAndPersistViewportCommand(() =>
      options.setCenter(payload.position.x, payload.position.y, { duration: 0, zoom }),
    )
  }

  function syncViewportAndCommit(transform: CanvasFlowMovePayload['flowTransform']) {
    if (transform) {
      syncCanvasViewport(transform, { persist: true })
    } else {
      syncCanvasViewport(options.getViewport())
    }
    options.commitPendingBoardViewportChange()
  }

  function syncViewportAndProbe(transform: CanvasFlowMovePayload['flowTransform']) {
    if (!transform) return
    beginViewportFrameProbe(transform)
    syncCanvasViewport(transform, { persist: true })
  }

  function beginViewportFrameProbe(transform: CanvasFlowMovePayload['flowTransform']) {
    if (activeViewportFrameProbe) return
    activeViewportFrameProbe = beginCanvasFrameProbe(
      typeof transform?.zoom === 'number' ? 'zoom' : 'pan',
      () => getCanvasFrameProbeMetadata(),
    )
  }

  function endViewportFrameProbe(reason: string) {
    activeViewportFrameProbe?.end(() => ({
      ...getCanvasFrameProbeMetadata(),
      reason,
    }))
    activeViewportFrameProbe = null
  }

  async function loadCanvasBoardAndProbe() {
    if (!isCanvasFrameProbeEnabled()) {
      await options.loadInitialOrRouteBoard()
      return
    }

    const frameProbe = beginCanvasFrameProbe('load-hydrate', () =>
      getCanvasFrameProbeMetadata(),
    )
    try {
      await options.loadInitialOrRouteBoard()
      frameProbe.mark('graph-committed', () => getCanvasFrameProbeMetadata())
      await waitForNextRenderFrame()
      frameProbe.end(() => getCanvasFrameProbeMetadata())
    } catch (error) {
      frameProbe.cleanup()
      throw error
    }
  }

  function onFlowMoveStart() {
    options.closeBoardHeaderEditing()
    options.startCanvasInteraction('flow-viewport')
  }

  function onFlowMove(payload: CanvasFlowMovePayload) {
    syncViewportAndProbe(payload.flowTransform)
  }

  function onFlowMoveEnd(payload: CanvasFlowMovePayload) {
    try {
      const transform = payload.flowTransform
      if (transform) beginViewportFrameProbe(transform)
      syncViewportAndCommit(transform)
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
    try {
      options.startNodeDragInteraction()
    } catch (error) {
      activeNodeDragFrameProbe?.cleanup()
      activeNodeDragFrameProbe = null
      options.stopCanvasInteraction('node-drag')
      throw error
    }
  }

  function onNodeDragStop() {
    try {
      options.stopNodeDragInteraction()
    } finally {
      activeNodeDragFrameProbe?.end(() => ({
        ...getCanvasFrameProbeMetadata(),
        reason: 'node-drag-stop',
      }))
      activeNodeDragFrameProbe = null
      options.stopCanvasInteraction('node-drag')
      options.refreshFlowHandleVisibility()
    }
  }

  function cleanupFrameProbes() {
    activeViewportFrameProbe?.cleanup()
    activeViewportFrameProbe = null
    activeNodeDragFrameProbe?.cleanup()
    activeNodeDragFrameProbe = null
  }

  if (getCurrentScope()) {
    onScopeDispose(cleanupFrameProbes)
  }

  return {
    syncCanvasViewport,
    onZoomIn,
    onZoomOut,
    onPanViewport,
    onMiniMapCenterRequested,
    onFlowMoveStart,
    onFlowMove,
    onFlowMoveEnd,
    onNodeDragStart,
    onNodeDragStop,
    loadCanvasBoardAndProbe,
  }
}
