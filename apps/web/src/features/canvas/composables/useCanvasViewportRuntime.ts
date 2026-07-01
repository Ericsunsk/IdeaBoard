import { computed, getCurrentScope, onScopeDispose, type Ref } from 'vue'
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
  viewport: Readonly<Ref<CanvasFlowViewport>>
  getViewport: () => CanvasFlowViewport
  setCenter: (
    x: number,
    y: number,
    options?: { duration?: number; zoom?: number },
  ) => Promise<unknown> | unknown
  zoomIn: () => Promise<unknown> | unknown
  zoomOut: () => Promise<unknown> | unknown
  panBy: (delta: { x: number; y: number }) => boolean
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

function viewportKey(viewport: CanvasFlowViewport) {
  return `${viewport.x}:${viewport.y}:${viewport.zoom}`
}

export function useCanvasViewportRuntime(options: UseCanvasViewportRuntimeOptions) {
  let activeViewportFrameProbe: CanvasFrameProbeSession | null = null
  let activeNodeDragFrameProbe: CanvasFrameProbeSession | null = null
  let lastCommittedViewportKey: string | null = null
  let programmaticViewportCommitDepth = 0

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
      fullNodes: options.fullNodes.value.length,
      fullEdges: options.fullEdges.value.length,
      viewportX: Math.round(viewport.x),
      viewportY: Math.round(viewport.y),
      zoomPct: Math.round(viewport.zoom * 100),
    }
  }

  function commitViewportPersistence() {
    lastCommittedViewportKey = viewportKey(options.viewport.value)
    options.markBoardViewportChanged()
    options.commitPendingBoardViewportChange()
  }

  async function runAndPersistViewportCommand(
    run: () => Promise<unknown> | unknown,
  ) {
    programmaticViewportCommitDepth += 1
    try {
      const result = await run()
      if (result === false) return

      commitViewportPersistence()
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
      commitViewportPersistence()
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

  function onViewportChangeEnd(viewport = options.viewport.value) {
    const key = viewportKey(viewport)
    if (programmaticViewportCommitDepth > 0) {
      lastCommittedViewportKey = key
      return
    }
    if (key === lastCommittedViewportKey) return
    commitViewportPersistence()
  }

  function observeViewportMove(transform: CanvasFlowMovePayload['flowTransform']) {
    if (!transform) return
    beginViewportFrameProbe(transform)
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
    zoomLevel,
    onZoomIn,
    onZoomOut,
    onPanViewport,
    onMiniMapCenterRequested,
    onViewportChangeEnd,
    onFlowMoveStart,
    onFlowMove,
    onFlowMoveEnd,
    onNodeDragStart,
    onNodeDragStop,
    loadCanvasBoardAndProbe,
  }
}
