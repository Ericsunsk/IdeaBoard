import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, ref } from 'vue'
import { useCanvasViewportRuntime, type CanvasFlowViewport } from './useCanvasViewportRuntime'
import type { CanvasViewportPersistence } from '../utils/canvasViewportPersistence'

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

async function flushPromises(times = 4) {
  for (let i = 0; i < times; i += 1) {
    await Promise.resolve()
  }
}

function readStoredViewport(boardId = 'board-a', userUid = 'user-uid') {
  const raw = localStorage.getItem(
    `ideaboard:canvas:view:${encodeURIComponent(userUid)}:${encodeURIComponent(boardId)}`,
  )
  return raw ? (JSON.parse(raw) as { x: number; y: number; zoom: number; updatedAt: number }) : null
}

function createViewportRuntimeHarness(
  options: { viewportPersistence?: CanvasViewportPersistence } = {},
) {
  const currentBoardId = ref<string | null>('board-a')
  const userUid = ref('user-uid')
  const viewport = ref<CanvasFlowViewport>({ x: 0, y: 0, zoom: 1 })
  const startCanvasInteraction = vi.fn()
  const stopCanvasInteraction = vi.fn()
  const panBy = vi.fn((delta: { x: number; y: number }) => {
    if (!delta.x && !delta.y) return false
    viewport.value = {
      x: viewport.value.x + delta.x,
      y: viewport.value.y + delta.y,
      zoom: viewport.value.zoom,
    }
    return true
  })
  const setCenter = vi.fn(
    (x: number, y: number, _options?: { duration?: number; zoom?: number }) => {
      viewport.value = { x: -x, y: -y, zoom: viewport.value.zoom }
    },
  )
  const setViewport = vi.fn((nextViewport: CanvasFlowViewport) => {
    viewport.value = nextViewport
  })
  const loadInitialOrRouteBoard = vi.fn(async () => undefined)

  const scope = effectScope()
  const runtime = scope.run(() =>
    useCanvasViewportRuntime({
      nodes: ref([]),
      edges: ref([]),
      fullNodes: ref([]),
      fullEdges: ref([]),
      currentBoardId,
      getUserUid: () => userUid.value,
      viewport,
      getViewport: () => viewport.value,
      setViewport,
      setCenter,
      zoomIn: vi.fn(() => {
        viewport.value = { ...viewport.value, zoom: viewport.value.zoom + 0.25 }
      }),
      zoomOut: vi.fn(() => {
        viewport.value = { ...viewport.value, zoom: viewport.value.zoom - 0.25 }
      }),
      panBy,
      loadInitialOrRouteBoard,
      closeBoardHeaderEditing: vi.fn(),
      startCanvasInteraction,
      stopCanvasInteraction,
      startNodeDragInteraction: vi.fn(),
      stopNodeDragInteraction: vi.fn(),
      refreshFlowHandleVisibility: vi.fn(),
      viewportPersistence: options.viewportPersistence,
    }),
  )
  if (!runtime) throw new Error('runtime was not created')

  return {
    runtime,
    scope,
    currentBoardId,
    userUid,
    viewport,
    panBy,
    setViewport,
    setCenter,
    loadInitialOrRouteBoard,
    startCanvasInteraction,
    stopCanvasInteraction,
  }
}

describe('useCanvasViewportRuntime', () => {
  beforeEach(() => {
    localStorage.clear()
    frameProbeMocks.isEnabled.mockReturnValue(false)
    frameProbeMocks.begin.mockReset()
    frameProbeMocks.begin.mockReturnValue({
      mark: vi.fn(),
      end: vi.fn(),
      cleanup: vi.fn(),
    })
    frameProbeMocks.waitForNextRenderFrame.mockClear()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })

  it('observes flow viewport movement without committing persistence during move frames', () => {
    const harness = createViewportRuntimeHarness()

    harness.runtime.onFlowMove({ flowTransform: { x: 24, y: 32, zoom: 1.5 } })

    expect(readStoredViewport()).toBeNull()
    harness.scope.stop()
  })

  it('marks only viewport pan movement with the pan interaction key', () => {
    const harness = createViewportRuntimeHarness()

    harness.runtime.onFlowMoveStart()
    harness.runtime.onFlowMove({ flowTransform: { x: 24, y: 32, zoom: 1 } })
    harness.runtime.onFlowMoveEnd({ flowTransform: { x: 24, y: 32, zoom: 1 } })

    expect(harness.startCanvasInteraction).toHaveBeenCalledWith('flow-viewport')
    expect(harness.startCanvasInteraction).toHaveBeenCalledWith('viewport-pan')
    expect(harness.stopCanvasInteraction).toHaveBeenCalledWith('viewport-pan')
    expect(harness.stopCanvasInteraction).toHaveBeenCalledWith('flow-viewport')

    const zoomHarness = createViewportRuntimeHarness()
    zoomHarness.runtime.onFlowMoveStart()
    zoomHarness.runtime.onFlowMove({ flowTransform: { x: 0, y: 0, zoom: 1.5 } })
    zoomHarness.runtime.onFlowMoveEnd({ flowTransform: { x: 0, y: 0, zoom: 1.5 } })

    expect(zoomHarness.startCanvasInteraction).not.toHaveBeenCalledWith('viewport-pan')
    zoomHarness.scope.stop()
    harness.scope.stop()
  })

  it('commits viewport persistence on viewport change end', () => {
    const harness = createViewportRuntimeHarness()

    harness.runtime.onFlowMoveStart()
    harness.runtime.onViewportChangeEnd({ x: 48, y: 64, zoom: 2 })
    harness.runtime.onFlowMoveEnd({ flowTransform: { x: 48, y: 64, zoom: 2 } })

    expect(harness.startCanvasInteraction).toHaveBeenCalledWith('flow-viewport')
    expect(readStoredViewport()).toMatchObject({ x: 48, y: 64, zoom: 2 })
    expect(harness.stopCanvasInteraction).toHaveBeenCalledWith('flow-viewport')
    harness.scope.stop()
  })

  it('coalesces viewport persistence after repeated zooming', async () => {
    vi.useFakeTimers()
    const harness = createViewportRuntimeHarness()

    harness.runtime.onZoomIn()
    harness.runtime.onZoomIn()
    harness.runtime.onZoomOut()
    await flushPromises()
    harness.runtime.onViewportChangeEnd(harness.viewport.value)

    expect(readStoredViewport()).toBeNull()

    vi.advanceTimersByTime(179)
    expect(readStoredViewport()).toBeNull()

    harness.runtime.onZoomIn()
    await flushPromises()
    vi.advanceTimersByTime(179)
    expect(readStoredViewport()).toBeNull()

    vi.advanceTimersByTime(180)

    expect(readStoredViewport()).toMatchObject({ x: 0, y: 0, zoom: 1.5 })
    harness.runtime.onViewportChangeEnd(harness.viewport.value)
    expect(readStoredViewport()).toMatchObject({ x: 0, y: 0, zoom: 1.5 })
    harness.scope.stop()
  })

  it('flushes pending programmatic viewport persistence before disposal', async () => {
    vi.useFakeTimers()
    const harness = createViewportRuntimeHarness()

    harness.runtime.onZoomIn()
    await flushPromises()

    expect(readStoredViewport()).toBeNull()

    harness.scope.stop()

    expect(readStoredViewport()).toMatchObject({ x: 0, y: 0, zoom: 1.25 })
  })

  it('keeps delayed programmatic viewport persistence scoped to the originating board', async () => {
    vi.useFakeTimers()
    const harness = createViewportRuntimeHarness()

    harness.runtime.onZoomIn()
    await flushPromises()
    harness.currentBoardId.value = 'board-b'
    harness.viewport.value = { x: 99, y: 88, zoom: 1 }

    vi.advanceTimersByTime(180)

    expect(readStoredViewport('board-a')).toMatchObject({ x: 0, y: 0, zoom: 1.25 })
    expect(readStoredViewport('board-b')).toBeNull()
    harness.scope.stop()
  })

  it('commits ordinary flow viewport end while programmatic persistence is pending', async () => {
    vi.useFakeTimers()
    const harness = createViewportRuntimeHarness()

    harness.runtime.onZoomIn()
    await flushPromises()
    harness.runtime.onFlowMoveStart()
    harness.viewport.value = { x: 48, y: 64, zoom: 2 }
    harness.runtime.onViewportChangeEnd(harness.viewport.value)
    harness.runtime.onFlowMoveEnd({ flowTransform: harness.viewport.value })

    expect(readStoredViewport()).toMatchObject({ x: 48, y: 64, zoom: 2 })

    vi.advanceTimersByTime(180)

    expect(readStoredViewport()).toMatchObject({ x: 48, y: 64, zoom: 2 })
    harness.scope.stop()
  })

  it('centers the viewport and coalesces persistence when the minimap requests centering', async () => {
    vi.useFakeTimers()
    const harness = createViewportRuntimeHarness()

    harness.runtime.onMiniMapCenterRequested({ position: { x: 240, y: 180 } })
    await flushPromises()

    expect(harness.setCenter).toHaveBeenCalledWith(240, 180, {
      duration: 0,
      zoom: 1,
    })
    expect(readStoredViewport()).toBeNull()

    vi.advanceTimersByTime(180)

    expect(readStoredViewport()).toMatchObject({ x: -240, y: -180, zoom: 1 })
    harness.scope.stop()
  })

  it('routes viewport load, programmatic, ordinary, minimap, and disposal persistence through one interface', async () => {
    const viewportPersistence: CanvasViewportPersistence = {
      restore: vi.fn(async () => ({ x: 0, y: 0, zoom: 1 })),
      scheduleDelayedPersist: vi.fn(),
      persistViewportEnd: vi.fn(),
      persistNow: vi.fn(),
      flush: vi.fn(),
      dispose: vi.fn(),
    }
    const harness = createViewportRuntimeHarness({ viewportPersistence })

    await harness.runtime.loadCanvasBoardAndProbe()
    harness.runtime.onZoomIn()
    await flushPromises()
    harness.runtime.onPanViewport({ x: 8, y: 4 })
    harness.viewport.value = { x: 24, y: 32, zoom: 1.25 }
    harness.runtime.onViewportChangeEnd(harness.viewport.value)
    harness.runtime.onMiniMapDragStart()
    harness.runtime.onMiniMapDragMove({ x: 2, y: 2 })
    await flushPromises()
    harness.runtime.onMiniMapDragEnd()
    await flushPromises()
    harness.scope.stop()

    expect(viewportPersistence.restore).toHaveBeenCalledOnce()
    expect(viewportPersistence.scheduleDelayedPersist).toHaveBeenCalledTimes(2)
    expect(viewportPersistence.persistViewportEnd).toHaveBeenCalledWith({
      x: 24,
      y: 32,
      zoom: 1.25,
    })
    expect(viewportPersistence.persistNow).toHaveBeenCalledOnce()
    expect(viewportPersistence.dispose).toHaveBeenCalledOnce()
  })

  it('coalesces minimap drag deltas and commits persistence once on drag end', async () => {
    vi.useFakeTimers()
    const requestAnimationFrame = vi
      .fn<(callback: FrameRequestCallback) => number>()
      .mockImplementation((callback) => window.setTimeout(() => callback(performance.now()), 16))
    const cancelAnimationFrame = vi.fn((id: number) => window.clearTimeout(id))
    vi.stubGlobal('requestAnimationFrame', requestAnimationFrame)
    vi.stubGlobal('cancelAnimationFrame', cancelAnimationFrame)
    const harness = createViewportRuntimeHarness()
    harness.viewport.value = { x: -100, y: -200, zoom: 2 }

    harness.runtime.onMiniMapDragStart()
    harness.runtime.onMiniMapDragMove({ x: 10, y: -5 })
    harness.runtime.onMiniMapDragMove({ x: 2, y: 4 })

    expect(harness.setViewport).not.toHaveBeenCalled()
    expect(readStoredViewport()).toBeNull()

    vi.runOnlyPendingTimers()
    await flushPromises()

    expect(harness.setViewport).toHaveBeenCalledOnce()
    expect(harness.viewport.value).toEqual({ x: -124, y: -198, zoom: 2 })
    expect(readStoredViewport()).toBeNull()

    harness.runtime.onMiniMapDragEnd()
    await flushPromises()

    expect(harness.startCanvasInteraction).toHaveBeenCalledWith('flow-viewport')
    expect(harness.startCanvasInteraction).not.toHaveBeenCalledWith('viewport-pan')
    expect(harness.stopCanvasInteraction).toHaveBeenCalledWith('flow-viewport')
    expect(readStoredViewport()).toMatchObject({ x: -124, y: -198, zoom: 2 })
    harness.scope.stop()
  })

  it('feeds minimap drag movement through the viewport frame probe path', async () => {
    const harness = createViewportRuntimeHarness()
    harness.viewport.value = { x: -100, y: -200, zoom: 2 }

    harness.runtime.onMiniMapDragStart()
    harness.runtime.onMiniMapDragMove({ x: 10, y: -5 })
    await flushPromises()
    harness.runtime.onMiniMapDragEnd()
    await flushPromises()

    expect(frameProbeMocks.begin).toHaveBeenCalledWith('pan', expect.any(Function))
    harness.scope.stop()
  })

  it('waits for async minimap viewport updates before committing persistence', async () => {
    let resolveViewportUpdate: () => void = () => {
      throw new Error('viewport update resolver was not captured')
    }
    const harness = createViewportRuntimeHarness()
    harness.viewport.value = { x: -100, y: -200, zoom: 2 }
    harness.setViewport.mockImplementationOnce(async (nextViewport) => {
      await new Promise<void>((resolve) => {
        resolveViewportUpdate = resolve
      })
      harness.viewport.value = nextViewport
    })

    harness.runtime.onMiniMapDragStart()
    harness.runtime.onMiniMapDragMove({ x: 10, y: -5 })
    harness.runtime.onMiniMapDragEnd()
    await flushPromises()

    expect(readStoredViewport()).toBeNull()

    resolveViewportUpdate()
    await flushPromises()

    expect(harness.viewport.value).toEqual({ x: -120, y: -190, zoom: 2 })
    expect(readStoredViewport()).toMatchObject({ x: -120, y: -190, zoom: 2 })
    harness.scope.stop()
  })

  it('serializes async minimap viewport updates and persists the latest drag target', async () => {
    vi.useFakeTimers()
    const requestAnimationFrame = vi
      .fn<(callback: FrameRequestCallback) => number>()
      .mockImplementation((callback) => window.setTimeout(() => callback(performance.now()), 16))
    const cancelAnimationFrame = vi.fn((id: number) => window.clearTimeout(id))
    vi.stubGlobal('requestAnimationFrame', requestAnimationFrame)
    vi.stubGlobal('cancelAnimationFrame', cancelAnimationFrame)
    let resolveFirstViewportUpdate: () => void = () => {
      throw new Error('first viewport update resolver was not captured')
    }
    const harness = createViewportRuntimeHarness()
    harness.viewport.value = { x: -100, y: -200, zoom: 2 }
    harness.setViewport.mockImplementationOnce(async (nextViewport) => {
      await new Promise<void>((resolve) => {
        resolveFirstViewportUpdate = resolve
      })
      harness.viewport.value = nextViewport
    })

    harness.runtime.onMiniMapDragStart()
    harness.runtime.onMiniMapDragMove({ x: 10, y: -5 })
    vi.runOnlyPendingTimers()
    await flushPromises()
    harness.runtime.onMiniMapDragMove({ x: 3, y: 2 })
    harness.runtime.onMiniMapDragEnd()
    await flushPromises()

    expect(harness.setViewport).toHaveBeenCalledOnce()
    expect(readStoredViewport()).toBeNull()

    resolveFirstViewportUpdate()
    await flushPromises(8)

    expect(harness.setViewport).toHaveBeenCalledTimes(2)
    expect(harness.setViewport).toHaveBeenLastCalledWith(
      { x: -126, y: -194, zoom: 2 },
      { duration: 0 },
    )
    expect(harness.viewport.value).toEqual({ x: -126, y: -194, zoom: 2 })
    expect(readStoredViewport()).toMatchObject({ x: -126, y: -194, zoom: 2 })
    harness.scope.stop()
  })

  it('keeps a new minimap drag behind an unresolved previous viewport update', async () => {
    let resolveFirstViewportUpdate: () => void = () => {
      throw new Error('first viewport update resolver was not captured')
    }
    const harness = createViewportRuntimeHarness()
    harness.viewport.value = { x: -100, y: -200, zoom: 2 }
    harness.setViewport.mockImplementationOnce(async (nextViewport) => {
      await new Promise<void>((resolve) => {
        resolveFirstViewportUpdate = resolve
      })
      harness.viewport.value = nextViewport
    })

    harness.runtime.onMiniMapDragStart()
    harness.runtime.onMiniMapDragMove({ x: 10, y: -5 })
    harness.runtime.onMiniMapDragEnd()
    await flushPromises()

    harness.runtime.onMiniMapDragStart()
    harness.runtime.onMiniMapDragMove({ x: 3, y: 2 })
    harness.runtime.onMiniMapDragEnd()
    await flushPromises()

    expect(harness.setViewport).toHaveBeenCalledOnce()
    expect(readStoredViewport()).toBeNull()

    resolveFirstViewportUpdate()
    await flushPromises(8)

    expect(harness.setViewport).toHaveBeenCalledTimes(2)
    expect(harness.setViewport).toHaveBeenLastCalledWith(
      { x: -106, y: -204, zoom: 2 },
      { duration: 0 },
    )
    expect(readStoredViewport()).toMatchObject({ x: -106, y: -204, zoom: 2 })
    harness.scope.stop()
  })

  it('does not persist a minimap drag that ends without movement', async () => {
    const harness = createViewportRuntimeHarness()

    harness.runtime.onMiniMapDragStart()
    harness.runtime.onMiniMapDragEnd()
    await flushPromises()

    expect(harness.startCanvasInteraction).toHaveBeenCalledWith('flow-viewport')
    expect(harness.stopCanvasInteraction).toHaveBeenCalledWith('flow-viewport')
    expect(harness.setViewport).not.toHaveBeenCalled()
    expect(readStoredViewport()).toBeNull()
    harness.scope.stop()
  })

  it('uses panBy for keyboard panning and coalesces persistence when the viewport moves', () => {
    vi.useFakeTimers()
    const harness = createViewportRuntimeHarness()

    harness.runtime.onPanViewport({ x: 16, y: -8 })
    harness.runtime.onViewportChangeEnd(harness.viewport.value)

    expect(harness.panBy).toHaveBeenCalledWith({ x: 16, y: -8 })
    expect(harness.viewport.value).toEqual({ x: 16, y: -8, zoom: 1 })
    expect(readStoredViewport()).toBeNull()

    harness.runtime.onPanViewport({ x: 4, y: 4 })
    vi.advanceTimersByTime(179)
    expect(readStoredViewport()).toBeNull()

    vi.advanceTimersByTime(180)

    expect(readStoredViewport()).toMatchObject({ x: 20, y: -4, zoom: 1 })
    harness.scope.stop()
  })

  it('does not commit dirty persistence when panBy is constrained or uninitialized', () => {
    const harness = createViewportRuntimeHarness()
    harness.panBy.mockReturnValueOnce(false)

    harness.runtime.onPanViewport({ x: 16, y: -8 })

    expect(readStoredViewport()).toBeNull()
    harness.scope.stop()
  })

  it('commits a later user viewport change after a constrained keyboard pan', () => {
    const harness = createViewportRuntimeHarness()
    harness.panBy.mockReturnValueOnce(false)

    harness.runtime.onPanViewport({ x: 16, y: -8 })
    harness.viewport.value = { x: 24, y: 32, zoom: 1.5 }
    harness.runtime.onViewportChangeEnd(harness.viewport.value)

    expect(readStoredViewport()).toMatchObject({ x: 24, y: 32, zoom: 1.5 })
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
    expect(harness.setViewport).toHaveBeenCalledWith({ x: 0, y: 0, zoom: 1 }, { duration: 0 })
    expect(frameProbeMocks.begin).not.toHaveBeenCalled()
    expect(frameProbeMocks.waitForNextRenderFrame).not.toHaveBeenCalled()
    harness.scope.stop()
  })

  it('restores browser-local viewport for the current user and board after loading', async () => {
    localStorage.setItem(
      'ideaboard:canvas:view:user-uid:board-a',
      JSON.stringify({ x: 12, y: -24, zoom: 1.75, updatedAt: 10 }),
    )
    const harness = createViewportRuntimeHarness()

    await harness.runtime.loadCanvasBoardAndProbe()

    expect(harness.setViewport).toHaveBeenCalledWith({ x: 12, y: -24, zoom: 1.75 }, { duration: 0 })
    expect(harness.viewport.value).toEqual({ x: 12, y: -24, zoom: 1.75 })
    harness.scope.stop()
  })

  it('falls back to the default viewport when browser-local viewport is invalid', async () => {
    localStorage.setItem(
      'ideaboard:canvas:view:user-uid:board-a',
      JSON.stringify({ x: 'bad', y: -24, zoom: 1.75, updatedAt: 10 }),
    )
    const harness = createViewportRuntimeHarness()

    await harness.runtime.loadCanvasBoardAndProbe()

    expect(harness.setViewport).toHaveBeenCalledWith({ x: 0, y: 0, zoom: 1 }, { duration: 0 })
    expect(harness.viewport.value).toEqual({ x: 0, y: 0, zoom: 1 })
    harness.scope.stop()
  })
})
