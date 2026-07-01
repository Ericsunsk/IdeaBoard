import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, ref, type Ref } from 'vue'
import { useCanvasViewportRuntime, type CanvasFlowViewport } from './useCanvasViewportRuntime'

const frameProbeMocks = vi.hoisted(() => ({
  isEnabled: vi.fn(() => false),
  begin: vi.fn(),
  waitForNextRenderFrame: vi.fn(async () => undefined),
}))

vi.mock('../utils/canvasFrameProbe', () => ({
  isCanvasFrameProbeEnabled: frameProbeMocks.isEnabled,
  beginCanvasFrameProbe: frameProbeMocks.begin,
}))

vi.mock('../utils/animationFrame', () => ({
  waitForNextRenderFrame: frameProbeMocks.waitForNextRenderFrame,
}))

function createViewportRuntimeHarness() {
  const viewport = ref<CanvasFlowViewport>({ x: 0, y: 0, zoom: 1 })
  const startCanvasInteraction = vi.fn()
  const stopCanvasInteraction = vi.fn()
  const markBoardViewportChanged = vi.fn()
  const commitPendingBoardViewportChange = vi.fn()
  const panBy = vi.fn((delta: { x: number; y: number }) => {
    if (!delta.x && !delta.y) return false
    viewport.value = {
      x: viewport.value.x + delta.x,
      y: viewport.value.y + delta.y,
      zoom: viewport.value.zoom,
    }
    return true
  })
  const setCenter = vi.fn((x: number, y: number, _options?: { duration?: number; zoom?: number }) => {
    viewport.value = { x: -x, y: -y, zoom: viewport.value.zoom }
  })
  const loadInitialOrRouteBoard = vi.fn(async () => undefined)

  const scope = effectScope()
  const runtime = scope.run(() =>
    useCanvasViewportRuntime({
      nodes: ref([]),
      edges: ref([]),
      fullNodes: ref([]),
      fullEdges: ref([]),
      currentBoardId: ref('board-a') as Ref<string | null>,
      viewport,
      getViewport: () => viewport.value,
      setCenter,
      zoomIn: vi.fn(() => {
        viewport.value = { ...viewport.value, zoom: viewport.value.zoom + 0.25 }
      }),
      zoomOut: vi.fn(() => {
        viewport.value = { ...viewport.value, zoom: viewport.value.zoom - 0.25 }
      }),
      panBy,
      markBoardViewportChanged,
      commitPendingBoardViewportChange,
      loadInitialOrRouteBoard,
      closeBoardHeaderEditing: vi.fn(),
      startCanvasInteraction,
      stopCanvasInteraction,
      startNodeDragInteraction: vi.fn(),
      stopNodeDragInteraction: vi.fn(),
      refreshFlowHandleVisibility: vi.fn(),
    }),
  )
  if (!runtime) throw new Error('runtime was not created')

  return {
    runtime,
    scope,
    viewport,
    panBy,
    setCenter,
    markBoardViewportChanged,
    commitPendingBoardViewportChange,
    loadInitialOrRouteBoard,
    startCanvasInteraction,
    stopCanvasInteraction,
  }
}

describe('useCanvasViewportRuntime', () => {
  beforeEach(() => {
    frameProbeMocks.isEnabled.mockReturnValue(false)
    frameProbeMocks.begin.mockReset()
    frameProbeMocks.waitForNextRenderFrame.mockClear()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('observes flow viewport movement without committing persistence during move frames', () => {
    const harness = createViewportRuntimeHarness()

    harness.runtime.onFlowMove({ flowTransform: { x: 24, y: 32, zoom: 1.5 } })

    expect(harness.markBoardViewportChanged).not.toHaveBeenCalled()
    harness.scope.stop()
  })

  it('commits viewport persistence on viewport change end', () => {
    const harness = createViewportRuntimeHarness()

    harness.runtime.onFlowMoveStart()
    harness.runtime.onViewportChangeEnd()
    harness.runtime.onFlowMoveEnd({ flowTransform: { x: 48, y: 64, zoom: 2 } })

    expect(harness.startCanvasInteraction).toHaveBeenCalledWith('flow-viewport')
    expect(harness.markBoardViewportChanged).toHaveBeenCalledOnce()
    expect(harness.commitPendingBoardViewportChange).toHaveBeenCalledOnce()
    expect(harness.stopCanvasInteraction).toHaveBeenCalledWith('flow-viewport')
    harness.scope.stop()
  })

  it('syncs the viewport after zooming', async () => {
    const harness = createViewportRuntimeHarness()

    harness.runtime.onZoomIn()
    await Promise.resolve()
    harness.runtime.onViewportChangeEnd(harness.viewport.value)

    expect(harness.markBoardViewportChanged).toHaveBeenCalledOnce()
    expect(harness.commitPendingBoardViewportChange).toHaveBeenCalledOnce()
    harness.scope.stop()
  })

  it('centers the viewport and commits persistence when the minimap requests centering', async () => {
    const harness = createViewportRuntimeHarness()

    harness.runtime.onMiniMapCenterRequested({ position: { x: 240, y: 180 } })
    await Promise.resolve()

    expect(harness.setCenter).toHaveBeenCalledWith(240, 180, {
      duration: 0,
      zoom: 1,
    })
    expect(harness.markBoardViewportChanged).toHaveBeenCalledOnce()
    expect(harness.commitPendingBoardViewportChange).toHaveBeenCalledOnce()
    harness.scope.stop()
  })

  it('uses panBy for keyboard panning and commits only when the viewport moves', () => {
    const harness = createViewportRuntimeHarness()

    harness.runtime.onPanViewport({ x: 16, y: -8 })
    harness.runtime.onViewportChangeEnd(harness.viewport.value)

    expect(harness.panBy).toHaveBeenCalledWith({ x: 16, y: -8 })
    expect(harness.viewport.value).toEqual({ x: 16, y: -8, zoom: 1 })
    expect(harness.markBoardViewportChanged).toHaveBeenCalledOnce()
    expect(harness.commitPendingBoardViewportChange).toHaveBeenCalledOnce()
    harness.scope.stop()
  })

  it('does not commit dirty persistence when panBy is constrained or uninitialized', () => {
    const harness = createViewportRuntimeHarness()
    harness.panBy.mockReturnValueOnce(false)

    harness.runtime.onPanViewport({ x: 16, y: -8 })

    expect(harness.markBoardViewportChanged).not.toHaveBeenCalled()
    expect(harness.commitPendingBoardViewportChange).not.toHaveBeenCalled()
    harness.scope.stop()
  })

  it('commits a later user viewport change after a constrained keyboard pan', () => {
    const harness = createViewportRuntimeHarness()
    harness.panBy.mockReturnValueOnce(false)

    harness.runtime.onPanViewport({ x: 16, y: -8 })
    harness.viewport.value = { x: 24, y: 32, zoom: 1.5 }
    harness.runtime.onViewportChangeEnd(harness.viewport.value)

    expect(harness.markBoardViewportChanged).toHaveBeenCalledOnce()
    expect(harness.commitPendingBoardViewportChange).toHaveBeenCalledOnce()
    harness.scope.stop()
  })

  it('stops interaction after node drag', () => {
    const harness = createViewportRuntimeHarness()

    harness.runtime.onNodeDragStart()
    harness.runtime.onNodeDragStop()

    expect(harness.startCanvasInteraction).toHaveBeenCalledWith('node-drag')
    expect(harness.stopCanvasInteraction).toHaveBeenCalledWith('node-drag')
    harness.scope.stop()
  })

  it('loads without waiting for an extra frame when frame probes are disabled', async () => {
    const harness = createViewportRuntimeHarness()

    await harness.runtime.loadCanvasBoardAndProbe()

    expect(harness.loadInitialOrRouteBoard).toHaveBeenCalledOnce()
    expect(frameProbeMocks.begin).not.toHaveBeenCalled()
    expect(frameProbeMocks.waitForNextRenderFrame).not.toHaveBeenCalled()
    harness.scope.stop()
  })
})
