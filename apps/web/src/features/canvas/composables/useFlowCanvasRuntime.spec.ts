import type { Edge, Node } from '@vue-flow/core'
import { computed, nextTick, ref, shallowRef } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { CanvasBoardSession } from '../board-session'
import { createEmptyCanvasBoardSnapshot, type CanvasBoardSnapshot } from '../../canvas-board/domain'
import { useFlowCanvasRuntime, type UseFlowCanvasRuntimeOptions } from './useFlowCanvasRuntime'

function installManualAnimationFrame() {
  const callbacks = new Map<number, FrameRequestCallback>()
  let nextFrameId = 1
  const requestAnimationFrame = vi.fn((callback: FrameRequestCallback) => {
    const frameId = nextFrameId
    nextFrameId += 1
    callbacks.set(frameId, callback)
    return frameId
  })
  const cancelAnimationFrame = vi.fn((frameId: number) => {
    callbacks.delete(frameId)
  })
  vi.stubGlobal('requestAnimationFrame', requestAnimationFrame)
  vi.stubGlobal('cancelAnimationFrame', cancelAnimationFrame)

  return {
    requestAnimationFrame,
    cancelAnimationFrame,
    flushFrame: () => {
      const frameCallbacks = [...callbacks.values()]
      callbacks.clear()
      frameCallbacks.forEach((callback) => callback(16))
    },
  }
}

function createSnapshot(): CanvasBoardSnapshot {
  const snapshot = createEmptyCanvasBoardSnapshot()
  snapshot.imageNodes = [
    {
      id: 1,
      type: 'image-node',
      x: 40,
      y: 40,
      width: 280,
      sourceImageUrl: 'https://example.com/source.png',
      sourceImageRatio: 1,
      outputStatus: 'ready',
      outputImages: [{ url: 'https://example.com/output.png', ratio: 1 }],
    },
  ]
  snapshot.promptNodes = [
    {
      id: 1,
      type: 'prompt',
      x: 400,
      y: 40,
      width: 280,
      text: 'visible',
      outputCount: 1,
      aspectRatio: 'auto',
      inputImages: [],
    },
    {
      id: 2,
      type: 'prompt',
      x: 2200,
      y: 40,
      width: 280,
      text: 'hidden',
      outputCount: 1,
      aspectRatio: 'auto',
      inputImages: [],
    },
  ]
  snapshot.connections = [
    {
      id: 3,
      fromNodeType: 'image',
      fromNodeId: 1,
      fromPort: 'image',
      toNodeType: 'prompt',
      toNodeId: 1,
      toPort: 'image',
    },
  ]
  return snapshot
}

function createRuntimeHarness(snapshot = createSnapshot()) {
  const nodes = shallowRef<Node[]>([])
  const edges = shallowRef<Edge[]>([])
  const fullNodes = shallowRef<Node[]>([])
  const fullEdges = shallowRef<Edge[]>([])
  const boardBaseSnapshot = shallowRef(createEmptyCanvasBoardSnapshot())
  const viewport = ref({ x: 0, y: 0, zoom: 1 })
  const savedSnapshots: CanvasBoardSnapshot[] = []
  const setNodes = vi.fn((nextNodes: Node[]) => {
    nodes.value = nextNodes
  })
  const setEdges = vi.fn((nextEdges: Edge[]) => {
    edges.value = nextEdges
  })
  const boardSession: CanvasBoardSession = {
    currentBoardId: computed(() => 'board-a'),
    exists: vi.fn(() => true),
    load: vi.fn(async () => snapshot),
    save: vi.fn(async (nextSnapshot: CanvasBoardSnapshot) => {
      savedSnapshots.push(nextSnapshot)
      return { snapshot: nextSnapshot }
    }),
    refreshPreview: vi.fn(),
  }

  const runtimeOptions: UseFlowCanvasRuntimeOptions = {
    nodes,
    edges,
    fullNodes,
    fullEdges,
    boardBaseSnapshot,
    isDirty: ref(false),
    saveStatus: ref<'idle' | 'saved' | 'failed'>('idle'),
    isLoadingBoardSnapshot: ref(false),
    isSavingBoard: ref(false),
    boardSession,
    getViewport: () => viewport.value,
    getViewportSize: () => ({ width: 900, height: 500 }),
    setViewport: (nextViewport) => {
      viewport.value = nextViewport
    },
    setNodes,
    setEdges,
    onWarning: vi.fn(),
    onSuccess: vi.fn(),
    onError: vi.fn(),
  }
  const runtime = useFlowCanvasRuntime(runtimeOptions)

  return {
    runtime,
    nodes,
    edges,
    fullNodes,
    fullEdges,
    savedSnapshots,
    viewport,
    setNodes,
    setEdges,
  }
}

describe('useFlowCanvasRuntime', () => {
  afterEach(() => {
    vi.unstubAllEnvs()
    vi.unstubAllGlobals()
  })

  it('hydrates the full graph while mounting only the viewport subset', async () => {
    const harness = createRuntimeHarness()

    await harness.runtime.loadInitialOrRouteBoard()

    expect(harness.fullNodes.value.map((node) => node.id)).toEqual([
      'image-1',
      'prompt-1',
      'prompt-2',
    ])
    expect(harness.fullEdges.value.map((edge) => edge.id)).toEqual(['connection-3'])
    expect(harness.nodes.value.map((node) => node.id)).toEqual(['image-1', 'prompt-1'])
    expect(harness.edges.value.map((edge) => edge.id)).toEqual(['connection-3'])
  })

  it('refreshes mounted nodes and visible edges when the viewport changes', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.loadInitialOrRouteBoard()

    harness.runtime.flushMountedGraphRefresh({ x: -900, y: 0, zoom: 1 })

    expect(harness.nodes.value.map((node) => node.id)).toEqual([
      'image-1',
      'prompt-1',
      'prompt-2',
    ])
    expect(harness.edges.value.map((edge) => edge.id)).toEqual(['connection-3'])
  })

  it('coalesces scheduled mounted graph refreshes into one animation frame', async () => {
    const animationFrame = installManualAnimationFrame()
    const harness = createRuntimeHarness()
    await harness.runtime.loadInitialOrRouteBoard()
    harness.setNodes.mockClear()
    harness.setEdges.mockClear()

    harness.runtime.scheduleMountedGraphRefresh({ x: -100, y: 0, zoom: 1 })
    harness.runtime.scheduleMountedGraphRefresh({ x: -900, y: 0, zoom: 1 })
    harness.runtime.scheduleMountedGraphRefresh({ x: -1800, y: 0, zoom: 1 })

    expect(animationFrame.requestAnimationFrame).toHaveBeenCalledTimes(1)
    expect(harness.setNodes).not.toHaveBeenCalled()
    expect(harness.setEdges).not.toHaveBeenCalled()

    animationFrame.flushFrame()

    expect(harness.setNodes).toHaveBeenCalledTimes(1)
    expect(harness.setEdges).toHaveBeenCalledTimes(1)
    expect(harness.nodes.value.map((node) => node.id)).toEqual(['prompt-2'])
    expect(harness.edges.value.map((edge) => edge.id)).toEqual([])
    expect(harness.fullNodes.value.map((node) => node.id)).toEqual([
      'image-1',
      'prompt-1',
      'prompt-2',
    ])
  })

  it('flushes a pending mounted graph refresh immediately with the latest viewport', async () => {
    const animationFrame = installManualAnimationFrame()
    const harness = createRuntimeHarness()
    await harness.runtime.loadInitialOrRouteBoard()
    harness.setNodes.mockClear()
    harness.setEdges.mockClear()

    harness.runtime.scheduleMountedGraphRefresh({ x: -100, y: 0, zoom: 1 })
    harness.runtime.scheduleMountedGraphRefresh({ x: -1800, y: 0, zoom: 1 })
    harness.runtime.flushMountedGraphRefresh()

    expect(animationFrame.cancelAnimationFrame).toHaveBeenCalledTimes(1)
    expect(harness.setNodes).toHaveBeenCalledTimes(1)
    expect(harness.setEdges).toHaveBeenCalledTimes(1)
    expect(harness.nodes.value.map((node) => node.id)).toEqual(['prompt-2'])
    expect(harness.edges.value.map((edge) => edge.id)).toEqual([])

    animationFrame.flushFrame()

    expect(harness.setNodes).toHaveBeenCalledTimes(1)
    expect(harness.setEdges).toHaveBeenCalledTimes(1)
  })

  it('runs scheduled mounted graph refresh synchronously when animation frames are unavailable', async () => {
    vi.stubGlobal('requestAnimationFrame', undefined)
    vi.stubGlobal('cancelAnimationFrame', undefined)
    const harness = createRuntimeHarness()
    await harness.runtime.loadInitialOrRouteBoard()
    harness.setNodes.mockClear()
    harness.setEdges.mockClear()

    harness.runtime.scheduleMountedGraphRefresh({ x: -1800, y: 0, zoom: 1 })

    expect(harness.setNodes).toHaveBeenCalledTimes(1)
    expect(harness.setEdges).toHaveBeenCalledTimes(1)
    expect(harness.nodes.value.map((node) => node.id)).toEqual(['prompt-2'])
    expect(harness.edges.value.map((edge) => edge.id)).toEqual([])
  })

  it('mounts the full graph and skips scheduled culling work when viewport culling is disabled', async () => {
    vi.stubEnv('VITE_CANVAS_FLOW_ENABLE_CULLING', 'false')
    const animationFrame = installManualAnimationFrame()
    const harness = createRuntimeHarness()

    await harness.runtime.loadInitialOrRouteBoard()

    expect(harness.nodes.value.map((node) => node.id)).toEqual([
      'image-1',
      'prompt-1',
      'prompt-2',
    ])
    expect(harness.edges.value.map((edge) => edge.id)).toEqual(['connection-3'])
    harness.setNodes.mockClear()
    harness.setEdges.mockClear()

    harness.runtime.scheduleMountedGraphRefresh({ x: -1800, y: 0, zoom: 1 })

    expect(animationFrame.requestAnimationFrame).not.toHaveBeenCalled()
    expect(harness.setNodes).not.toHaveBeenCalled()
    expect(harness.setEdges).not.toHaveBeenCalled()

    harness.runtime.flushMountedGraphRefresh({ x: -1800, y: 0, zoom: 1 })

    expect(harness.setNodes).not.toHaveBeenCalled()
    expect(harness.setEdges).not.toHaveBeenCalled()
    expect(harness.nodes.value.map((node) => node.id)).toEqual([
      'image-1',
      'prompt-1',
      'prompt-2',
    ])
  })

  it('saves hidden nodes and edges from the full graph', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.loadInitialOrRouteBoard()
    await nextTick()

    harness.runtime.flowTransactions.commitDirty()
    await harness.runtime.flushAutosave({ silent: true })

    expect(harness.savedSnapshots[0]?.imageNodes.map((node) => node.id)).toEqual([1])
    expect(harness.savedSnapshots[0]?.promptNodes.map((node) => node.id)).toEqual([1, 2])
    expect(harness.savedSnapshots[0]?.connections.map((connection) => connection.id)).toEqual([3])
  })

  it('defers mounted node position sync during drag and saves the final drag position', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.loadInitialOrRouteBoard()

    const originalFullNode = harness.fullNodes.value.find((node) => node.id === 'prompt-1')
    expect(originalFullNode?.position).toEqual({ x: 400, y: 40 })

    harness.runtime.flowTransactions.beginNodeDragTransaction()

    for (const x of [440, 500, 620]) {
      harness.nodes.value = harness.nodes.value.map((node) =>
        node.id === 'prompt-1' ? { ...node, position: { x, y: 80 } } : node,
      )
      await nextTick()

      expect(harness.fullNodes.value.find((node) => node.id === 'prompt-1')?.position).toEqual({
        x: 400,
        y: 40,
      })
    }

    harness.runtime.flowTransactions.commitNodeDragTransaction()
    await nextTick()

    expect(harness.fullNodes.value.find((node) => node.id === 'prompt-1')?.position).toEqual({
      x: 620,
      y: 80,
    })

    await harness.runtime.flushAutosave({ silent: true })

    expect(harness.savedSnapshots[0]?.promptNodes.find((node) => node.id === 1)).toMatchObject({
      x: 620,
      y: 80,
    })
    expect(harness.savedSnapshots[0]?.promptNodes.find((node) => node.id === 2)).toMatchObject({
      x: 2200,
      y: 40,
    })
  })
})
