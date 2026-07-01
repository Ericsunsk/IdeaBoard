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
  const fullNodes = shallowRef<Node[]>([])
  const fullEdges = shallowRef<Edge[]>([])
  const nodes = fullNodes
  const edges = fullEdges
  const boardBaseSnapshot = shallowRef(createEmptyCanvasBoardSnapshot())
  const viewport = ref({ x: 0, y: 0, zoom: 1 })
  const savedSnapshots: CanvasBoardSnapshot[] = []
  const scheduledMicrotasks: Array<() => void> = []
  const onNativeRemove = vi.fn()
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
    onWarning: vi.fn(),
    onSuccess: vi.fn(),
    onError: vi.fn(),
    onNativeRemove,
    scheduleMicrotask: (callback) => {
      scheduledMicrotasks.push(callback)
    },
  }
  const runtime = useFlowCanvasRuntime(runtimeOptions)

  async function flushNativeGraphChanges() {
    while (scheduledMicrotasks.length > 0) {
      scheduledMicrotasks.shift()?.()
      await nextTick()
    }
  }

  async function applyNativeNodeChanges(
    applyChanges: () => void,
    changes: Parameters<typeof runtime.onNodesChange>[0],
  ) {
    runtime.onNodesChange(changes)
    applyChanges()
    await flushNativeGraphChanges()
  }

  async function applyNativeEdgeChanges(
    applyChanges: () => void,
    changes: Parameters<typeof runtime.onEdgesChange>[0],
  ) {
    runtime.onEdgesChange(changes)
    applyChanges()
    await flushNativeGraphChanges()
  }

  return {
    runtime,
    nodes,
    edges,
    fullNodes,
    fullEdges,
    savedSnapshots,
    viewport,
    boardSession,
    onNativeRemove,
    flushNativeGraphChanges,
    applyNativeNodeChanges,
    applyNativeEdgeChanges,
  }
}

function readSelectedFlag(node: Node | undefined) {
  return (node as (Node & { selected?: boolean }) | undefined)?.selected
}

function markFlowItemSelected<T extends Node | Edge>(items: T[], itemId: string): T[] {
  return items.map((item) => (item.id === itemId ? { ...item, selected: true } : item))
}

function patchFlowItem<T extends Node | Edge>(
  items: T[],
  itemId: string,
  patch: Partial<T> & { selected?: boolean; dragging?: boolean },
): T[] {
  return items.map((item) => (item.id === itemId ? ({ ...item, ...patch } as T) : item))
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
    expect(harness.nodes.value.map((node) => node.id)).toEqual(['image-1', 'prompt-1', 'prompt-2'])
    expect(harness.edges.value.map((edge) => edge.id)).toEqual(['connection-3'])
  })

  it('does not reslice the Vue Flow store when the viewport changes', async () => {
    const harness = createRuntimeHarness()

    await harness.runtime.loadInitialOrRouteBoard()

    expect(harness.nodes.value.map((node) => node.id)).toEqual(['image-1', 'prompt-1', 'prompt-2'])
    expect(harness.edges.value.map((edge) => edge.id)).toEqual(['connection-3'])

    harness.viewport.value = { x: -1800, y: 0, zoom: 1 }
    await nextTick()

    expect(harness.nodes.value.map((node) => node.id)).toEqual(['image-1', 'prompt-1', 'prompt-2'])
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

  it('syncs Vue Flow node position during drag, saves on drag commit, and keeps selection', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.loadInitialOrRouteBoard()

    const originalFullNode = harness.fullNodes.value.find((node) => node.id === 'prompt-1')
    expect(originalFullNode?.position).toEqual({ x: 400, y: 40 })

    harness.runtime.flowTransactions.beginNodeDragTransaction()

    for (const x of [440, 500, 620]) {
      await harness.applyNativeNodeChanges(() => {
        harness.nodes.value = patchFlowItem(harness.nodes.value, 'prompt-1', {
          selected: true,
          position: { x, y: 80 },
          dragging: true,
        } as Partial<Node>)
      }, [
        { id: 'prompt-1', type: 'select', selected: true },
        {
          id: 'prompt-1',
          type: 'position',
          position: { x, y: 80 },
          from: { x: 400, y: 40 },
          dragging: true,
        },
      ])
      await nextTick()

      expect(harness.fullNodes.value.find((node) => node.id === 'prompt-1')?.position).toEqual({
        x,
        y: 80,
      })
      expect(harness.savedSnapshots).toHaveLength(0)
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

  it('keeps deselection on the Vue Flow bound graph when history is captured', async () => {
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

  it('accepts Vue Flow select changes without marking the board dirty', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.loadInitialOrRouteBoard()
    await nextTick()

    await harness.applyNativeNodeChanges(() => {
      harness.nodes.value = patchFlowItem(harness.nodes.value, 'prompt-1', { selected: true })
    }, [{ id: 'prompt-1', type: 'select', selected: true }])
    await harness.applyNativeEdgeChanges(() => {
      harness.edges.value = patchFlowItem(harness.edges.value, 'connection-3', { selected: true })
    }, [{ id: 'connection-3', type: 'select', selected: true }])

    expect(readNodeSelected(harness.fullNodes.value, 'prompt-1')).toBe(true)
    expect(readNodeSelected(harness.nodes.value, 'prompt-1')).toBe(true)
    expect(readEdgeSelected(harness.fullEdges.value, 'connection-3')).toBe(true)
    expect(readEdgeSelected(harness.edges.value, 'connection-3')).toBe(true)
    expect(harness.savedSnapshots).toHaveLength(0)
    expect(harness.onNativeRemove).not.toHaveBeenCalled()
  })

  it('accepts Vue Flow position changes without saving before drag transaction commits', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.loadInitialOrRouteBoard()
    await nextTick()

    await harness.applyNativeNodeChanges(() => {
      harness.nodes.value = patchFlowItem(harness.nodes.value, 'prompt-1', {
        position: { x: 720, y: 96 },
        dragging: true,
      } as Partial<Node>)
    }, [
      {
        id: 'prompt-1',
        type: 'position',
        position: { x: 720, y: 96 },
        from: { x: 400, y: 40 },
        dragging: true,
      },
    ])

    expect(harness.fullNodes.value.find((node) => node.id === 'prompt-1')?.position).toEqual({
      x: 720,
      y: 96,
    })
    expect(harness.savedSnapshots).toHaveLength(0)
  })

  it('does not rewrite the controlled node model when a drag transaction commits', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.loadInitialOrRouteBoard()
    await nextTick()

    harness.runtime.flowTransactions.beginNodeDragTransaction()

    await harness.applyNativeNodeChanges(() => {
      harness.nodes.value = patchFlowItem(harness.nodes.value, 'prompt-1', {
        position: { x: 720, y: 96 },
        dragging: true,
      } as Partial<Node>)
    }, [
      {
        id: 'prompt-1',
        type: 'position',
        position: { x: 720, y: 96 },
        from: { x: 400, y: 40 },
        dragging: true,
      },
    ])
    const nodesBeforeCommit = harness.nodes.value

    harness.runtime.flowTransactions.commitNodeDragTransaction()
    await nextTick()

    expect(harness.fullNodes.value.find((node) => node.id === 'prompt-1')?.position).toEqual({
      x: 720,
      y: 96,
    })
    expect(harness.nodes.value).toBe(nodesBeforeCommit)
  })

  it('records native node and edge remove changes after Vue Flow applies them', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.loadInitialOrRouteBoard()
    await nextTick()

    harness.runtime.onNodesChange([{ id: 'image-1', type: 'remove' }])
    harness.runtime.onEdgesChange([
      {
        id: 'connection-3',
        type: 'remove',
        source: 'image-1',
        target: 'prompt-1',
        sourceHandle: 'out:image',
        targetHandle: 'in:image',
      },
    ])
    harness.nodes.value = harness.nodes.value.filter((node) => node.id !== 'image-1')
    harness.edges.value = harness.edges.value.filter((edge) => edge.id !== 'connection-3')
    await harness.flushNativeGraphChanges()

    expect(harness.onNativeRemove).toHaveBeenCalledOnce()
    expect(harness.fullNodes.value.map((node) => node.id)).toEqual(['prompt-1', 'prompt-2'])
    expect(harness.fullEdges.value).toHaveLength(0)

    await harness.runtime.flushAutosave({ silent: true })

    expect(harness.savedSnapshots[0]?.imageNodes).toHaveLength(0)
    expect(harness.savedSnapshots[0]?.connections).toHaveLength(0)
  })

  it('records native edge remove before node remove as one cleanup', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.loadInitialOrRouteBoard()
    await nextTick()

    harness.runtime.onEdgesChange([
      {
        id: 'connection-3',
        type: 'remove',
        source: 'image-1',
        target: 'prompt-1',
        sourceHandle: 'out:image',
        targetHandle: 'in:image',
      },
    ])
    harness.runtime.onNodesChange([{ id: 'image-1', type: 'remove' }])
    harness.edges.value = harness.edges.value.filter((edge) => edge.id !== 'connection-3')
    harness.nodes.value = harness.nodes.value.filter((node) => node.id !== 'image-1')
    await harness.flushNativeGraphChanges()

    expect(harness.onNativeRemove).toHaveBeenCalledOnce()
    expect(harness.fullNodes.value.map((node) => node.id)).toEqual(['prompt-1', 'prompt-2'])
    expect(harness.fullEdges.value).toHaveLength(0)
  })

  it('restores native deletes through undo and redo', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.loadInitialOrRouteBoard()
    await nextTick()

    harness.runtime.onNodesChange([{ id: 'image-1', type: 'remove' }])
    harness.runtime.onEdgesChange([
      {
        id: 'connection-3',
        type: 'remove',
        source: 'image-1',
        target: 'prompt-1',
        sourceHandle: 'out:image',
        targetHandle: 'in:image',
      },
    ])
    harness.nodes.value = harness.nodes.value.filter((node) => node.id !== 'image-1')
    harness.edges.value = harness.edges.value.filter((edge) => edge.id !== 'connection-3')
    await harness.flushNativeGraphChanges()

    harness.runtime.flowHistory.undo()

    expect(harness.fullNodes.value.map((node) => node.id)).toEqual([
      'image-1',
      'prompt-1',
      'prompt-2',
    ])
    expect(harness.fullEdges.value.map((edge) => edge.id)).toEqual(['connection-3'])

    harness.runtime.flowHistory.redo()

    expect(harness.fullNodes.value.map((node) => node.id)).toEqual(['prompt-1', 'prompt-2'])
    expect(harness.fullEdges.value).toHaveLength(0)
  })
})
