import type { Edge, EdgeChange, Node, NodeChange } from '@vue-flow/core'
import { computed, nextTick, ref, shallowRef } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import type { CanvasBoardSession } from '../board-session'
import { createEmptyCanvasBoardSnapshot, type CanvasBoardSnapshot } from '../../canvas-board/domain'
import { useFlowCanvasRuntime, type UseFlowCanvasRuntimeOptions } from './useFlowCanvasRuntime'
import { createFlowGraphCommandOptions } from './flowGraphAdapterCommandSideEffects'

type MutableHarnessNode = Node & {
  selected?: boolean
  dragging?: boolean
}

type MutableHarnessEdge = Edge & {
  selected?: boolean
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

function applyHarnessNodeChanges(nodes: Node[], changes: NodeChange[]): Node[] {
  for (const change of changes) {
    if (change.type === 'add') {
      if (!nodes.some((node) => node.id === change.item.id)) nodes.push(change.item as Node)
      continue
    }
    if (change.type === 'remove') {
      const index = nodes.findIndex((node) => node.id === change.id)
      if (index >= 0) nodes.splice(index, 1)
    }
  }

  for (const change of changes) {
    if (change.type === 'add' || change.type === 'remove') continue
    const node = nodes.find((item) => item.id === change.id)
    if (!node) continue
    const mutableNode = node as MutableHarnessNode
    if (change.type === 'select') {
      mutableNode.selected = change.selected
      continue
    }
    if (change.type === 'position') {
      if (change.position) mutableNode.position = change.position
      if (change.dragging !== undefined) mutableNode.dragging = change.dragging
    }
  }

  return nodes
}

function applyHarnessEdgeChanges(edges: Edge[], changes: EdgeChange[]): Edge[] {
  for (const change of changes) {
    if (change.type === 'add') {
      if (!edges.some((edge) => edge.id === change.item.id)) edges.push(change.item as Edge)
      continue
    }
    if (change.type === 'remove') {
      const index = edges.findIndex((edge) => edge.id === change.id)
      if (index >= 0) edges.splice(index, 1)
    }
  }

  for (const change of changes) {
    if (change.type !== 'select') continue
    const edge = edges.find((item) => item.id === change.id)
    if (edge) (edge as MutableHarnessEdge).selected = change.selected
  }

  return edges
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
  const scheduledAnimationFrames = new Map<number, FrameRequestCallback>()
  let nextAnimationFrameId = 1
  const onWarning = vi.fn()
  const onNativeRemove = vi.fn()
  const graphActions = {
    addNodes: vi.fn(),
    setNodes: vi.fn(),
    removeNodes: vi.fn(),
    addEdges: vi.fn(),
    setEdges: vi.fn(),
    removeEdges: vi.fn(),
    applyNodeChanges: vi.fn((changes: NodeChange[]) =>
      applyHarnessNodeChanges(nodes.value, changes),
    ),
    applyEdgeChanges: vi.fn((changes: EdgeChange[]) =>
      applyHarnessEdgeChanges(edges.value, changes),
    ),
  }
  const isDirty = ref(false)
  const saveStatus = ref<'idle' | 'saved' | 'failed'>('idle')
  const boardSession: CanvasBoardSession = {
    currentBoardId: computed(() => 'board-a'),
    exists: vi.fn(() => true),
    load: vi.fn(async () => snapshot),
    save: vi.fn(async (nextSnapshot: CanvasBoardSnapshot) => {
      savedSnapshots.push(nextSnapshot)
      return { snapshot: nextSnapshot, remoteSync: { ok: true as const } }
    }),
    refreshPreview: vi.fn(),
  }

  const runtimeOptions: UseFlowCanvasRuntimeOptions = {
    nodes,
    edges,
    boardBaseSnapshot,
    isDirty,
    saveStatus,
    isLoadingBoardSnapshot: ref(false),
    isSavingBoard: ref(false),
    boardSession,
    getViewport: () => viewport.value,
    setViewport: (nextViewport) => {
      viewport.value = nextViewport
    },
    onWarning,
    onSuccess: vi.fn(),
    onError: vi.fn(),
    onNativeRemove,
    scheduleMicrotask: (callback) => {
      scheduledMicrotasks.push(callback)
    },
    scheduleAnimationFrame: (callback) => {
      const frameId = nextAnimationFrameId
      nextAnimationFrameId += 1
      scheduledAnimationFrames.set(frameId, callback)
      return frameId
    },
    cancelAnimationFrame: (frameId) => {
      scheduledAnimationFrames.delete(frameId)
    },
    vueFlowGraphActions:
      graphActions as unknown as UseFlowCanvasRuntimeOptions['vueFlowGraphActions'],
  }
  const runtime = useFlowCanvasRuntime(runtimeOptions)

  function flushAnimationFrames() {
    const callbacks = Array.from(scheduledAnimationFrames.values())
    scheduledAnimationFrames.clear()
    callbacks.forEach((callback) => {
      callback(performance.now())
    })
  }

  async function flushNativeGraphChanges() {
    while (scheduledMicrotasks.length > 0) {
      scheduledMicrotasks.shift()?.()
      await nextTick()
    }
  }

  async function applyNativeNodeChanges(changes: Parameters<typeof runtime.onNodesChange>[0]) {
    runtime.onNodesChange(changes)
    flushAnimationFrames()
    await nextTick()
    await flushNativeGraphChanges()
  }

  async function applyNativeEdgeChanges(changes: Parameters<typeof runtime.onEdgesChange>[0]) {
    runtime.onEdgesChange(changes)
    await nextTick()
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
    onWarning,
    onNativeRemove,
    graphActions,
    isDirty,
    saveStatus,
    flushAnimationFrames,
    flushNativeGraphChanges,
    applyNativeNodeChanges,
    applyNativeEdgeChanges,
  }
}

function createPromptFlowNode(id: number, x: number, y: number): Node {
  return {
    id: `prompt-${id}`,
    type: 'canvas',
    position: { x, y },
    data: {
      kind: 'prompt',
      nodeType: 'prompt',
      title: `Prompt ${id}`,
      snapshotNode: {
        id,
        type: 'prompt',
        x,
        y,
        width: 280,
        title: `Prompt ${id}`,
        text: '',
        outputCount: 1,
        aspectRatio: 'auto',
        inputImages: [],
      },
    },
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

  it('uses graph adapter command side effects as the history and dirty boundary', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.loadInitialOrRouteBoard()
    await nextTick()
    harness.graphActions.setNodes.mockClear()

    harness.runtime.graphAdapter.addNodes(
      createPromptFlowNode(99, 720, 96),
      createFlowGraphCommandOptions(),
    )

    expect(harness.isDirty.value).toBe(true)
    expect(harness.fullNodes.value.map((node) => node.id)).toContain('prompt-99')

    harness.runtime.flowHistory.undo()

    expect(harness.graphActions.setNodes).toHaveBeenCalled()
    expect(harness.fullNodes.value.map((node) => node.id)).not.toContain('prompt-99')

    harness.runtime.flowHistory.redo()

    expect(harness.fullNodes.value.map((node) => node.id)).toContain('prompt-99')

    await harness.runtime.flushAutosave({ silent: true })

    expect(harness.savedSnapshots[0]?.promptNodes.map((node) => node.id)).toContain(99)
  })

  it('does not mark dirty for selection-only graph adapter commands without side effects', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.loadInitialOrRouteBoard()
    await nextTick()

    harness.runtime.graphAdapter.applyNodeChanges([
      { id: 'prompt-1', type: 'select', selected: true },
    ])

    expect(readNodeSelected(harness.fullNodes.value, 'prompt-1')).toBe(true)
    expect(harness.isDirty.value).toBe(false)

    await harness.runtime.flushAutosave({ silent: true })

    expect(harness.savedSnapshots).toHaveLength(0)
  })

  it('saves hidden nodes and edges from the full graph', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.loadInitialOrRouteBoard()
    await nextTick()

    harness.runtime.graphRuntime.markDirty()
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

    harness.runtime.graphRuntime.markDirty()
    await harness.runtime.flushAutosave({ silent: true })

    expect(readNodeSelected(harness.fullNodes.value, 'prompt-1')).toBe(true)
    expect(readNodeSelected(harness.nodes.value, 'prompt-1')).toBe(true)
  })

  it('preserves selection recorded in the full graph when a save commits a stabilized graph', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.loadInitialOrRouteBoard()
    await nextTick()

    harness.fullNodes.value = markFlowItemSelected(harness.fullNodes.value, 'prompt-1')

    harness.runtime.graphRuntime.markDirty()
    await harness.runtime.flushAutosave({ silent: true })

    expect(readNodeSelected(harness.fullNodes.value, 'prompt-1')).toBe(true)
    expect(readNodeSelected(harness.nodes.value, 'prompt-1')).toBe(true)
  })

  it('preserves edge selection when a save commits a stabilized graph', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.loadInitialOrRouteBoard()
    await nextTick()

    harness.edges.value = markFlowItemSelected(harness.edges.value, 'connection-3')

    harness.runtime.graphRuntime.markDirty()
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

    harness.runtime.graphRuntime.nodeDrag.begin()

    for (const x of [440, 500, 620]) {
      await harness.applyNativeNodeChanges([
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

    harness.runtime.graphRuntime.nodeDrag.commit()
    await nextTick()

    expect(harness.fullNodes.value.find((node) => node.id === 'prompt-1')?.position).toEqual({
      x: 620,
      y: 80,
    })
    expect(readNodeSelected(harness.fullNodes.value, 'prompt-1')).toBe(true)
    expect(harness.savedSnapshots).toHaveLength(0)

    harness.runtime.flowHistory.undo()

    expect(harness.fullNodes.value.find((node) => node.id === 'prompt-1')?.position).toEqual({
      x: 400,
      y: 40,
    })

    harness.runtime.flowHistory.redo()

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

    harness.runtime.graphRuntime.captureHistory()

    expect(readNodeSelected(harness.fullNodes.value, 'prompt-1')).toBe(false)
  })

  it('accepts Vue Flow select changes without marking the board dirty', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.loadInitialOrRouteBoard()
    await nextTick()

    await harness.applyNativeNodeChanges([{ id: 'prompt-1', type: 'select', selected: true }])
    await harness.applyNativeEdgeChanges([{ id: 'connection-3', type: 'select', selected: true }])

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

    await harness.applyNativeNodeChanges([
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

  it('coalesces native node drag progress through frame projection', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.loadInitialOrRouteBoard()
    await nextTick()
    harness.graphActions.applyNodeChanges.mockClear()

    harness.runtime.graphRuntime.nodeDrag.begin()
    harness.runtime.onNodesChange([
      {
        id: 'prompt-1',
        type: 'position',
        position: { x: 440, y: 80 },
        from: { x: 400, y: 40 },
        dragging: true,
      },
    ])
    harness.runtime.onNodesChange([
      {
        id: 'prompt-1',
        type: 'position',
        position: { x: 500, y: 80 },
        from: { x: 400, y: 40 },
        dragging: true,
      },
    ])
    harness.runtime.onNodesChange([
      {
        id: 'prompt-1',
        type: 'position',
        position: { x: 620, y: 80 },
        from: { x: 400, y: 40 },
        dragging: true,
      },
    ])

    expect(harness.graphActions.applyNodeChanges).not.toHaveBeenCalled()
    expect(harness.fullNodes.value.find((node) => node.id === 'prompt-1')?.position).toEqual({
      x: 400,
      y: 40,
    })

    harness.flushAnimationFrames()
    await nextTick()

    expect(harness.graphActions.applyNodeChanges).toHaveBeenCalledOnce()
    expect(harness.graphActions.applyNodeChanges.mock.calls[0]?.[0]).toEqual([
      {
        id: 'prompt-1',
        type: 'position',
        position: { x: 620, y: 80 },
        from: { x: 400, y: 40 },
        dragging: true,
      },
    ])
    expect(harness.fullNodes.value.find((node) => node.id === 'prompt-1')?.position).toEqual({
      x: 620,
      y: 80,
    })

    harness.runtime.graphRuntime.nodeDrag.commit()
    await nextTick()

    expect(harness.isDirty.value).toBe(true)
    expect(harness.savedSnapshots).toHaveLength(0)
  })

  it('cancels queued node drag progress without mutating the graph', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.loadInitialOrRouteBoard()
    await nextTick()
    harness.graphActions.applyNodeChanges.mockClear()

    harness.runtime.graphRuntime.nodeDrag.begin()
    harness.runtime.onNodesChange([
      {
        id: 'prompt-1',
        type: 'position',
        position: { x: 720, y: 96 },
        from: { x: 400, y: 40 },
        dragging: true,
      },
    ])

    harness.runtime.graphRuntime.nodeDrag.cancel()
    harness.flushAnimationFrames()
    await nextTick()

    expect(harness.graphActions.applyNodeChanges).not.toHaveBeenCalled()
    expect(harness.fullNodes.value.find((node) => node.id === 'prompt-1')?.position).toEqual({
      x: 400,
      y: 40,
    })
    expect(harness.isDirty.value).toBe(false)
  })

  it('does not clone unrelated nodes while syncing native drag position changes', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.loadInitialOrRouteBoard()
    await nextTick()

    const unrelatedNodeBefore = harness.fullNodes.value.find((node) => node.id === 'prompt-2')

    await harness.applyNativeNodeChanges([
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
    expect(harness.fullNodes.value.find((node) => node.id === 'prompt-2')).toBe(unrelatedNodeBefore)
  })

  it('does not rewrite the controlled node model when a drag transaction commits', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.loadInitialOrRouteBoard()
    await nextTick()

    harness.runtime.graphRuntime.nodeDrag.begin()

    await harness.applyNativeNodeChanges([
      {
        id: 'prompt-1',
        type: 'position',
        position: { x: 720, y: 96 },
        from: { x: 400, y: 40 },
        dragging: true,
      },
    ])
    const nodesBeforeCommit = harness.nodes.value

    harness.runtime.graphRuntime.nodeDrag.commit()
    await nextTick()

    expect(harness.fullNodes.value.find((node) => node.id === 'prompt-1')?.position).toEqual({
      x: 720,
      y: 96,
    })
    expect(harness.nodes.value).toBe(nodesBeforeCommit)
  })

  it('records native node and edge remove changes through the graph adapter', async () => {
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
    await harness.flushNativeGraphChanges()

    expect(harness.onNativeRemove).toHaveBeenCalledOnce()
    expect(harness.graphActions.applyNodeChanges).toHaveBeenCalledOnce()
    expect(harness.graphActions.applyEdgeChanges).toHaveBeenCalledOnce()
    expect(harness.fullNodes.value.map((node) => node.id)).toEqual(['prompt-1', 'prompt-2'])
    expect(harness.fullEdges.value).toHaveLength(0)

    await harness.runtime.flushAutosave({ silent: true })

    expect(harness.savedSnapshots[0]?.imageNodes).toHaveLength(0)
    expect(harness.savedSnapshots[0]?.connections).toHaveLength(0)
  })

  it('pins the Flow Pane native delete mutation path as a runtime contract', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.loadInitialOrRouteBoard()
    await nextTick()

    await harness.applyNativeNodeChanges([{ id: 'image-1', type: 'remove' }])

    expect(harness.onNativeRemove).toHaveBeenCalledOnce()
    expect(harness.fullNodes.value.map((node) => node.id)).toEqual(['prompt-1', 'prompt-2'])
    expect(harness.fullEdges.value).toHaveLength(0)
    expect(harness.isDirty.value).toBe(true)

    harness.runtime.flowHistory.undo()

    expect(harness.fullNodes.value.map((node) => node.id)).toEqual([
      'image-1',
      'prompt-1',
      'prompt-2',
    ])
    expect(harness.fullEdges.value.map((edge) => edge.id)).toEqual(['connection-3'])

    await harness.runtime.flushAutosave({ silent: true })

    expect(harness.savedSnapshots[0]?.imageNodes.map((node) => node.id)).toEqual([1])
    expect(harness.savedSnapshots[0]?.connections.map((connection) => connection.id)).toEqual([3])
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
    await harness.flushNativeGraphChanges()

    expect(harness.onNativeRemove).toHaveBeenCalledOnce()
    expect(harness.fullNodes.value.map((node) => node.id)).toEqual(['prompt-1', 'prompt-2'])
    expect(harness.fullEdges.value).toHaveLength(0)
  })

  it('records native edge disconnect as one undoable transaction', async () => {
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
    await harness.flushNativeGraphChanges()

    expect(harness.onNativeRemove).toHaveBeenCalledOnce()
    expect(harness.graphActions.applyEdgeChanges).toHaveBeenCalledOnce()
    expect(harness.fullNodes.value.map((node) => node.id)).toEqual([
      'image-1',
      'prompt-1',
      'prompt-2',
    ])
    expect(harness.fullEdges.value).toHaveLength(0)

    harness.runtime.flowHistory.undo()
    expect(harness.fullEdges.value.map((edge) => edge.id)).toEqual(['connection-3'])

    harness.runtime.flowHistory.redo()
    expect(harness.fullEdges.value).toHaveLength(0)
  })

  it('resolves native edge delete by endpoints when Vue Flow edge metadata is missing', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.loadInitialOrRouteBoard()
    await nextTick()

    const [currentEdge] = harness.fullEdges.value
    expect(currentEdge).toBeDefined()
    const generatedEdge = {
      ...currentEdge,
      id: 'vueflow__edge-image-1out:image-prompt-1in:image',
      data: undefined,
    } as Edge
    harness.fullEdges.value = [generatedEdge]

    await harness.applyNativeEdgeChanges([
      {
        id: generatedEdge.id,
        type: 'remove',
        source: generatedEdge.source,
        target: generatedEdge.target,
        sourceHandle: generatedEdge.sourceHandle ?? null,
        targetHandle: generatedEdge.targetHandle ?? null,
      },
    ])

    expect(harness.onWarning).not.toHaveBeenCalledWith(
      '删除操作未能应用，请刷新画布后重试',
    )
    expect(harness.onNativeRemove).toHaveBeenCalledOnce()
    expect(harness.fullEdges.value).toHaveLength(0)
  })

  it('coalesces native multi-node delete into one undoable transaction', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.loadInitialOrRouteBoard()
    await nextTick()

    harness.runtime.onNodesChange([
      { id: 'image-1', type: 'remove' },
      { id: 'prompt-2', type: 'remove' },
    ])
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
    await harness.flushNativeGraphChanges()

    expect(harness.onNativeRemove).toHaveBeenCalledOnce()
    expect(harness.graphActions.applyNodeChanges).toHaveBeenCalledOnce()
    expect(harness.graphActions.applyEdgeChanges).toHaveBeenCalledOnce()
    expect(harness.fullNodes.value.map((node) => node.id)).toEqual(['prompt-1'])
    expect(harness.fullEdges.value).toHaveLength(0)

    harness.runtime.flowHistory.undo()
    expect(harness.fullNodes.value.map((node) => node.id)).toEqual([
      'image-1',
      'prompt-1',
      'prompt-2',
    ])
    expect(harness.fullEdges.value.map((edge) => edge.id)).toEqual(['connection-3'])

    harness.runtime.flowHistory.redo()
    expect(harness.fullNodes.value.map((node) => node.id)).toEqual(['prompt-1'])
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
    await harness.flushNativeGraphChanges()

    harness.runtime.flowHistory.undo()

    expect(harness.fullNodes.value.map((node) => node.id)).toEqual([
      'image-1',
      'prompt-1',
      'prompt-2',
    ])
    expect(harness.fullEdges.value.map((edge) => edge.id)).toEqual(['connection-3'])

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
