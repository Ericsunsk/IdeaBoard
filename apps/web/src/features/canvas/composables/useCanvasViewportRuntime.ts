import { getCurrentScope, onScopeDispose, watch } from 'vue'
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
  cull: 'flush' | 'schedule'
  persist?: boolean
}

export type CanvasViewportRuntimeInteraction = 'flow-viewport' | 'node-drag'

export type CanvasViewportSize = {
  width: number
  height: number
}

type ReadonlyValueRef<T> = {
  readonly value: T
}

type WritableValueRef<T> = {
  value: T
}

export type UseCanvasViewportRuntimeOptions = {
  canvasWrapRef: ReadonlyValueRef<HTMLElement | null>
  canvasViewportSize: WritableValueRef<CanvasViewportSize>
  nodes: ReadonlyValueRef<readonly unknown[]>
  edges: ReadonlyValueRef<readonly unknown[]>
  fullNodes: ReadonlyValueRef<readonly unknown[]>
  fullEdges: ReadonlyValueRef<readonly unknown[]>
  currentBoardId: ReadonlyValueRef<string | null>
  getViewport: () => CanvasFlowViewport
  setViewport: (viewport: CanvasFlowViewport) => Promise<unknown> | unknown
  zoomIn: () => Promise<unknown> | unknown
  zoomOut: () => Promise<unknown> | unknown
  applyFlowTransform: (viewport: CanvasFlowViewport) => boolean
  markBoardViewportChanged: () => void
  commitPendingBoardViewportChange: () => void
  scheduleMountedGraphRefresh: (viewport?: CanvasFlowViewport) => void
  flushMountedGraphRefresh: (viewport?: CanvasFlowViewport) => void
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
      mountedNodes: options.nodes.value.length,
      mountedEdges: options.edges.value.length,
      fullNodes: options.fullNodes.value.length,
      fullEdges: options.fullEdges.value.length,
      viewportX: Math.round(viewport.x),
      viewportY: Math.round(viewport.y),
      zoomPct: Math.round(viewport.zoom * 100),
    }
  }

  function resolveFlowViewportFromTransform(
    transform: CanvasFlowMovePayload['flowTransform'],
  ): CanvasFlowViewport {
    const viewport = options.getViewport()
    return {
      x: typeof transform?.x === 'number' ? transform.x : viewport.x,
      y: typeof transform?.y === 'number' ? transform.y : viewport.y,
      zoom: typeof transform?.zoom === 'number' ? transform.zoom : viewport.zoom,
    }
  }

  function syncCanvasViewport(
    transform: CanvasFlowMovePayload['flowTransform'],
    syncOptions: CanvasViewportSyncOptions,
  ) {
    const viewport = resolveFlowViewportFromTransform(transform)
    const didChange = options.applyFlowTransform(viewport)
    if (didChange && syncOptions.persist) options.markBoardViewportChanged()
    if (syncOptions.cull === 'flush') {
      options.flushMountedGraphRefresh(viewport)
    } else {
      options.scheduleMountedGraphRefresh(viewport)
    }
    return didChange
  }

  function onZoomIn() {
    const result = options.zoomIn()
    void Promise.resolve(result).then(() => {
      syncCanvasViewport(options.getViewport(), { cull: 'flush', persist: true })
      options.commitPendingBoardViewportChange()
    })
  }

  function onZoomOut() {
    const result = options.zoomOut()
    void Promise.resolve(result).then(() => {
      syncCanvasViewport(options.getViewport(), { cull: 'flush', persist: true })
      options.commitPendingBoardViewportChange()
    })
  }

  function onPanViewport(delta: { x: number; y: number }) {
    const viewport = options.getViewport()
    const nextViewport = {
      x: viewport.x + delta.x,
      y: viewport.y + delta.y,
      zoom: viewport.zoom,
    }
    void options.setViewport(nextViewport)
    syncCanvasViewport(nextViewport, { cull: 'flush', persist: true })
    options.commitPendingBoardViewportChange()
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
    const transform = payload.flowTransform
    if (!transform) return
    beginViewportFrameProbe(transform)
    syncCanvasViewport(transform, { cull: 'schedule', persist: true })
  }

  function onFlowMoveEnd(payload: CanvasFlowMovePayload) {
    try {
      const transform = payload.flowTransform
      if (transform) {
        beginViewportFrameProbe(transform)
        syncCanvasViewport(transform, { cull: 'flush', persist: true })
      } else {
        syncCanvasViewport(options.getViewport(), { cull: 'flush' })
      }
      options.commitPendingBoardViewportChange()
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
      options.flushMountedGraphRefresh()
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

  watch(
    () => options.canvasWrapRef.value,
    (element, _previous, onCleanup) => {
      if (!element || typeof ResizeObserver === 'undefined') return
      const updateSize = () => {
        const width = element.clientWidth
        const height = element.clientHeight
        if (
          options.canvasViewportSize.value.width === width &&
          options.canvasViewportSize.value.height === height
        ) {
          return
        }
        options.canvasViewportSize.value = { width, height }
        options.flushMountedGraphRefresh()
      }
      updateSize()
      const observer = new ResizeObserver(updateSize)
      observer.observe(element)
      onCleanup(() => observer.disconnect())
    },
    { immediate: true },
  )

  if (getCurrentScope()) {
    onScopeDispose(cleanupFrameProbes)
  }

  return {
    syncCanvasViewport,
    onZoomIn,
    onZoomOut,
    onPanViewport,
    onFlowMoveStart,
    onFlowMove,
    onFlowMoveEnd,
    onNodeDragStart,
    onNodeDragStop,
    loadCanvasBoardAndProbe,
  }
}
