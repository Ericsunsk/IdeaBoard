import type { Edge, Node } from '@vue-flow/core'
import { computed, nextTick, ref, shallowRef } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import type { CanvasBoardSession } from '../board-session'
import { createEmptyCanvasBoardSnapshot, type CanvasBoardSnapshot } from '../../canvas-board/domain'
import { useFlowCanvasRuntime, type UseFlowCanvasRuntimeOptions } from './useFlowCanvasRuntime'

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
    boardSession,
  }
}

function readSelectedFlag(node: Node | undefined) {
  return (node as (Node & { selected?: boolean }) | undefined)?.selected
}

function markFlowItemSelected<T extends Node | Edge>(items: T[], itemId: string): T[] {
  return items.map((item) => (item.id === itemId ? { ...item, selected: true } : item))
}

function readNodeSelected(nodes: Node[], nodeId: string) {
  return readSelectedFlag(nodes.find((node) => node.id === nodeId))
}

function readEdgeSelected(edges: Edge[], edgeId: string) {
  return (edges.find((edge) => edge.id === edgeId) as (Edge & { selected?: boolean }) | undefined)
    ?.selected
}

describe('useFlowCanvasRuntime', () => {
  it('hydrates the full graph into the Vue Flow store', async () => {
    const harness = createRuntimeHarness()

    await harness.runtime.loadInitialOrRouteBoard()

    expect(harness.fullNodes.value.map((node) => node.id)).toEqual([
      'image-1',
      'prompt-1',
      'prompt-2',
    ])
    expect(harness.fullEdges.value.map((edge) => edge.id)).toEqual(['connection-3'])
    expect(harness.nodes.value.map((node) => node.id)).toEqual([
      'image-1',
      'prompt-1',
      'prompt-2',
    ])
    expect(harness.edges.value.map((edge) => edge.id)).toEqual(['connection-3'])
  })

  it('does not reslice the Vue Flow store when the viewport changes', async () => {
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

    harness.viewport.value = { x: -1800, y: 0, zoom: 1 }
    await nextTick()

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

  it('preserves Vue Flow selection when a save commits a stabilized graph', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.loadInitialOrRouteBoard()
    await nextTick()

    harness.nodes.value = markFlowItemSelected(harness.nodes.value, 'prompt-1')

    harness.runtime.flowTransactions.commitDirty()
    await harness.runtime.flushAutosave({ silent: true })

    expect(readNodeSelected(harness.fullNodes.value, 'prompt-1')).toBe(true)
    expect(readNodeSelected(harness.nodes.value, 'prompt-1')).toBe(true)
  })

  it('preserves selection recorded in the full graph when a save commits a stabilized graph', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.loadInitialOrRouteBoard()
    await nextTick()

    harness.fullNodes.value = markFlowItemSelected(harness.fullNodes.value, 'prompt-1')

    harness.runtime.flowTransactions.commitDirty()
    await harness.runtime.flushAutosave({ silent: true })

    expect(readNodeSelected(harness.fullNodes.value, 'prompt-1')).toBe(true)
    expect(readNodeSelected(harness.nodes.value, 'prompt-1')).toBe(true)
  })

  it('preserves edge selection when a save commits a stabilized graph', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.loadInitialOrRouteBoard()
    await nextTick()

    harness.edges.value = markFlowItemSelected(harness.edges.value, 'connection-3')

    harness.runtime.flowTransactions.commitDirty()
    await harness.runtime.flushAutosave({ silent: true })

    expect(readEdgeSelected(harness.fullEdges.value, 'connection-3')).toBe(true)
    expect(readEdgeSelected(harness.edges.value, 'connection-3')).toBe(true)
  })

  it('does not carry selection into a freshly imported board snapshot', async () => {
    const firstSnapshot = createSnapshot()
    const harness = createRuntimeHarness(firstSnapshot)
    await harness.runtime.loadInitialOrRouteBoard()
    await nextTick()

    harness.nodes.value = markFlowItemSelected(harness.nodes.value, 'prompt-1')
    harness.fullNodes.value = markFlowItemSelected(harness.fullNodes.value, 'prompt-1')
    harness.edges.value = markFlowItemSelected(harness.edges.value, 'connection-3')
    harness.fullEdges.value = markFlowItemSelected(harness.fullEdges.value, 'connection-3')

    const nextSnapshot = createSnapshot()
    nextSnapshot.promptNodes = [
      {
        id: 1,
        type: 'prompt',
        x: 40,
        y: 80,
        width: 280,
        text: 'fresh board',
        outputCount: 1,
        aspectRatio: 'auto',
        inputImages: [],
      },
    ]
    ;(harness.boardSession.load as ReturnType<typeof vi.fn>).mockResolvedValue(nextSnapshot)

    await harness.runtime.loadInitialOrRouteBoard()

    expect(readNodeSelected(harness.fullNodes.value, 'prompt-1')).toBeUndefined()
    expect(readNodeSelected(harness.nodes.value, 'prompt-1')).toBeUndefined()
    expect(readEdgeSelected(harness.fullEdges.value, 'connection-3')).toBeUndefined()
    expect(readEdgeSelected(harness.edges.value, 'connection-3')).toBeUndefined()
  })

  it('defers Vue Flow node position sync during drag, saves the final position, and keeps selection', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.loadInitialOrRouteBoard()

    const originalFullNode = harness.fullNodes.value.find((node) => node.id === 'prompt-1')
    expect(originalFullNode?.position).toEqual({ x: 400, y: 40 })

    harness.runtime.flowTransactions.beginNodeDragTransaction()

    for (const x of [440, 500, 620]) {
      harness.nodes.value = harness.nodes.value.map((node) =>
        node.id === 'prompt-1' ? { ...node, selected: true, position: { x, y: 80 } } : node,
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
    expect(readNodeSelected(harness.fullNodes.value, 'prompt-1')).toBe(true)

    await harness.runtime.flushAutosave({ silent: true })

    expect(harness.savedSnapshots[0]?.promptNodes.find((node) => node.id === 1)).toMatchObject({
      x: 620,
      y: 80,
    })
    expect(harness.savedSnapshots[0]?.promptNodes.find((node) => node.id === 2)).toMatchObject({
      x: 2200,
      y: 40,
    })
    expect(readNodeSelected(harness.fullNodes.value, 'prompt-1')).toBe(true)
    expect(readNodeSelected(harness.nodes.value, 'prompt-1')).toBe(true)
  })

  it('allows Vue Flow deselection to sync back into the full graph', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.loadInitialOrRouteBoard()
    await nextTick()

    harness.fullNodes.value = markFlowItemSelected(harness.fullNodes.value, 'prompt-1')
    harness.nodes.value = harness.nodes.value.map((node) =>
      node.id === 'prompt-1' ? { ...node, selected: false } : node,
    )

    harness.runtime.flowTransactions.captureHistory()

    expect(readNodeSelected(harness.fullNodes.value, 'prompt-1')).toBe(false)
  })

})
