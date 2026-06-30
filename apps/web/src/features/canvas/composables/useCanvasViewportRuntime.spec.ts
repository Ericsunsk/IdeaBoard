import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, nextTick, ref, shallowRef, type Ref } from 'vue'
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

class TestResizeObserver {
  static instances: TestResizeObserver[] = []
  readonly callback: ResizeObserverCallback
  readonly observe = vi.fn()
  readonly disconnect = vi.fn()

  constructor(callback: ResizeObserverCallback) {
    this.callback = callback
    TestResizeObserver.instances.push(this)
  }
}

function installResizeObserverMock() {
  TestResizeObserver.instances = []
  vi.stubGlobal('ResizeObserver', TestResizeObserver)
}

function createViewportRuntimeHarness() {
  const canvasWrapRef = shallowRef<HTMLElement | null>(null)
  const canvasViewportSize = ref({ width: 0, height: 0 })
  const viewport = ref<CanvasFlowViewport>({ x: 0, y: 0, zoom: 1 })
  const startCanvasInteraction = vi.fn()
  const stopCanvasInteraction = vi.fn()
  const markBoardViewportChanged = vi.fn()
  const commitPendingBoardViewportChange = vi.fn()
  const applyFlowTransform = vi.fn((nextViewport: CanvasFlowViewport) => {
    const didChange =
      viewport.value.x !== nextViewport.x ||
      viewport.value.y !== nextViewport.y ||
      viewport.value.zoom !== nextViewport.zoom
    viewport.value = nextViewport
    return didChange
  })
  const loadInitialOrRouteBoard = vi.fn(async () => undefined)

  const scope = effectScope()
  const runtime = scope.run(() =>
    useCanvasViewportRuntime({
      canvasWrapRef,
      canvasViewportSize,
      nodes: ref([]),
      edges: ref([]),
      fullNodes: ref([]),
      fullEdges: ref([]),
      currentBoardId: ref('board-a') as Ref<string | null>,
      getViewport: () => viewport.value,
      setViewport: (nextViewport) => {
        viewport.value = nextViewport
      },
      zoomIn: vi.fn(() => {
        viewport.value = { ...viewport.value, zoom: viewport.value.zoom + 0.25 }
      }),
      zoomOut: vi.fn(() => {
        viewport.value = { ...viewport.value, zoom: viewport.value.zoom - 0.25 }
      }),
      applyFlowTransform,
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
    canvasWrapRef,
    canvasViewportSize,
    viewport,
    applyFlowTransform,
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

  it('syncs the flow viewport while the flow viewport is moving', () => {
    const harness = createViewportRuntimeHarness()

    harness.runtime.onFlowMove({ flowTransform: { x: 24, y: 32, zoom: 1.5 } })

    expect(harness.applyFlowTransform).toHaveBeenCalledWith({ x: 24, y: 32, zoom: 1.5 })
    harness.scope.stop()
  })

  it('commits viewport persistence on move end', () => {
    const harness = createViewportRuntimeHarness()

    harness.runtime.onFlowMoveStart()
    harness.runtime.onFlowMoveEnd({ flowTransform: { x: 48, y: 64, zoom: 2 } })

    expect(harness.startCanvasInteraction).toHaveBeenCalledWith('flow-viewport')
    expect(harness.applyFlowTransform).toHaveBeenCalledWith({ x: 48, y: 64, zoom: 2 })
    expect(harness.stopCanvasInteraction).toHaveBeenCalledWith('flow-viewport')
    harness.scope.stop()
  })

  it('syncs and commits viewport changes from minimap dragging', () => {
    const harness = createViewportRuntimeHarness()

    harness.runtime.onMiniMapViewportDragStart()
    harness.runtime.onMiniMapViewportDrag({ x: -120, y: -80, zoom: 1 })
    harness.runtime.onMiniMapViewportDragEnd()

    expect(harness.startCanvasInteraction).toHaveBeenCalledWith('flow-viewport')
    expect(harness.applyFlowTransform).toHaveBeenCalledWith({ x: -120, y: -80, zoom: 1 })
    expect(harness.stopCanvasInteraction).toHaveBeenCalledWith('flow-viewport')
    harness.scope.stop()
  })

  it('syncs the viewport after zooming', async () => {
    const harness = createViewportRuntimeHarness()

    harness.runtime.onZoomIn()
    await Promise.resolve()

    expect(harness.applyFlowTransform).toHaveBeenCalledWith({ x: 0, y: 0, zoom: 1.25 })
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

  it('updates viewport size when the canvas wrapper resizes', async () => {
    installResizeObserverMock()
    const harness = createViewportRuntimeHarness()
    const element = {
      clientWidth: 1024,
      clientHeight: 768,
    } as HTMLElement

    harness.canvasWrapRef.value = element
    await nextTick()

    expect(harness.canvasViewportSize.value).toEqual({ width: 1024, height: 768 })
    expect(TestResizeObserver.instances.at(-1)?.observe).toHaveBeenCalledWith(element)
    harness.scope.stop()
    expect(TestResizeObserver.instances.at(-1)?.disconnect).toHaveBeenCalled()
  })
})
