import type { Edge, EdgeChange, Node, NodeChange } from '@vue-flow/core'
import { computed, nextTick, ref, shallowRef } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import type { CanvasBoardSession, SaveStatus } from '../../board-session'
import {
  assertLatestCanvasBoardState,
  createEmptyCanvasBoardSnapshot,
  type CanvasBoardSnapshot,
  type CanvasGroup,
} from '../../../canvas-board/domain'
import { CanvasGraphSession, type CanvasGraphTransaction } from '../../graph-runtime'
import { canvasBoardSnapshotToFlow } from '../../utils/flowSnapshotAdapter'
import { useCanvasGraphEditing } from '../../graph-editing'
import { useFlowCanvasRuntime, type UseFlowCanvasRuntimeOptions } from './useFlowCanvasRuntime'
import { createFlowGraphCommandOptions } from '../../vue-flow-adapter/flowGraphAdapterCommandSideEffects'
import {
  CanvasBoardGraphSession,
  collectPendingImageTasks,
  createPendingImageExecutionCore,
} from '../../execution/pending-execution'

type MutableHarnessNode = Node & {
  selected?: boolean
  dragging?: boolean
}

type MutableHarnessEdge = Edge & {
  selected?: boolean
}

const INITIAL_GROUP_GEOMETRY = {
  x: 30,
  y: 30,
  width: 660,
  height: 322,
}

const SAMPLE_GROUP_GEOMETRY = {
  x: 20,
  y: 20,
  width: 700,
  height: 420,
}

function createGroup(
  id: CanvasGroup['id'],
  nodeIds: CanvasGroup['nodeIds'],
  geometry: Pick<CanvasGroup, 'x' | 'y' | 'width' | 'height'> = SAMPLE_GROUP_GEOMETRY,
): CanvasGroup {
  return {
    id,
    nodeIds,
    ...geometry,
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

function createSingleInputReplacementSnapshot(): CanvasBoardSnapshot {
  const snapshot = createEmptyCanvasBoardSnapshot()
  snapshot.promptNodes = [
    {
      id: 1,
      type: 'prompt',
      x: 40,
      y: 40,
      width: 280,
      text: 'source prompt',
      outputCount: 1,
      aspectRatio: 'auto',
      inputImages: [],
    },
    {
      id: 2,
      type: 'prompt',
      x: 40,
      y: 420,
      width: 280,
      text: 'replacement source prompt',
      outputCount: 1,
      aspectRatio: 'auto',
      inputImages: [],
    },
  ]
  snapshot.fusionNodes = [
    {
      id: 1,
      type: 'fusion',
      x: 520,
      y: 40,
      width: 286,
      prompt: '',
      outputCount: 1,
      aspectRatio: 'auto',
      blendPoint: { x: 0.5, y: 0.5 },
      inputImages: [],
      outputStatus: 'idle',
      outputImages: [],
    },
    {
      id: 2,
      type: 'fusion',
      x: 520,
      y: 420,
      width: 286,
      prompt: '',
      outputCount: 1,
      aspectRatio: 'auto',
      blendPoint: { x: 0.5, y: 0.5 },
      inputImages: [],
      outputStatus: 'idle',
      outputImages: [],
    },
  ]
  snapshot.connections = [
    {
      id: 1,
      fromNodeType: 'prompt',
      fromNodeId: 1,
      fromPort: 'prompt',
      toNodeType: 'fusion',
      toNodeId: 1,
      toPort: 'prompt',
    },
    {
      id: 2,
      fromNodeType: 'prompt',
      fromNodeId: 2,
      fromPort: 'prompt',
      toNodeType: 'fusion',
      toNodeId: 2,
      toPort: 'prompt',
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
  const routeBoardId = ref('board-a')
  const savedSnapshots: CanvasBoardSnapshot[] = []
  const scheduledMicrotasks: Array<() => void> = []
  const onWarning = vi.fn()
  const onError = vi.fn()
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
  const saveStatus = ref<SaveStatus>('idle')
  const executionCore = createPendingImageExecutionCore()
  const terminalStates = new Map<string, { status: string; taskId?: string }>()
  const executionRuntime = {
    get attachment() {
      return { value: executionCore.currentAttachment() }
    },
    attachBoard(boardId: string) {
      return executionCore.attachBoard(
        CanvasBoardGraphSession.create(boardId, {
          graphSession: CanvasGraphSession.hydrate(boardId, createEmptyCanvasBoardSnapshot()),
        }),
      )
    },
    hydratePending(
      attachment: ReturnType<typeof executionCore.currentAttachment>,
      nodeId: string,
      taskId: string,
    ) {
      if (!attachment) return null
      const hydrated = executionCore.hydratePendingLease(attachment, {
        resultNodeId: nodeId as `image-${number}`,
        taskId,
        clientActionId: `action-${taskId}`,
      })
      return hydrated.kind === 'started' ? hydrated.lease : null
    },
    beginSubmission(
      attachment: ReturnType<typeof executionCore.currentAttachment>,
      nodeId: string,
    ) {
      if (!attachment) return null
      const started = executionCore.begin(attachment, nodeId as `image-${number}`)
      return started.kind === 'started' ? started.lease : null
    },
    getNodeState(nodeId: string) {
      const terminal = terminalStates.get(nodeId)
      if (terminal) return terminal
      const fact = executionCore.factFor(nodeId as `image-${number}`)
      return fact ? { status: fact.status, ...(fact.taskId ? { taskId: fact.taskId } : {}) } : null
    },
    getNodeLease(nodeId: string) {
      return executionCore.captureActiveLeases([nodeId])[0] ?? null
    },
    transitionNode(
      lease: { resultNodeId: `image-${number}` },
      state: { status: string; taskId?: string },
    ) {
      if (state.status === 'failed') return executionCore.fail(lease as never).kind === 'applied'
      if (state.status === 'cancelled')
        return executionCore.cancel([lease as never]).outcomes.length > 0
      if (state.status === 'running' && state.taskId)
        return (
          executionCore.markSubmitted(lease as never, state.taskId, `action-${state.taskId}`)
            .kind === 'applied'
        )
      if (state.status === 'succeeded') {
        terminalStates.set(lease.resultNodeId, { status: state.status, taskId: state.taskId })
        executionCore.release([lease as never])
        return true
      }
      return false
    },
  }
  const boardSession: CanvasBoardSession = {
    currentBoardId: computed(() => routeBoardId.value),
    exists: vi.fn(() => true),
    load: vi.fn(async () => snapshot),
    save: vi.fn(async (nextSnapshot: CanvasBoardSnapshot) => {
      assertLatestCanvasBoardState({ ...nextSnapshot, updatedAt: Date.now() }, 'board-save')
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
    executionCore,
    hydratePendingImageLeases: (snapshot) => {
      const attachment = executionCore.currentAttachment()
      if (!attachment) return
      collectPendingImageTasks(snapshot ?? boardBaseSnapshot.value).forEach((pending) => {
        executionCore.hydratePendingLease(attachment, pending)
      })
    },
    cancelTasks: vi.fn(),
    onWarning,
    onSuccess: vi.fn(),
    onError,
    scheduleMicrotask: (callback) => {
      scheduledMicrotasks.push(callback)
    },
    vueFlowGraphActions:
      graphActions as unknown as UseFlowCanvasRuntimeOptions['vueFlowGraphActions'],
  }
  const runtime = useFlowCanvasRuntime(runtimeOptions)

  async function flushNativeGraphChanges() {
    while (scheduledMicrotasks.length > 0) {
      scheduledMicrotasks.shift()?.()
      await nextTick()
    }
  }

  async function applyNativeNodeChanges(
    changes: Parameters<typeof runtime.flowPane.onNodesChange>[0],
  ) {
    runtime.flowPane.onNodesChange(changes)
    await nextTick()
    await flushNativeGraphChanges()
  }

  async function applyNativeEdgeChanges(
    changes: Parameters<typeof runtime.flowPane.onEdgesChange>[0],
  ) {
    runtime.flowPane.onEdgesChange(changes)
    await nextTick()
    await flushNativeGraphChanges()
  }

  return {
    runtime,
    nodes,
    edges,
    renderEdges: runtime.flowPane.renderEdges,
    fullNodes,
    fullEdges,
    boardBaseSnapshot,
    savedSnapshots,
    boardSession,
    routeBoardId,
    onWarning,
    onError,
    graphActions,
    isDirty,
    saveStatus,
    executionCore,
    executionRuntime,
    flushNativeGraphChanges,
    applyNativeNodeChanges,
    applyNativeEdgeChanges,
  }
}

function createCanvasGraphRuntimeHarness(harness: ReturnType<typeof createRuntimeHarness>) {
  const graphFeedback = {
    operationSucceeded: vi.fn(),
    operationWarning: vi.fn((message: string) => {
      harness.onWarning(message)
    }),
    operationLoading: vi.fn(),
  }
  const editing = useCanvasGraphEditing({
    nodes: harness.fullNodes,
    edges: harness.fullEdges,
    boardBaseSnapshot: harness.boardBaseSnapshot,
    currentBoardId: harness.runtime.persistence.currentBoardId,
    graphAdapter: harness.runtime.graphEditing.adapter,
    readCanvasGraphStore: harness.runtime.graphEditing.readCanvasGraphStore,
    deleteNode: harness.runtime.graphEditing.deleteNode,
    updateNodeInternals: vi.fn(),
    screenToFlowCoordinate: (point) => point,
    browser: {
      pickImageFile: vi.fn(async () => null),
      readDroppedFile: vi.fn(() => null),
      readDropScreenPoint: vi.fn(() => ({ x: 0, y: 0 })),
      markDropAsCopy: vi.fn(),
    },
    upload: {
      upload: vi.fn(async () => ({
        url: 'https://example.com/upload.png',
        asset_id: 'asset_42',
      })),
    },
    materializeLibraryAsset: vi.fn(async () => ({
      mediaAssetId: 43,
      url: 'https://example.com/materialized.png',
      width: 200,
      height: 100,
    })),
    readLibraryImportAttachment: () => harness.executionRuntime.attachment.value,
    selectOnlyNode: vi.fn(() => true),
    preview: {
      openImageUrl: vi.fn(async () => {}),
    },
    feedback: graphFeedback,
  })
  const nodeInteractions = editing.commands.createNodeInteractions({
    dismissTransientCanvasUi: vi.fn(),
    discardNodeEdits: vi.fn(),
  })
  const graphRuntime = {
    connectionAdapter: editing.flowPane.connectionAdapter,
    onFlowDeleteNode: nodeInteractions.onFlowDeleteNode,
  }

  return { graphRuntime, graphFeedback }
}

function clearGraphActionMocks(harness: ReturnType<typeof createRuntimeHarness>) {
  Object.values(harness.graphActions).forEach((action) => {
    action.mockClear()
  })
}

function exportCurrentBoardSnapshot(harness: ReturnType<typeof createRuntimeHarness>) {
  return harness.runtime.graphEditing
    .readCanvasGraphStore()
    .exportSnapshot(harness.boardBaseSnapshot.value)
}

function projectCanonicalGraph(
  harness: ReturnType<typeof createRuntimeHarness>,
  commandOptions = createFlowGraphCommandOptions({ recordHistory: false, markDirty: false }),
) {
  const snapshot = exportCurrentBoardSnapshot(harness)
  harness.boardBaseSnapshot.value = snapshot
  const flow = canvasBoardSnapshotToFlow(snapshot)
  harness.runtime.graphEditing.adapter.replaceGraph(
    { nodes: flow.nodes as Node[], edges: flow.edges as Edge[] },
    commandOptions,
  )
}

function commitCanonicalGraphMutation(
  harness: ReturnType<typeof createRuntimeHarness>,
  reason: Parameters<
    ReturnType<typeof harness.runtime.graphEditing.readCanvasGraphStore>['beginTransaction']
  >[0],
  mutate: (transaction: CanvasGraphTransaction) => void,
) {
  const transaction = harness.runtime.graphEditing.readCanvasGraphStore().beginTransaction(reason, {
    recordHistory: false,
    markDirty: false,
  })
  mutate(transaction)
  const summary = transaction.commit()
  if (summary.didChange) projectCanonicalGraph(harness)
  return summary
}

function readEdgeProjection(edges: Edge[]) {
  return edges.map((edge) => ({
    id: edge.id,
    source: edge.source,
    target: edge.target,
    sourceHandle: edge.sourceHandle,
    targetHandle: edge.targetHandle,
    data: edge.data,
  }))
}

function readSelectedFlag(node: Node | undefined) {
  return (node as (Node & { selected?: boolean }) | undefined)?.selected
}

function markFlowItemSelected<T extends Node | Edge>(items: T[], itemId: string): T[] {
  return items.map((item) => (item.id === itemId ? { ...item, selected: true } : item))
}

function selectFlowNodes(nodes: Node[], nodeIds: string[]): Node[] {
  const selectedIds = new Set(nodeIds)
  return nodes.map((node) => ({ ...node, selected: selectedIds.has(node.id) }))
}

function readNodeSelected(nodes: Node[], nodeId: string) {
  return readSelectedFlag(nodes.find((node) => node.id === nodeId))
}

function readNodePosition(nodes: Node[], nodeId: string) {
  const node = nodes.find((item) => item.id === nodeId)
  return node ? { ...node.position } : null
}

function readEdgeSelected(edges: Edge[], edgeId: string) {
  return (edges.find((edge) => edge.id === edgeId) as (Edge & { selected?: boolean }) | undefined)
    ?.selected
}

function clickPaneThroughGrouping(runtime: ReturnType<typeof useFlowCanvasRuntime>) {
  const selectPane = vi.fn()
  runtime.graphEditing.grouping.commands.handlePaneClick(selectPane)
  expect(selectPane).toHaveBeenCalledOnce()
}

function clickNodeThroughGrouping(
  harness: ReturnType<typeof createRuntimeHarness>,
  nodeId: string,
) {
  const node = harness.fullNodes.value.find((item) => item.id === nodeId)
  expect(node).toBeDefined()
  const selectNode = vi.fn()
  harness.runtime.graphEditing.grouping.commands.handleNodeClick({
    node: node as Node,
    selectNode,
  })
  expect(selectNode).toHaveBeenCalledOnce()
}

describe('useFlowCanvasRuntime', () => {
  it('hydrates the full graph into the Vue Flow store', async () => {
    const harness = createRuntimeHarness()
    const hydrateSession = vi.spyOn(CanvasGraphSession, 'hydrate')

    await harness.runtime.persistence.loadInitialOrRouteBoard()

    expect(harness.fullNodes.value.map((node) => node.id)).toEqual([
      'image-1',
      'prompt-1',
      'prompt-2',
    ])
    expect(harness.fullEdges.value.map((edge) => edge.id)).toEqual(['connection-3'])
    expect(harness.nodes.value.map((node) => node.id)).toEqual(['image-1', 'prompt-1', 'prompt-2'])
    expect(harness.edges.value.map((edge) => edge.id)).toEqual(['connection-3'])
    expect(hydrateSession).toHaveBeenCalledTimes(1)
    hydrateSession.mockRestore()
  })

  it('replaces the Graph Session and advances the attachment epoch when the board reloads', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.persistence.loadInitialOrRouteBoard()

    const firstStore = harness.runtime.graphEditing.readCanvasGraphStore()
    const firstAttachment = harness.executionCore.currentAttachment()

    await harness.runtime.persistence.loadInitialOrRouteBoard()

    const nextStore = harness.runtime.graphEditing.readCanvasGraphStore()
    const nextAttachment = harness.executionCore.currentAttachment()
    expect(nextStore).not.toBe(firstStore)
    expect(nextAttachment?.epoch).toBeGreaterThan(firstAttachment?.epoch ?? -1)
    firstStore.beginTransaction('node-update').updateNode('prompt-1', { x: 999 }).commit()
    expect(nextStore.exportSnapshot().promptNodes.find((node) => node.id === 1)?.x).toBe(400)
  })

  it('detaches Board A before a deferred Board B load so autosave cannot persist A into B', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    const attachmentA = harness.executionCore.currentAttachment()
    let resolveBoardB!: (snapshot: CanvasBoardSnapshot) => void
    vi.mocked(harness.boardSession.load).mockImplementationOnce(
      () =>
        new Promise<CanvasBoardSnapshot>((resolve) => {
          resolveBoardB = resolve
        }),
    )

    harness.routeBoardId.value = 'board-b'
    const loadingBoardB = harness.runtime.persistence.loadInitialOrRouteBoard()
    await Promise.resolve()

    expect(harness.executionCore.isAttachmentCurrent(attachmentA!)).toBe(false)
    harness.isDirty.value = true
    await harness.runtime.persistence.flushAutosave()
    expect(harness.boardSession.save).not.toHaveBeenCalled()

    resolveBoardB(createEmptyCanvasBoardSnapshot())
    await loadingBoardB

    expect(harness.executionCore.currentAttachment()?.boardId).toBe('board-b')
  })

  it('hydrates persisted pending tasks into Runtime before the first graph projection', async () => {
    const snapshot = createSnapshot()
    snapshot.imageNodes[0] = {
      ...snapshot.imageNodes[0]!,
      sourceImageUrl: '',
      outputStatus: 'running',
      pendingTaskId: 'task-1',
      pendingClientActionId: 'action-task-1',
      outputImages: [],
    }
    snapshot.connections = [
      {
        id: 4,
        fromNodeType: 'prompt',
        fromNodeId: 1,
        fromPort: 'prompt',
        toNodeType: 'image',
        toNodeId: 1,
        toPort: 'image',
      },
    ]
    const harness = createRuntimeHarness(snapshot)

    await harness.runtime.persistence.loadInitialOrRouteBoard()

    expect(harness.executionRuntime.getNodeState('image-1')).toEqual({
      status: 'running',
      taskId: 'task-1',
    })
    expect(harness.renderEdges.value).toEqual([
      expect.objectContaining({ id: 'connection-4', animated: true }),
    ])
  })

  it('keeps runtime edge projection active when a deferred save reconciles', async () => {
    const snapshot = createSnapshot()
    snapshot.connections = [
      {
        id: 4,
        fromNodeType: 'prompt',
        fromNodeId: 1,
        fromPort: 'prompt',
        toNodeType: 'image',
        toNodeId: 1,
        toPort: 'image',
      },
    ]
    const harness = createRuntimeHarness(snapshot)
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    const attachment = harness.executionRuntime.attachment.value!
    const lease = harness.executionRuntime.hydratePending(attachment, 'image-1', 'task-1')!
    expect(harness.renderEdges.value[0]?.animated).toBe(true)

    let resolveSave!: () => void
    vi.mocked(harness.boardSession.save).mockImplementationOnce(
      (snapshot) =>
        new Promise((resolve) => {
          resolveSave = () => resolve({ snapshot, remoteSync: { ok: true as const } })
        }),
    )
    harness.isDirty.value = true
    const save = harness.runtime.persistence.flushAutosave({ silent: true })
    await vi.waitFor(() => expect(harness.boardSession.save).toHaveBeenCalled())
    resolveSave()
    await save

    expect(harness.executionRuntime.getNodeLease('image-1')).toBe(lease)
    expect(harness.renderEdges.value[0]?.animated).toBe(true)
    expect(JSON.stringify(vi.mocked(harness.boardSession.save).mock.calls[0]?.[0])).not.toContain(
      'animated',
    )
  })

  it('does not revive stale animation when execution finishes during a deferred save', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    harness.runtime.graphEditing.adapter.replaceEdges(
      [
        {
          id: 'connection-4',
          source: 'prompt-1',
          sourceHandle: 'out:prompt',
          target: 'image-1',
          targetHandle: 'in:image',
          animated: false,
        },
      ],
      createFlowGraphCommandOptions(),
    )
    harness.isDirty.value = true
    const attachment = harness.executionRuntime.attachment.value!
    const lease = harness.executionRuntime.hydratePending(attachment, 'image-1', 'task-1')!
    expect(harness.renderEdges.value[0]?.animated).toBe(true)

    let resolveSave!: () => void
    vi.mocked(harness.boardSession.save).mockImplementationOnce(
      (snapshot) =>
        new Promise((resolve) => {
          resolveSave = () => resolve({ snapshot, remoteSync: { ok: true as const } })
        }),
    )
    const save = harness.runtime.persistence.flushAutosave({ silent: true })
    await vi.waitFor(() => expect(harness.boardSession.save).toHaveBeenCalled())
    vi.mocked(harness.graphActions.setEdges).mockClear()
    expect(
      harness.executionRuntime.transitionNode(lease, {
        status: 'succeeded',
        taskId: 'task-1',
      }),
    ).toBe(true)
    expect(harness.renderEdges.value[0]?.animated).toBe(false)
    expect(harness.graphActions.setEdges).toHaveBeenCalled()
    const lastPresentedEdges = vi.mocked(harness.graphActions.setEdges).mock.lastCall?.[0] ?? []
    expect(lastPresentedEdges.find((edge: Edge) => edge.id === 'connection-4')?.animated).toBe(
      false,
    )
    resolveSave()
    await save

    expect(harness.executionRuntime.getNodeState('image-1')).toEqual({
      status: 'succeeded',
      taskId: 'task-1',
    })
    expect(harness.renderEdges.value[0]?.animated).toBe(false)
  })

  it('preserves canonical graph data during save reconciliation', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    const imageTemplate = exportCurrentBoardSnapshot(harness).imageNodes[0]!
    commitCanonicalGraphMutation(harness, 'node-update', (transaction) => {
      for (const id of [9, 10]) {
        transaction.addNode({
          ...imageTemplate,
          id,
          sourceImageUrl: '',
          outputStatus: 'running',
          outputImages: [],
          pendingTaskId: undefined,
        })
      }
      transaction.addEdge({
        id: 4,
        fromNodeType: 'prompt',
        fromNodeId: 1,
        fromPort: 'prompt',
        toNodeType: 'image',
        toNodeId: 9,
        toPort: 'image',
      })
      transaction.addEdge({
        id: 5,
        fromNodeType: 'prompt',
        fromNodeId: 1,
        fromPort: 'prompt',
        toNodeType: 'image',
        toNodeId: 10,
        toPort: 'image',
      })
    })
    const attachment = harness.executionRuntime.attachment.value!
    harness.executionRuntime.beginSubmission(attachment, 'image-9')

    await harness.runtime.persistence.flushAutosave({ silent: true })

    expect(harness.nodes.value.map((node) => node.id)).toContain('image-9')
    expect(harness.nodes.value.map((node) => node.id)).toContain('image-10')
    expect(harness.edges.value.map((edge) => edge.id)).toContain('connection-4')
    expect(harness.edges.value.map((edge) => edge.id)).toContain('connection-5')
    expect(harness.renderEdges.value.find((edge) => edge.id === 'connection-4')?.animated).toBe(
      true,
    )
  })

  it('reconciles a persisted pending edge by endpoints instead of retaining both edge ids', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    const imageTemplate = exportCurrentBoardSnapshot(harness).imageNodes[0]!
    commitCanonicalGraphMutation(harness, 'node-update', (transaction) => {
      transaction.addNode({
        ...imageTemplate,
        id: 9,
        sourceImageUrl: '',
        outputStatus: 'running',
        outputImages: [],
        pendingTaskId: 'task-9',
        pendingClientActionId: 'action-task-9',
      })
      transaction.addEdge({
        id: 4,
        fromNodeType: 'prompt',
        fromNodeId: 1,
        fromPort: 'prompt',
        toNodeType: 'image',
        toNodeId: 9,
        toPort: 'image',
      })
    })
    const attachment = harness.executionRuntime.attachment.value!
    harness.executionRuntime.hydratePending(attachment, 'image-9', 'task-9')

    await harness.runtime.persistence.flushAutosave({ silent: true })

    const reconciledEdges = harness.renderEdges.value.filter(
      (edge) => edge.source === 'prompt-1' && edge.target === 'image-9',
    )
    expect(reconciledEdges).toHaveLength(1)
    expect(reconciledEdges[0]?.id).toBe('connection-4')
    expect(reconciledEdges[0]?.animated).toBe(true)
    expect(harness.renderEdges.value).toEqual(
      expect.arrayContaining([expect.objectContaining({ id: 'connection-4', animated: true })]),
    )
  })

  it('uses canonical graph transactions as the history and dirty boundary', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()
    harness.graphActions.setNodes.mockClear()

    const transaction = harness.runtime.graphEditing
      .readCanvasGraphStore()
      .beginTransaction('node-update', { recordHistory: true, markDirty: true })
    transaction.addNode({
      id: 99,
      type: 'prompt',
      x: 720,
      y: 96,
      width: 280,
      title: 'Prompt 99',
      text: '',
      outputCount: 1,
      aspectRatio: 'auto',
      inputImages: [],
    })
    transaction.commit()

    expect(harness.isDirty.value).toBe(true)
    expect(harness.saveStatus.value).toBe('pending')
    expect(harness.fullNodes.value.map((node) => node.id)).toContain('prompt-99')

    harness.runtime.persistence.history.undo()

    expect(harness.graphActions.setNodes).toHaveBeenCalled()
    expect(harness.fullNodes.value.map((node) => node.id)).not.toContain('prompt-99')
    expect(exportCurrentBoardSnapshot(harness).promptNodes.map((node) => node.id)).not.toContain(99)

    harness.runtime.persistence.history.redo()

    expect(harness.fullNodes.value.map((node) => node.id)).toContain('prompt-99')
    expect(exportCurrentBoardSnapshot(harness).promptNodes.map((node) => node.id)).toContain(99)

    await harness.runtime.persistence.flushAutosave({ silent: true })

    expect(harness.savedSnapshots[0]?.promptNodes.map((node) => node.id)).toContain(99)
  })

  it('pins valid connection mutations through Canvas state, Vue Flow projection, history, and autosave', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()
    const { graphRuntime } = createCanvasGraphRuntimeHarness(harness)
    clearGraphActionMocks(harness)

    graphRuntime.connectionAdapter.vueFlowBindings.onConnect({
      source: 'image-1',
      target: 'prompt-2',
      sourceHandle: 'out:image',
      targetHandle: 'in:image',
    })

    expect(harness.onWarning).not.toHaveBeenCalled()
    expect(exportCurrentBoardSnapshot(harness).connections).toEqual([
      {
        id: 3,
        fromNodeType: 'image',
        fromNodeId: 1,
        fromPort: 'image',
        toNodeType: 'prompt',
        toNodeId: 1,
        toPort: 'image',
      },
      {
        id: 4,
        fromNodeType: 'image',
        fromNodeId: 1,
        fromPort: 'image',
        toNodeType: 'prompt',
        toNodeId: 2,
        toPort: 'image',
      },
    ])
    expect(harness.fullEdges.value.map((edge) => edge.id)).toEqual(['connection-3', 'connection-4'])
    expect(harness.fullEdges.value[1]).toMatchObject({
      id: 'connection-4',
      source: 'image-1',
      target: 'prompt-2',
      sourceHandle: 'out:image',
      targetHandle: 'in:image',
      data: {
        connectionSnapshot: {
          id: 4,
          fromNodeType: 'image',
          fromNodeId: 1,
          fromPort: 'image',
          toNodeType: 'prompt',
          toNodeId: 2,
          toPort: 'image',
        },
      },
    })
    expect(harness.graphActions.setEdges).toHaveBeenCalledWith(
      expect.arrayContaining([expect.objectContaining({ id: 'connection-4' })]),
    )
    expect(harness.isDirty.value).toBe(true)
    expect(harness.saveStatus.value).toBe('pending')
    expect(harness.savedSnapshots).toHaveLength(0)

    harness.runtime.persistence.history.undo()
    expect(harness.fullEdges.value.map((edge) => edge.id)).toEqual(['connection-3'])

    harness.runtime.persistence.history.redo()
    expect(harness.fullEdges.value.map((edge) => edge.id)).toEqual(['connection-3', 'connection-4'])

    await harness.runtime.persistence.flushAutosave({ silent: true })

    expect(harness.savedSnapshots[0]?.connections.map((connection) => connection.id)).toEqual([
      3, 4,
    ])
    expect(harness.savedSnapshots[0]?.connections[1]).toMatchObject({
      fromNodeType: 'image',
      fromNodeId: 1,
      toNodeType: 'prompt',
      toNodeId: 2,
    })
  })

  it('replaces stale duplicate projection edges with one single-input connection', async () => {
    const snapshot = createSingleInputReplacementSnapshot()
    snapshot.promptNodes.push({
      ...snapshot.promptNodes[1]!,
      id: 3,
      text: 'new source prompt',
      y: 760,
    })
    snapshot.fusionNodes = [snapshot.fusionNodes[0]!]
    snapshot.connections = [snapshot.connections[0]!]
    const harness = createRuntimeHarness(snapshot)
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()
    const { graphRuntime } = createCanvasGraphRuntimeHarness(harness)
    const staleEdge = {
      id: 'vueflow__edge-prompt-2out:prompt-fusion-1in:prompt',
      source: 'prompt-2',
      target: 'fusion-1',
      sourceHandle: 'out:prompt',
      targetHandle: 'in:prompt',
      data: undefined,
    } as Edge
    harness.fullEdges.value = [staleEdge, ...harness.fullEdges.value]
    clearGraphActionMocks(harness)

    graphRuntime.connectionAdapter.vueFlowBindings.onConnect({
      source: 'prompt-3',
      target: 'fusion-1',
      sourceHandle: 'out:prompt',
      targetHandle: 'in:prompt',
    })

    expect(harness.onWarning).not.toHaveBeenCalled()
    expect(harness.fullEdges.value.map((edge) => edge.id)).toEqual(['connection-2'])
    expect(exportCurrentBoardSnapshot(harness).connections).toEqual([
      {
        id: 2,
        fromNodeType: 'prompt',
        fromNodeId: 3,
        fromPort: 'prompt',
        toNodeType: 'fusion',
        toNodeId: 1,
        toPort: 'prompt',
      },
    ])
    expect(harness.graphActions.removeEdges).toHaveBeenCalled()
    expect(harness.isDirty.value).toBe(true)
    expect(harness.saveStatus.value).toBe('pending')
    expect(harness.savedSnapshots).toHaveLength(0)

    harness.runtime.persistence.history.undo()
    expect(harness.fullEdges.value.map((edge) => edge.id)).toEqual(['connection-1'])
    expect(
      exportCurrentBoardSnapshot(harness).connections.map((connection) => connection.id),
    ).toEqual([1])

    harness.runtime.persistence.history.redo()
    expect(harness.fullEdges.value.map((edge) => edge.id)).toEqual(['connection-2'])

    await harness.runtime.persistence.flushAutosave({ silent: true })
    expect(harness.savedSnapshots[0]?.connections.map((connection) => connection.id)).toEqual([2])
  })

  it('replaces a trailing stale projection edge when its generated id is reused', async () => {
    const snapshot = createSingleInputReplacementSnapshot()
    snapshot.promptNodes.push({
      ...snapshot.promptNodes[1]!,
      id: 3,
      text: 'new source prompt',
      y: 760,
    })
    snapshot.fusionNodes = [snapshot.fusionNodes[0]!]
    snapshot.connections = [snapshot.connections[0]!]
    const harness = createRuntimeHarness(snapshot)
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()
    const { graphRuntime } = createCanvasGraphRuntimeHarness(harness)
    const staleEdge = {
      id: 'vueflow__edge-prompt-2out:prompt-fusion-1in:prompt',
      source: 'prompt-2',
      target: 'fusion-1',
      sourceHandle: 'out:prompt',
      targetHandle: 'in:prompt',
      data: undefined,
    } as Edge
    harness.fullEdges.value = [...harness.fullEdges.value, staleEdge]
    clearGraphActionMocks(harness)

    graphRuntime.connectionAdapter.vueFlowBindings.onConnect({
      source: 'prompt-3',
      target: 'fusion-1',
      sourceHandle: 'out:prompt',
      targetHandle: 'in:prompt',
    })

    expect(harness.onWarning).not.toHaveBeenCalled()
    expect(harness.fullEdges.value).toHaveLength(1)
    expect(harness.fullEdges.value[0]).toMatchObject({
      id: 'connection-2',
      source: 'prompt-3',
      target: 'fusion-1',
    })
    expect(exportCurrentBoardSnapshot(harness).connections).toEqual([
      {
        id: 2,
        fromNodeType: 'prompt',
        fromNodeId: 3,
        fromPort: 'prompt',
        toNodeType: 'fusion',
        toNodeId: 1,
        toPort: 'prompt',
      },
    ])
    expect(harness.isDirty.value).toBe(true)
    expect(harness.saveStatus.value).toBe('pending')
    expect(harness.savedSnapshots).toHaveLength(0)

    harness.runtime.persistence.history.undo()
    expect(harness.fullEdges.value.map((edge) => edge.id)).toEqual(['connection-1'])
    expect(exportCurrentBoardSnapshot(harness).connections).toEqual([
      expect.objectContaining({ id: 1, fromNodeId: 1 }),
    ])

    harness.runtime.persistence.history.redo()
    expect(harness.fullEdges.value).toHaveLength(1)
    expect(harness.fullEdges.value[0]).toMatchObject({
      id: 'connection-2',
      source: 'prompt-3',
    })

    await harness.runtime.persistence.flushAutosave({ silent: true })
    expect(harness.savedSnapshots[0]?.connections).toEqual([
      expect.objectContaining({ id: 2, fromNodeId: 3 }),
    ])
  })

  it('leaves connection state unchanged when stale projection endpoints are corrupt', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()
    const { graphRuntime } = createCanvasGraphRuntimeHarness(harness)
    const corruptEdge = {
      id: 'vueflow__edge-corrupt',
      source: 'external-source',
      target: 'external-target',
      sourceHandle: null,
      targetHandle: null,
      data: undefined,
    } as Edge
    harness.fullEdges.value = [corruptEdge, ...harness.fullEdges.value]
    clearGraphActionMocks(harness)
    const projectionBefore = readEdgeProjection(harness.fullEdges.value)
    const snapshotBefore = exportCurrentBoardSnapshot(harness)
    const saveStatusBefore = harness.saveStatus.value

    graphRuntime.connectionAdapter.vueFlowBindings.onConnect({
      source: 'image-1',
      target: 'prompt-2',
      sourceHandle: 'out:image',
      targetHandle: 'in:image',
    })

    expect(harness.onWarning).toHaveBeenCalledWith(
      'Failed to write the connection to the canvas. Refresh and try again.',
    )
    expect(readEdgeProjection(harness.fullEdges.value)).toEqual(projectionBefore)
    expect(exportCurrentBoardSnapshot(harness)).toEqual(snapshotBefore)
    expect(harness.graphActions.addEdges).not.toHaveBeenCalled()
    expect(harness.graphActions.setEdges).not.toHaveBeenCalled()
    expect(harness.isDirty.value).toBe(false)
    expect(harness.saveStatus.value).toBe(saveStatusBefore)
    expect(harness.savedSnapshots).toHaveLength(0)
  })

  it('records board groups in the shared undo and redo history', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    const transaction = harness.runtime.graphEditing
      .readCanvasGraphStore()
      .beginTransaction('group', {
        recordHistory: true,
        markDirty: true,
      })
    transaction.createGroup(['image-1', 'prompt-1'], SAMPLE_GROUP_GEOMETRY, 'group-1')
    transaction.commit()

    harness.runtime.persistence.history.undo()
    expect(harness.boardBaseSnapshot.value.groups).toEqual([])

    harness.runtime.persistence.history.redo()
    expect(harness.boardBaseSnapshot.value.groups).toEqual([
      createGroup('group-1', ['image-1', 'prompt-1']),
    ])

    await harness.runtime.persistence.flushAutosave({ silent: true })

    expect(harness.savedSnapshots[0]?.groups).toEqual([
      createGroup('group-1', ['image-1', 'prompt-1']),
    ])
  })

  it('exposes a group action for two or more selected ungrouped nodes', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    expect(harness.runtime.graphEditing.grouping.view.selectionGroupAction.value).toBeNull()

    harness.fullNodes.value = selectFlowNodes(harness.fullNodes.value, ['image-1'])
    expect(harness.runtime.graphEditing.grouping.view.selectionGroupAction.value).toBeNull()

    harness.fullNodes.value = selectFlowNodes(harness.fullNodes.value, ['image-1', 'prompt-1'])
    expect(harness.runtime.graphEditing.grouping.view.selectionGroupAction.value).toEqual({
      kind: 'group',
      label: 'Group',
      nodeIds: ['image-1', 'prompt-1'],
    })

    harness.fullEdges.value = markFlowItemSelected(harness.fullEdges.value, 'connection-3')
    expect(harness.runtime.graphEditing.grouping.view.selectionGroupAction.value).toBeNull()
  })

  it('hides the group action for nodes that are already partly grouped', async () => {
    const snapshot = createSnapshot()
    snapshot.groups = [createGroup('group-1', ['image-1', 'prompt-1'])]
    const harness = createRuntimeHarness(snapshot)
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    harness.fullNodes.value = selectFlowNodes(harness.fullNodes.value, ['image-1', 'prompt-2'])

    expect(harness.runtime.graphEditing.grouping.view.selectionGroupAction.value).toBeNull()
  })

  it('groups selected nodes into a fixed selected frame and participates in undo and redo', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()
    harness.fullNodes.value = selectFlowNodes(harness.fullNodes.value, ['image-1', 'prompt-1'])
    const historyPush = vi.spyOn(harness.runtime.persistence.history, 'record')

    expect(harness.runtime.graphEditing.grouping.commands.groupSelectedNodes()).toBe(true)

    expect(harness.boardBaseSnapshot.value.groups).toEqual([
      createGroup('group-1', ['image-1', 'prompt-1'], INITIAL_GROUP_GEOMETRY),
    ])
    expect(harness.runtime.graphEditing.grouping.view.selectionGroupAction.value).toEqual({
      kind: 'ungroup',
      label: 'Ungroup',
      groupId: 'group-1',
      nodeIds: ['image-1', 'prompt-1'],
    })
    expect(readNodeSelected(harness.fullNodes.value, 'image-1')).toBe(false)
    expect(readNodeSelected(harness.fullNodes.value, 'prompt-1')).toBe(false)
    expect(harness.runtime.graphEditing.grouping.internal.selectedGroupId.value).toBe('group-1')
    expect(harness.runtime.graphEditing.grouping.view.groupFrames.value[0]).toMatchObject({
      id: 'group-1',
      bounds: {
        left: INITIAL_GROUP_GEOMETRY.x,
        top: INITIAL_GROUP_GEOMETRY.y,
        width: INITIAL_GROUP_GEOMETRY.width,
        height: INITIAL_GROUP_GEOMETRY.height,
      },
      selected: true,
    })
    expect(harness.isDirty.value).toBe(true)
    expect(historyPush).toHaveBeenCalledOnce()
    expect(harness.saveStatus.value).toBe('pending')

    harness.fullNodes.value = selectFlowNodes(harness.fullNodes.value, [])

    expect(harness.runtime.graphEditing.grouping.internal.selectedGroupId.value).toBe('group-1')
    expect(harness.runtime.graphEditing.grouping.view.selectionGroupAction.value).toEqual({
      kind: 'ungroup',
      label: 'Ungroup',
      groupId: 'group-1',
      nodeIds: ['image-1', 'prompt-1'],
    })

    harness.runtime.persistence.history.undo()
    expect(harness.boardBaseSnapshot.value.groups).toEqual([])

    harness.runtime.persistence.history.redo()
    expect(harness.boardBaseSnapshot.value.groups).toEqual([
      createGroup('group-1', ['image-1', 'prompt-1'], INITIAL_GROUP_GEOMETRY),
    ])

    await harness.runtime.persistence.flushAutosave({ silent: true })

    expect(harness.savedSnapshots).toHaveLength(1)
    expect(harness.savedSnapshots[0]?.groups).toEqual([
      createGroup('group-1', ['image-1', 'prompt-1'], INITIAL_GROUP_GEOMETRY),
    ])
  })

  it('clears group selection when Vue Flow selects a node', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()
    harness.fullNodes.value = selectFlowNodes(harness.fullNodes.value, ['image-1', 'prompt-1'])

    expect(harness.runtime.graphEditing.grouping.commands.groupSelectedNodes()).toBe(true)
    expect(harness.runtime.graphEditing.grouping.internal.selectedGroupId.value).toBe('group-1')

    await harness.applyNativeNodeChanges([{ id: 'prompt-2', type: 'select', selected: true }])

    expect(harness.runtime.graphEditing.grouping.internal.selectedGroupId.value).toBeNull()
    expect(harness.runtime.graphEditing.grouping.view.groupFrames.value[0]?.selected).toBe(false)
  })

  it('saves groups that reference decimal canvas node ids from active drafts', async () => {
    const snapshot = createEmptyCanvasBoardSnapshot()
    snapshot.promptNodes = [
      {
        id: 1000.25,
        type: 'prompt',
        x: 40,
        y: 40,
        width: 280,
        text: 'first draft node',
        outputCount: 1,
        aspectRatio: 'auto',
        inputImages: [],
      },
      {
        id: 1001.5,
        type: 'prompt',
        x: 400,
        y: 40,
        width: 280,
        text: 'second draft node',
        outputCount: 1,
        aspectRatio: 'auto',
        inputImages: [],
      },
    ]
    const harness = createRuntimeHarness(snapshot)
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()
    harness.fullNodes.value = selectFlowNodes(harness.fullNodes.value, [
      'prompt-1000.25',
      'prompt-1001.5',
    ])

    expect(harness.runtime.graphEditing.grouping.commands.groupSelectedNodes()).toBe(true)
    await harness.runtime.persistence.flushAutosave({ silent: true })

    expect(harness.onError).not.toHaveBeenCalled()
    expect(harness.savedSnapshots[0]?.groups).toEqual([
      createGroup('group-1', ['prompt-1000.25', 'prompt-1001.5'], INITIAL_GROUP_GEOMETRY),
    ])
  })

  it('ungroups selected group members without deleting or moving nodes', async () => {
    const snapshot = createSnapshot()
    snapshot.groups = [createGroup('group-1', ['image-1', 'prompt-1'])]
    const harness = createRuntimeHarness(snapshot)
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()
    harness.fullNodes.value = selectFlowNodes(harness.fullNodes.value, ['image-1', 'prompt-1'])
    const historyPush = vi.spyOn(harness.runtime.persistence.history, 'record')

    expect(harness.runtime.graphEditing.grouping.commands.ungroupSelectedGroup()).toBe(true)

    expect(harness.boardBaseSnapshot.value.groups).toEqual([])
    expect(harness.fullNodes.value.map((node) => node.id)).toEqual([
      'image-1',
      'prompt-1',
      'prompt-2',
    ])
    expect(harness.fullEdges.value.map((edge) => edge.id)).toEqual(['connection-3'])
    expect(historyPush).toHaveBeenCalledOnce()
    expect(harness.isDirty.value).toBe(true)
    expect(harness.saveStatus.value).toBe('pending')

    harness.runtime.persistence.history.undo()
    expect(harness.boardBaseSnapshot.value.groups).toEqual([
      createGroup('group-1', ['image-1', 'prompt-1']),
    ])

    harness.runtime.persistence.history.redo()
    expect(harness.boardBaseSnapshot.value.groups).toEqual([])

    await harness.runtime.persistence.flushAutosave({ silent: true })

    expect(harness.savedSnapshots).toHaveLength(1)
    expect(harness.savedSnapshots[0]?.groups).toEqual([])
  })

  it.each([
    ['single-member', ['image-1']],
    ['empty', []],
  ] as const)('ungroups a selected %s group frame without touching nodes', async (_, nodeIds) => {
    const snapshot = createSnapshot()
    snapshot.groups = [createGroup('group-1', [...nodeIds])]
    const harness = createRuntimeHarness(snapshot)
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    expect(
      harness.runtime.graphEditing.grouping.commands.selectGroupAtPoint({ x: 350, y: 80 }),
    ).toBe(true)
    expect(harness.runtime.graphEditing.grouping.view.selectionGroupAction.value).toEqual({
      kind: 'ungroup',
      label: 'Ungroup',
      groupId: 'group-1',
      nodeIds,
    })

    expect(harness.runtime.graphEditing.grouping.commands.ungroupSelectedGroup()).toBe(true)

    expect(harness.boardBaseSnapshot.value.groups).toEqual([])
    expect(harness.fullNodes.value.map((node) => node.id)).toEqual([
      'image-1',
      'prompt-1',
      'prompt-2',
    ])
  })

  it('selects a group by frame point without marking the board dirty', async () => {
    const snapshot = createSnapshot()
    snapshot.groups = [createGroup('group-1', ['image-1', 'prompt-1'])]
    const harness = createRuntimeHarness(snapshot)
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    expect(
      harness.runtime.graphEditing.grouping.commands.selectGroupAtPoint({ x: 350, y: 80 }),
    ).toBe(true)
    expect(
      harness.runtime.graphEditing.grouping.commands.selectGroupAtPoint({ x: 350, y: 80 }),
    ).toBe(true)

    expect(harness.runtime.graphEditing.grouping.internal.selectedGroupId.value).toBe('group-1')
    expect(harness.runtime.graphEditing.grouping.view.selectionGroupAction.value).toEqual({
      kind: 'ungroup',
      label: 'Ungroup',
      groupId: 'group-1',
      nodeIds: ['image-1', 'prompt-1'],
    })
    expect(harness.isDirty.value).toBe(false)
  })

  it('deletes a selected group with member nodes and releases local execution without cancelling the task', async () => {
    const snapshot = createSnapshot()
    const runningNode = snapshot.imageNodes[0]
    if (!runningNode) throw new Error('expected image node fixture')
    runningNode.pendingTaskId = 'task-running-1'
    runningNode.outputStatus = 'running'
    snapshot.groups = [createGroup('group-1', ['image-1', 'prompt-1'])]
    const harness = createRuntimeHarness(snapshot)
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    harness.runtime.graphEditing.grouping.commands.selectGroupAtPoint({ x: 350, y: 80 })

    expect(harness.runtime.graphEditing.grouping.commands.deleteSelectedGroup()).toBe(true)

    expect(harness.fullNodes.value.map((node) => node.id)).toEqual(['prompt-2'])
    expect(harness.fullEdges.value).toEqual([])
    expect(harness.boardBaseSnapshot.value.groups).toEqual([])
    expect(harness.executionRuntime.getNodeState('image-1')).toBeNull()
    expect(harness.isDirty.value).toBe(true)

    harness.runtime.persistence.history.undo()
    expect(harness.fullNodes.value.map((node) => node.id)).toEqual([
      'image-1',
      'prompt-1',
      'prompt-2',
    ])
    expect(harness.fullEdges.value.map((edge) => edge.id)).toEqual(['connection-3'])
    expect(harness.boardBaseSnapshot.value.groups).toEqual([
      createGroup('group-1', ['image-1', 'prompt-1']),
    ])

    harness.runtime.persistence.history.redo()
    expect(harness.fullNodes.value.map((node) => node.id)).toEqual(['prompt-2'])
    expect(harness.boardBaseSnapshot.value.groups).toEqual([])
  })

  it('deletes a selected empty group frame without touching nodes or edges', async () => {
    const snapshot = createSnapshot()
    snapshot.groups = [createGroup('group-1', [])]
    const harness = createRuntimeHarness(snapshot)
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()
    const historyPush = vi.spyOn(harness.runtime.persistence.history, 'record')

    expect(
      harness.runtime.graphEditing.grouping.commands.selectGroupAtPoint({ x: 350, y: 80 }),
    ).toBe(true)

    expect(harness.runtime.graphEditing.grouping.commands.deleteSelectedGroup()).toBe(true)

    expect(harness.boardBaseSnapshot.value.groups).toEqual([])
    expect(harness.runtime.graphEditing.grouping.internal.selectedGroupId.value).toBeNull()
    expect(harness.fullNodes.value.map((node) => node.id)).toEqual([
      'image-1',
      'prompt-1',
      'prompt-2',
    ])
    expect(harness.fullEdges.value.map((edge) => edge.id)).toEqual(['connection-3'])
    expect(harness.isDirty.value).toBe(true)
    expect(historyPush).toHaveBeenCalledOnce()
    expect(harness.saveStatus.value).toBe('pending')

    harness.runtime.persistence.history.undo()
    expect(harness.boardBaseSnapshot.value.groups).toEqual([createGroup('group-1', [])])

    harness.runtime.persistence.history.redo()
    expect(harness.boardBaseSnapshot.value.groups).toEqual([])

    await harness.runtime.persistence.flushAutosave({ silent: true })

    expect(harness.savedSnapshots).toHaveLength(1)
    expect(harness.savedSnapshots[0]?.groups).toEqual([])
  })

  it('derives grouped node z-index without changing edge layering', async () => {
    const snapshot = createSnapshot()
    snapshot.promptNodes.push(
      {
        id: 3,
        type: 'prompt',
        x: 2600,
        y: 40,
        width: 280,
        text: 'grouped peer',
        outputCount: 1,
        aspectRatio: 'auto',
        inputImages: [],
      },
      {
        id: 4,
        type: 'prompt',
        x: 3000,
        y: 40,
        width: 280,
        text: 'ungrouped peer',
        outputCount: 1,
        aspectRatio: 'auto',
        inputImages: [],
      },
    )
    snapshot.groups = [
      createGroup('group-1', ['image-1', 'prompt-1']),
      createGroup('group-2', ['prompt-2', 'prompt-3']),
    ]
    const harness = createRuntimeHarness(snapshot)
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    expect(harness.fullNodes.value.find((node) => node.id === 'image-1')?.zIndex).toBe(2)
    expect(harness.fullNodes.value.find((node) => node.id === 'prompt-1')?.zIndex).toBe(2)
    expect(harness.fullNodes.value.find((node) => node.id === 'prompt-2')?.zIndex).toBe(2)
    expect(harness.fullNodes.value.find((node) => node.id === 'prompt-3')?.zIndex).toBe(2)
    expect(harness.fullNodes.value.find((node) => node.id === 'prompt-4')?.zIndex).toBe(1)
    expect(harness.fullEdges.value[0]?.zIndex).toBeUndefined()
  })

  it('drags selected group empty space as one undoable movement', async () => {
    const snapshot = createSnapshot()
    snapshot.groups = [createGroup('group-1', ['image-1', 'prompt-1'])]
    const harness = createRuntimeHarness(snapshot)
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    expect(harness.runtime.graphEditing.grouping.commands.groupDrag.begin({ x: 350, y: 80 })).toBe(
      true,
    )
    expect(harness.runtime.graphEditing.grouping.commands.groupDrag.move({ x: 450, y: 120 })).toBe(
      true,
    )
    expect(harness.runtime.graphEditing.grouping.commands.groupDrag.commit()).toBe(true)

    expect(readNodePosition(harness.fullNodes.value, 'image-1')).toEqual({ x: 140, y: 80 })
    expect(readNodePosition(harness.fullNodes.value, 'prompt-1')).toEqual({ x: 500, y: 80 })
    expect(readNodePosition(harness.fullNodes.value, 'prompt-2')).toEqual({ x: 2200, y: 40 })
    expect(harness.boardBaseSnapshot.value.groups).toEqual([
      createGroup('group-1', ['image-1', 'prompt-1'], {
        x: SAMPLE_GROUP_GEOMETRY.x + 100,
        y: SAMPLE_GROUP_GEOMETRY.y + 40,
        width: SAMPLE_GROUP_GEOMETRY.width,
        height: SAMPLE_GROUP_GEOMETRY.height,
      }),
    ])

    harness.runtime.persistence.history.undo()
    expect(readNodePosition(harness.fullNodes.value, 'image-1')).toEqual({ x: 40, y: 40 })
    expect(readNodePosition(harness.fullNodes.value, 'prompt-1')).toEqual({ x: 400, y: 40 })
  })

  it('keeps group drag and resize transient until commit and discards them on cancel', async () => {
    const snapshot = createSnapshot()
    const originalGroup = createGroup('group-1', ['image-1', 'prompt-1'])
    snapshot.groups = [originalGroup]
    const harness = createRuntimeHarness(snapshot)
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()
    const grouping = harness.runtime.graphEditing.grouping

    expect(grouping.commands.groupDrag.begin({ x: 350, y: 80 })).toBe(true)
    expect(grouping.commands.groupDrag.move({ x: 450, y: 120 })).toBe(true)
    expect(grouping.view.groupFrames.value[0]?.bounds.left).toBe(SAMPLE_GROUP_GEOMETRY.x + 100)
    expect(harness.boardBaseSnapshot.value.groups).toEqual([originalGroup])
    expect(grouping.commands.groupDrag.cancel()).toBe(true)

    expect(readNodePosition(harness.fullNodes.value, 'image-1')).toEqual({ x: 40, y: 40 })
    expect(readNodePosition(harness.fullNodes.value, 'prompt-1')).toEqual({ x: 400, y: 40 })
    expect(grouping.view.groupFrames.value[0]?.bounds.left).toBe(SAMPLE_GROUP_GEOMETRY.x)

    expect(
      grouping.commands.groupResize.begin('group-1', 'e', {
        x: 720,
        y: 80,
      }),
    ).toBe(true)
    expect(grouping.commands.groupResize.move({ x: 920, y: 80 })).toBe(true)
    expect(grouping.view.groupFrames.value[0]?.bounds.width).toBe(SAMPLE_GROUP_GEOMETRY.width + 200)
    expect(harness.boardBaseSnapshot.value.groups).toEqual([originalGroup])
    expect(grouping.commands.groupResize.cancel()).toBe(true)

    expect(grouping.view.groupFrames.value[0]?.bounds.width).toBe(SAMPLE_GROUP_GEOMETRY.width)
    expect(harness.boardBaseSnapshot.value.groups).toEqual([originalGroup])
    expect(harness.isDirty.value).toBe(false)
    await harness.runtime.persistence.flushAutosave({ silent: true })
    expect(harness.savedSnapshots).toEqual([])
  })

  it('treats gestures that return to their origin as no-ops and owns gesture cleanup', async () => {
    const snapshot = createSnapshot()
    snapshot.groups = [createGroup('group-1', ['image-1', 'prompt-1'])]
    const harness = createRuntimeHarness(snapshot)
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()
    const grouping = harness.runtime.graphEditing.grouping

    expect(grouping.commands.groupDrag.begin({ x: 350, y: 80 })).toBe(true)
    expect(grouping.commands.groupDrag.begin({ x: 350, y: 80 })).toBe(false)
    expect(grouping.commands.groupResize.begin('group-1', 'e', { x: 720, y: 80 })).toBe(false)
    expect(grouping.commands.groupDrag.move({ x: 450, y: 120 })).toBe(true)
    expect(grouping.commands.groupDrag.move({ x: 350, y: 80 })).toBe(true)
    expect(grouping.commands.groupDrag.commit()).toBe(false)

    expect(readNodePosition(harness.fullNodes.value, 'image-1')).toEqual({ x: 40, y: 40 })
    expect(readNodePosition(harness.fullNodes.value, 'prompt-1')).toEqual({ x: 400, y: 40 })
    expect(grouping.commands.groupResize.begin('group-1', 'e', { x: 720, y: 80 })).toBe(true)
    expect(grouping.commands.groupResize.move({ x: 920, y: 80 })).toBe(true)
    expect(grouping.commands.groupResize.move({ x: 720, y: 80 })).toBe(true)
    expect(grouping.commands.groupResize.commit()).toBe(false)

    expect(harness.boardBaseSnapshot.value.groups).toEqual([
      createGroup('group-1', ['image-1', 'prompt-1']),
    ])
    expect(harness.isDirty.value).toBe(false)
    await harness.runtime.persistence.flushAutosave({ silent: true })
    expect(harness.savedSnapshots).toEqual([])

    expect(grouping.commands.groupDrag.begin({ x: 350, y: 80 })).toBe(true)
    grouping.internal.resetTransientSelectionState()
    expect(grouping.commands.groupDrag.commit()).toBe(false)
    expect(grouping.commands.groupResize.begin('group-1', 'e', { x: 720, y: 80 })).toBe(true)
    expect(grouping.commands.groupResize.cancel()).toBe(true)
  })

  it('does not auto-join covered external nodes on group drag and joins a pending node only after click', async () => {
    const snapshot = createSnapshot()
    snapshot.groups = [createGroup('group-1', ['image-1', 'prompt-1'])]
    const harness = createRuntimeHarness(snapshot)
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    expect(harness.runtime.graphEditing.grouping.commands.groupDrag.begin({ x: 350, y: 80 })).toBe(
      true,
    )
    expect(harness.runtime.graphEditing.grouping.commands.groupDrag.move({ x: 2150, y: 80 })).toBe(
      true,
    )
    expect(harness.runtime.graphEditing.grouping.commands.groupDrag.commit()).toBe(true)

    expect(harness.boardBaseSnapshot.value.groups).toEqual([
      createGroup('group-1', ['image-1', 'prompt-1'], {
        x: SAMPLE_GROUP_GEOMETRY.x + 1800,
        y: SAMPLE_GROUP_GEOMETRY.y,
        width: SAMPLE_GROUP_GEOMETRY.width,
        height: SAMPLE_GROUP_GEOMETRY.height,
      }),
    ])
    expect(
      harness.runtime.graphEditing.grouping.internal.selectedGroupCoveredExternalNodeIds.value,
    ).toEqual(['prompt-2'])

    clickPaneThroughGrouping(harness.runtime)

    clickNodeThroughGrouping(harness, 'prompt-2')

    expect(harness.boardBaseSnapshot.value.groups).toEqual([
      createGroup('group-1', ['image-1', 'prompt-1', 'prompt-2'], {
        x: SAMPLE_GROUP_GEOMETRY.x + 1800,
        y: SAMPLE_GROUP_GEOMETRY.y,
        width: SAMPLE_GROUP_GEOMETRY.width,
        height: SAMPLE_GROUP_GEOMETRY.height,
      }),
    ])
    expect(readNodeSelected(harness.fullNodes.value, 'prompt-2')).toBe(true)
    expect(readNodeSelected(harness.fullNodes.value, 'image-1')).not.toBe(true)
    expect(readNodeSelected(harness.fullNodes.value, 'prompt-1')).not.toBe(true)
  })

  it('clears pending group join when a candidate node starts moving', async () => {
    const snapshot = createSnapshot()
    snapshot.groups = [
      createGroup('group-1', ['image-1', 'prompt-1'], {
        x: 2100,
        y: 20,
        width: 700,
        height: 420,
      }),
    ]
    const harness = createRuntimeHarness(snapshot)
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    expect(
      harness.runtime.graphEditing.grouping.commands.selectGroupAtPoint({ x: 2300, y: 80 }),
    ).toBe(true)
    expect(
      harness.runtime.graphEditing.grouping.internal.selectedGroupCoveredExternalNodeIds.value,
    ).toEqual(['prompt-2'])
    clickPaneThroughGrouping(harness.runtime)

    harness.runtime.flowPane.nodeDragTransactions.begin(['prompt-2'])

    clickNodeThroughGrouping(harness, 'prompt-2')
    harness.runtime.flowPane.nodeDragTransactions.commit()
    expect(harness.boardBaseSnapshot.value.groups).toEqual([
      createGroup('group-1', ['image-1', 'prompt-1'], {
        x: 2100,
        y: 20,
        width: 700,
        height: 420,
      }),
    ])
  })

  it('drags an empty group frame without moving nodes or reconciling membership', async () => {
    const snapshot = createSnapshot()
    snapshot.groups = [createGroup('group-1', [])]
    const harness = createRuntimeHarness(snapshot)
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    expect(harness.runtime.graphEditing.grouping.commands.groupDrag.begin({ x: 350, y: 80 })).toBe(
      true,
    )
    expect(harness.runtime.graphEditing.grouping.commands.groupDrag.move({ x: 450, y: 120 })).toBe(
      true,
    )
    expect(harness.runtime.graphEditing.grouping.commands.groupDrag.commit()).toBe(true)

    expect(readNodePosition(harness.fullNodes.value, 'image-1')).toEqual({ x: 40, y: 40 })
    expect(readNodePosition(harness.fullNodes.value, 'prompt-1')).toEqual({ x: 400, y: 40 })
    expect(readNodePosition(harness.fullNodes.value, 'prompt-2')).toEqual({ x: 2200, y: 40 })
    expect(harness.boardBaseSnapshot.value.groups).toEqual([
      createGroup('group-1', [], {
        x: SAMPLE_GROUP_GEOMETRY.x + 100,
        y: SAMPLE_GROUP_GEOMETRY.y + 40,
        width: SAMPLE_GROUP_GEOMETRY.width,
        height: SAMPLE_GROUP_GEOMETRY.height,
      }),
    ])
  })

  it('resizes a selected group frame without auto-joining covered external nodes', async () => {
    const snapshot = createSnapshot()
    snapshot.groups = [createGroup('group-1', ['image-1', 'prompt-1'])]
    const harness = createRuntimeHarness(snapshot)
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    expect(
      harness.runtime.graphEditing.grouping.commands.groupResize.begin('group-1', 'e', {
        x: 720,
        y: 80,
      }),
    ).toBe(true)
    expect(
      harness.runtime.graphEditing.grouping.commands.groupResize.move({ x: 2520, y: 80 }),
    ).toBe(true)
    expect(harness.runtime.graphEditing.grouping.commands.groupResize.commit()).toBe(true)

    expect(readNodePosition(harness.fullNodes.value, 'prompt-2')).toEqual({ x: 2200, y: 40 })
    expect(harness.boardBaseSnapshot.value.groups).toEqual([
      createGroup('group-1', ['image-1', 'prompt-1'], {
        x: SAMPLE_GROUP_GEOMETRY.x,
        y: SAMPLE_GROUP_GEOMETRY.y,
        width: SAMPLE_GROUP_GEOMETRY.width + 1800,
        height: SAMPLE_GROUP_GEOMETRY.height,
      }),
    ])
    expect(
      harness.runtime.graphEditing.grouping.internal.selectedGroupCoveredExternalNodeIds.value,
    ).toEqual(['prompt-2'])

    clickPaneThroughGrouping(harness.runtime)

    clickNodeThroughGrouping(harness, 'prompt-2')

    expect(harness.boardBaseSnapshot.value.groups).toEqual([
      createGroup('group-1', ['image-1', 'prompt-1', 'prompt-2'], {
        x: SAMPLE_GROUP_GEOMETRY.x,
        y: SAMPLE_GROUP_GEOMETRY.y,
        width: SAMPLE_GROUP_GEOMETRY.width + 1800,
        height: SAMPLE_GROUP_GEOMETRY.height,
      }),
    ])
    expect(readNodeSelected(harness.fullNodes.value, 'prompt-2')).toBe(true)

    harness.runtime.persistence.history.undo()
    expect(harness.boardBaseSnapshot.value.groups).toEqual([
      createGroup('group-1', ['image-1', 'prompt-1'], {
        x: SAMPLE_GROUP_GEOMETRY.x,
        y: SAMPLE_GROUP_GEOMETRY.y,
        width: SAMPLE_GROUP_GEOMETRY.width + 1800,
        height: SAMPLE_GROUP_GEOMETRY.height,
      }),
    ])
    expect(readNodePosition(harness.fullNodes.value, 'prompt-2')).toEqual({ x: 2200, y: 40 })
  })

  it('removes existing members that fully leave a resized group frame', async () => {
    const snapshot = createSnapshot()
    snapshot.groups = [createGroup('group-1', ['image-1', 'prompt-1'])]
    const harness = createRuntimeHarness(snapshot)
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    expect(
      harness.runtime.graphEditing.grouping.commands.groupResize.begin('group-1', 'e', {
        x: 720,
        y: 80,
      }),
    ).toBe(true)
    expect(harness.runtime.graphEditing.grouping.commands.groupResize.move({ x: 350, y: 80 })).toBe(
      true,
    )
    expect(harness.runtime.graphEditing.grouping.commands.groupResize.commit()).toBe(true)

    expect(harness.boardBaseSnapshot.value.groups).toEqual([
      createGroup('group-1', ['image-1'], {
        x: SAMPLE_GROUP_GEOMETRY.x,
        y: SAMPLE_GROUP_GEOMETRY.y,
        width: SAMPLE_GROUP_GEOMETRY.width - 370,
        height: SAMPLE_GROUP_GEOMETRY.height,
      }),
    ])
    expect(readNodePosition(harness.fullNodes.value, 'image-1')).toEqual({ x: 40, y: 40 })
    expect(readNodePosition(harness.fullNodes.value, 'prompt-1')).toEqual({ x: 400, y: 40 })
  })

  it('uses the Graph snapshot rather than a divergent Flow projection for resize membership', async () => {
    const snapshot = createSnapshot()
    snapshot.groups = [createGroup('group-1', ['image-1', 'prompt-1'])]
    const harness = createRuntimeHarness(snapshot)
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()
    const grouping = harness.runtime.graphEditing.grouping

    expect(grouping.commands.groupResize.begin('group-1', 'e', { x: 720, y: 80 })).toBe(true)
    const promptNode = harness.fullNodes.value.find((node) => node.id === 'prompt-1')
    expect(promptNode).toBeDefined()
    if (promptNode) promptNode.position = { x: 40, y: 40 }
    expect(grouping.commands.groupResize.move({ x: 350, y: 80 })).toBe(true)
    expect(grouping.commands.groupResize.commit()).toBe(true)

    expect(harness.boardBaseSnapshot.value.groups).toEqual([
      createGroup('group-1', ['image-1'], {
        x: SAMPLE_GROUP_GEOMETRY.x,
        y: SAMPLE_GROUP_GEOMETRY.y,
        width: SAMPLE_GROUP_GEOMETRY.width - 370,
        height: SAMPLE_GROUP_GEOMETRY.height,
      }),
    ])
  })

  it('updates group membership when a node is dragged out or in', async () => {
    const snapshot = createSnapshot()
    snapshot.groups = [createGroup('group-1', ['image-1', 'prompt-1'])]
    const harness = createRuntimeHarness(snapshot)
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    harness.runtime.flowPane.nodeDragTransactions.begin(['image-1'])
    harness.runtime.flowPane.onNodesChange([
      {
        id: 'image-1',
        type: 'position',
        from: { x: 40, y: 40 },
        position: { x: -1000, y: 40 },
        dragging: true,
      },
    ])
    harness.runtime.flowPane.nodeDragTransactions.commit()

    expect(harness.boardBaseSnapshot.value.groups).toEqual([createGroup('group-1', ['prompt-1'])])

    harness.runtime.persistence.history.undo()
    expect(harness.boardBaseSnapshot.value.groups).toEqual([
      createGroup('group-1', ['image-1', 'prompt-1']),
    ])

    harness.runtime.flowPane.nodeDragTransactions.begin(['prompt-2'])
    harness.runtime.flowPane.onNodesChange([
      {
        id: 'prompt-2',
        type: 'position',
        from: { x: 2200, y: 40 },
        position: { x: 200, y: 80 },
        dragging: true,
      },
    ])
    harness.runtime.flowPane.nodeDragTransactions.commit()

    expect(harness.boardBaseSnapshot.value.groups).toEqual([
      createGroup('group-1', ['image-1', 'prompt-1', 'prompt-2']),
    ])
  })

  it('keeps a dragged member in its group until the node fully leaves the frame', async () => {
    const snapshot = createSnapshot()
    snapshot.groups = [createGroup('group-1', ['image-1', 'prompt-1'])]
    const harness = createRuntimeHarness(snapshot)
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    harness.runtime.flowPane.nodeDragTransactions.begin(['image-1'])
    harness.runtime.flowPane.onNodesChange([
      {
        id: 'image-1',
        type: 'position',
        from: { x: 40, y: 40 },
        position: { x: -100, y: 40 },
        dragging: true,
      },
    ])
    harness.runtime.flowPane.nodeDragTransactions.commit()

    expect(harness.boardBaseSnapshot.value.groups).toEqual([
      createGroup('group-1', ['image-1', 'prompt-1']),
    ])

    harness.runtime.flowPane.nodeDragTransactions.begin(['image-1'])
    harness.runtime.flowPane.onNodesChange([
      {
        id: 'image-1',
        type: 'position',
        from: { x: -100, y: 40 },
        position: { x: -400, y: 40 },
        dragging: true,
      },
    ])
    harness.runtime.flowPane.nodeDragTransactions.commit()

    expect(harness.boardBaseSnapshot.value.groups).toEqual([createGroup('group-1', ['prompt-1'])])
  })

  it('moves selection to dragged nodes when they leave the selected group', async () => {
    const snapshot = createSnapshot()
    snapshot.groups = [createGroup('group-1', ['image-1', 'prompt-1'])]
    const harness = createRuntimeHarness(snapshot)
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    expect(
      harness.runtime.graphEditing.grouping.commands.selectGroupAtPoint({ x: 350, y: 80 }),
    ).toBe(true)
    expect(harness.runtime.graphEditing.grouping.internal.selectedGroupId.value).toBe('group-1')

    harness.runtime.flowPane.nodeDragTransactions.begin(['image-1'])
    harness.runtime.flowPane.onNodesChange([
      {
        id: 'image-1',
        type: 'position',
        from: { x: 40, y: 40 },
        position: { x: -1000, y: 40 },
        dragging: true,
      },
    ])
    harness.runtime.flowPane.nodeDragTransactions.commit()

    expect(harness.boardBaseSnapshot.value.groups).toEqual([createGroup('group-1', ['prompt-1'])])
    expect(harness.runtime.graphEditing.grouping.internal.selectedGroupId.value).toBeNull()
    expect(harness.runtime.graphEditing.grouping.view.groupFrames.value[0]?.selected).toBe(false)
    expect(readNodeSelected(harness.fullNodes.value, 'image-1')).toBe(true)
    expect(readNodeSelected(harness.fullNodes.value, 'prompt-1')).not.toBe(true)
  })

  it('does not auto-join a node when drag end lands inside overlapping groups', async () => {
    const snapshot = createSnapshot()
    snapshot.promptNodes.push(
      {
        id: 3,
        type: 'prompt',
        x: 80,
        y: 80,
        width: 280,
        text: 'overlap-a',
        outputCount: 1,
        aspectRatio: 'auto',
        inputImages: [],
      },
      {
        id: 4,
        type: 'prompt',
        x: 400,
        y: 80,
        width: 280,
        text: 'overlap-b',
        outputCount: 1,
        aspectRatio: 'auto',
        inputImages: [],
      },
    )
    snapshot.groups = [
      createGroup('group-1', ['image-1', 'prompt-1'], {
        x: 20,
        y: 20,
        width: 700,
        height: 420,
      }),
      createGroup('group-2', ['prompt-3', 'prompt-4'], {
        x: 60,
        y: 40,
        width: 700,
        height: 420,
      }),
    ]
    const harness = createRuntimeHarness(snapshot)
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    harness.runtime.flowPane.nodeDragTransactions.begin(['prompt-2'])
    harness.runtime.flowPane.onNodesChange([
      {
        id: 'prompt-2',
        type: 'position',
        from: { x: 2200, y: 40 },
        position: { x: 200, y: 80 },
        dragging: true,
      },
    ])
    harness.runtime.flowPane.nodeDragTransactions.commit()

    expect(harness.boardBaseSnapshot.value.groups).toEqual([
      createGroup('group-1', ['image-1', 'prompt-1'], {
        x: 20,
        y: 20,
        width: 700,
        height: 420,
      }),
      createGroup('group-2', ['prompt-3', 'prompt-4'], {
        x: 60,
        y: 40,
        width: 700,
        height: 420,
      }),
    ])
  })

  it('does not mark dirty for selection-only graph adapter commands without side effects', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    harness.runtime.graphEditing.adapter.applyNodeChanges([
      { id: 'prompt-1', type: 'select', selected: true },
    ])

    expect(readNodeSelected(harness.fullNodes.value, 'prompt-1')).toBe(true)
    expect(harness.isDirty.value).toBe(false)

    await harness.runtime.persistence.flushAutosave({ silent: true })

    expect(harness.savedSnapshots).toHaveLength(0)
  })

  it('saves hidden nodes and edges from the full graph', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    harness.isDirty.value = true
    await harness.runtime.persistence.flushAutosave({ silent: true })

    expect(harness.savedSnapshots[0]?.imageNodes.map((node) => node.id)).toEqual([1])
    expect(harness.savedSnapshots[0]?.promptNodes.map((node) => node.id)).toEqual([1, 2])
    expect(harness.savedSnapshots[0]?.connections.map((connection) => connection.id)).toEqual([3])
  })

  it('preserves Vue Flow selection when a save commits a stabilized graph', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    harness.nodes.value = markFlowItemSelected(harness.nodes.value, 'prompt-1')

    harness.isDirty.value = true
    await harness.runtime.persistence.flushAutosave({ silent: true })

    expect(readNodeSelected(harness.fullNodes.value, 'prompt-1')).toBe(true)
    expect(readNodeSelected(harness.nodes.value, 'prompt-1')).toBe(true)
  })

  it('preserves selection recorded in the full graph when a save commits a stabilized graph', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    harness.fullNodes.value = markFlowItemSelected(harness.fullNodes.value, 'prompt-1')

    harness.isDirty.value = true
    await harness.runtime.persistence.flushAutosave({ silent: true })

    expect(readNodeSelected(harness.fullNodes.value, 'prompt-1')).toBe(true)
    expect(readNodeSelected(harness.nodes.value, 'prompt-1')).toBe(true)
  })

  it('preserves edge selection when a save commits a stabilized graph', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    harness.edges.value = markFlowItemSelected(harness.edges.value, 'connection-3')

    harness.isDirty.value = true
    await harness.runtime.persistence.flushAutosave({ silent: true })

    expect(readEdgeSelected(harness.fullEdges.value, 'connection-3')).toBe(true)
    expect(readEdgeSelected(harness.edges.value, 'connection-3')).toBe(true)
  })

  it('reconciles only explicitly selected Vue Flow elements', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    const promptNode = harness.nodes.value.find((node) => node.id === 'prompt-1')
    const imageNode = harness.nodes.value.find((node) => node.id === 'image-1')
    const edge = harness.edges.value.find((item) => item.id === 'connection-3')
    expect(promptNode).toBeDefined()
    expect(imageNode).toBeDefined()
    expect(edge).toBeDefined()
    if (!promptNode || !imageNode || !edge) return

    harness.runtime.flowPane.reconcileRendererSelection([
      { ...promptNode, selected: true },
      { ...imageNode, selected: false },
      { ...edge, selected: false },
    ])

    expect(readNodeSelected(harness.fullNodes.value, 'prompt-1')).toBe(true)
    expect(readNodeSelected(harness.fullNodes.value, 'image-1')).not.toBe(true)
    expect(readEdgeSelected(harness.fullEdges.value, 'connection-3')).not.toBe(true)
  })

  it('does not carry selection into a freshly imported board snapshot', async () => {
    const firstSnapshot = createSnapshot()
    const harness = createRuntimeHarness(firstSnapshot)
    await harness.runtime.persistence.loadInitialOrRouteBoard()
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

    await harness.runtime.persistence.loadInitialOrRouteBoard()

    expect(readNodeSelected(harness.fullNodes.value, 'prompt-1')).toBeUndefined()
    expect(readNodeSelected(harness.nodes.value, 'prompt-1')).toBeUndefined()
    expect(readEdgeSelected(harness.fullEdges.value, 'connection-3')).toBeUndefined()
    expect(readEdgeSelected(harness.edges.value, 'connection-3')).toBeUndefined()
  })

  it('syncs Vue Flow node position during drag, saves on drag commit, and keeps selection', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.persistence.loadInitialOrRouteBoard()

    const originalFullNode = harness.fullNodes.value.find((node) => node.id === 'prompt-1')
    expect(originalFullNode?.position).toEqual({ x: 400, y: 40 })

    harness.runtime.flowPane.nodeDragTransactions.begin(['prompt-1'])

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

    harness.runtime.flowPane.nodeDragTransactions.commit()
    await nextTick()

    expect(harness.fullNodes.value.find((node) => node.id === 'prompt-1')?.position).toEqual({
      x: 620,
      y: 80,
    })
    expect(readNodeSelected(harness.fullNodes.value, 'prompt-1')).toBe(true)
    expect(harness.savedSnapshots).toHaveLength(0)

    harness.runtime.persistence.history.undo()

    expect(harness.fullNodes.value.find((node) => node.id === 'prompt-1')?.position).toEqual({
      x: 400,
      y: 40,
    })

    harness.runtime.persistence.history.redo()

    expect(harness.fullNodes.value.find((node) => node.id === 'prompt-1')?.position).toEqual({
      x: 620,
      y: 80,
    })

    await harness.runtime.persistence.flushAutosave({ silent: true })

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
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    harness.fullNodes.value = markFlowItemSelected(harness.fullNodes.value, 'prompt-1')
    harness.nodes.value = harness.nodes.value.map((node) =>
      node.id === 'prompt-1' ? { ...node, selected: false } : node,
    )

    expect(readNodeSelected(harness.fullNodes.value, 'prompt-1')).toBe(false)
  })

  it('accepts Vue Flow select changes without marking the board dirty', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    await harness.applyNativeNodeChanges([{ id: 'prompt-1', type: 'select', selected: true }])
    await harness.applyNativeEdgeChanges([{ id: 'connection-3', type: 'select', selected: true }])

    expect(readNodeSelected(harness.fullNodes.value, 'prompt-1')).toBe(true)
    expect(readNodeSelected(harness.nodes.value, 'prompt-1')).toBe(true)
    expect(readEdgeSelected(harness.fullEdges.value, 'connection-3')).toBe(true)
    expect(readEdgeSelected(harness.edges.value, 'connection-3')).toBe(true)
    expect(harness.savedSnapshots).toHaveLength(0)
  })

  it('accepts Vue Flow position changes without saving before drag transaction commits', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.persistence.loadInitialOrRouteBoard()
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

  it('keeps drag progress in the controlled model without re-entering Vue Flow', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()
    harness.graphActions.applyNodeChanges.mockClear()

    harness.runtime.flowPane.nodeDragTransactions.begin(['prompt-1'])
    harness.runtime.flowPane.onNodesChange([
      {
        id: 'prompt-1',
        type: 'position',
        position: { x: 440, y: 80 },
        from: { x: 400, y: 40 },
        dragging: true,
      },
    ])
    harness.runtime.flowPane.onNodesChange([
      {
        id: 'prompt-1',
        type: 'position',
        position: { x: 500, y: 80 },
        from: { x: 400, y: 40 },
        dragging: true,
      },
    ])
    harness.runtime.flowPane.onNodesChange([
      {
        id: 'prompt-1',
        type: 'position',
        position: { x: 620, y: 80 },
        from: { x: 400, y: 40 },
        dragging: true,
      },
    ])

    expect(harness.graphActions.applyNodeChanges).not.toHaveBeenCalled()
    expect(harness.graphActions.applyNodeChanges).not.toHaveBeenCalled()
    expect(harness.fullNodes.value.find((node) => node.id === 'prompt-1')?.position).toEqual({
      x: 620,
      y: 80,
    })

    harness.runtime.flowPane.nodeDragTransactions.commit()
    await nextTick()

    expect(harness.isDirty.value).toBe(true)
    expect(harness.savedSnapshots).toHaveLength(0)

    harness.runtime.persistence.history.undo()
    expect(harness.fullNodes.value.find((node) => node.id === 'prompt-1')?.position).toEqual({
      x: 400,
      y: 40,
    })

    harness.runtime.persistence.history.redo()
    expect(harness.fullNodes.value.find((node) => node.id === 'prompt-1')?.position).toEqual({
      x: 620,
      y: 80,
    })

    await harness.runtime.persistence.flushAutosave({ silent: true })
    expect(harness.savedSnapshots).toHaveLength(1)
    expect(harness.savedSnapshots[0]?.promptNodes.find((node) => node.id === 1)).toMatchObject({
      x: 620,
      y: 80,
    })
  })

  it('commits a non-target position from a mixed drag batch through the canonical graph', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    harness.runtime.flowPane.nodeDragTransactions.begin(['prompt-1'])
    harness.runtime.flowPane.onNodesChange([
      {
        id: 'prompt-1',
        type: 'position',
        position: { x: 620, y: 80 },
        from: { x: 400, y: 40 },
        dragging: true,
      },
      {
        id: 'prompt-2',
        type: 'position',
        position: { x: 2280, y: 120 },
        from: { x: 2200, y: 40 },
        dragging: true,
      },
    ])
    harness.runtime.flowPane.nodeDragTransactions.commit()
    await nextTick()

    expect(harness.fullNodes.value.find((node) => node.id === 'prompt-1')?.position).toEqual({
      x: 620,
      y: 80,
    })
    expect(harness.fullNodes.value.find((node) => node.id === 'prompt-2')?.position).toEqual({
      x: 2280,
      y: 120,
    })

    await harness.runtime.persistence.flushAutosave({ silent: true })
    expect(harness.savedSnapshots[0]?.promptNodes.find((node) => node.id === 2)).toMatchObject({
      x: 2280,
      y: 120,
    })
  })

  it('does not commit a stale save snapshot over active node drag progress', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    let resolveSave: ((result: Awaited<ReturnType<CanvasBoardSession['save']>>) => void) | undefined
    let shouldDeferSave = true
    harness.boardSession.save = vi.fn((nextSnapshot: CanvasBoardSnapshot) => {
      harness.savedSnapshots.push(nextSnapshot)
      if (!shouldDeferSave) {
        return Promise.resolve({
          snapshot: nextSnapshot,
          remoteSync: { ok: true as const },
        })
      }
      shouldDeferSave = false
      return new Promise<Awaited<ReturnType<CanvasBoardSession['save']>>>((resolve) => {
        resolveSave = resolve
      })
    })

    harness.isDirty.value = true
    const savePromise = harness.runtime.persistence.flushAutosave({ silent: true })
    await nextTick()

    expect(harness.boardSession.save).toHaveBeenCalledOnce()
    expect(harness.savedSnapshots[0]?.promptNodes.find((node) => node.id === 1)).toMatchObject({
      x: 400,
      y: 40,
    })

    harness.runtime.flowPane.nodeDragTransactions.begin(['prompt-1'])
    harness.runtime.flowPane.onNodesChange([
      {
        id: 'prompt-1',
        type: 'position',
        position: { x: 720, y: 96 },
        from: { x: 400, y: 40 },
        dragging: true,
      },
    ])
    await nextTick()

    expect(harness.fullNodes.value.find((node) => node.id === 'prompt-1')?.position).toEqual({
      x: 720,
      y: 96,
    })

    resolveSave?.({
      snapshot: harness.savedSnapshots[0] as CanvasBoardSnapshot,
      remoteSync: { ok: true },
    })
    await savePromise
    await nextTick()

    expect(harness.fullNodes.value.find((node) => node.id === 'prompt-1')?.position).toEqual({
      x: 720,
      y: 96,
    })
    expect(harness.isDirty.value).toBe(true)
    expect(harness.saveStatus.value).toBe('pending')

    harness.runtime.flowPane.nodeDragTransactions.commit()
    await nextTick()
    await harness.runtime.persistence.flushAutosave({ silent: true })

    expect(harness.savedSnapshots[1]?.promptNodes.find((node) => node.id === 1)).toMatchObject({
      x: 720,
      y: 96,
    })
  })

  it('cancels an active node drag when reloading the board', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()
    harness.graphActions.applyNodeChanges.mockClear()

    harness.runtime.flowPane.nodeDragTransactions.begin(['prompt-1'])
    harness.runtime.flowPane.onNodesChange([
      {
        id: 'prompt-1',
        type: 'position',
        position: { x: 720, y: 96 },
        from: { x: 400, y: 40 },
        dragging: true,
      },
    ])

    await harness.runtime.persistence.loadInitialOrRouteBoard()
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
    await harness.runtime.persistence.loadInitialOrRouteBoard()
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
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    harness.runtime.flowPane.nodeDragTransactions.begin(['prompt-1'])

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

    harness.runtime.flowPane.nodeDragTransactions.commit()
    await nextTick()

    expect(harness.fullNodes.value.find((node) => node.id === 'prompt-1')?.position).toEqual({
      x: 720,
      y: 96,
    })
    expect(harness.nodes.value).not.toBe(nodesBeforeCommit)
  })

  it('keeps one canonical Graph Store for the entire drag transaction', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    const store = harness.runtime.graphEditing.readCanvasGraphStore()

    harness.runtime.flowPane.nodeDragTransactions.begin(['prompt-1'])
    await harness.applyNativeNodeChanges([
      {
        id: 'prompt-1',
        type: 'position',
        position: { x: 720, y: 96 },
        from: { x: 400, y: 40 },
        dragging: true,
      },
    ])
    expect(harness.runtime.graphEditing.readCanvasGraphStore()).toBe(store)

    harness.runtime.flowPane.nodeDragTransactions.commit()
    expect(harness.runtime.graphEditing.readCanvasGraphStore()).toBe(store)
  })

  it('routes UI node deletion through the shared delete contract', async () => {
    const snapshot = createSnapshot()
    snapshot.imageNodes[0] = {
      ...snapshot.imageNodes[0]!,
      outputStatus: 'running',
      pendingTaskId: 'task-ui-delete',
      outputImages: [],
    }
    snapshot.groups = [createGroup('group-1', ['image-1', 'prompt-1'])]
    const harness = createRuntimeHarness(snapshot)
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    const currentEdge = harness.fullEdges.value[0]
    expect(currentEdge).toBeDefined()
    harness.fullEdges.value = [
      {
        ...(currentEdge as Edge),
        id: 'vueflow__edge-image-1out:image-prompt-1in:image',
        data: undefined,
      },
    ]
    const { graphRuntime } = createCanvasGraphRuntimeHarness(harness)

    graphRuntime.onFlowDeleteNode('image-1')

    expect(harness.fullNodes.value.map((node) => node.id)).toEqual(['prompt-1', 'prompt-2'])
    expect(harness.fullEdges.value).toEqual([])
    expect(harness.boardBaseSnapshot.value.groups).toEqual([createGroup('group-1', ['prompt-1'])])
    expect(harness.executionRuntime.getNodeState('image-1')).toBeNull()
    expect(harness.isDirty.value).toBe(true)

    harness.runtime.persistence.history.undo()
    expect(harness.fullNodes.value.map((node) => node.id)).toEqual([
      'image-1',
      'prompt-1',
      'prompt-2',
    ])
    expect(harness.fullEdges.value).toHaveLength(1)
    expect(harness.boardBaseSnapshot.value.groups).toEqual([
      createGroup('group-1', ['image-1', 'prompt-1']),
    ])

    harness.runtime.persistence.history.redo()
    expect(harness.fullNodes.value.map((node) => node.id)).toEqual(['prompt-1', 'prompt-2'])
    expect(harness.fullEdges.value).toEqual([])
    expect(harness.boardBaseSnapshot.value.groups).toEqual([createGroup('group-1', ['prompt-1'])])

    await harness.runtime.persistence.flushAutosave({ silent: true })
    expect(harness.savedSnapshots[0]?.imageNodes).toEqual([])
    expect(harness.savedSnapshots[0]?.connections).toEqual([])
    expect(harness.savedSnapshots[0]?.groups).toEqual([createGroup('group-1', ['prompt-1'])])
  })

  it('removes a deleted source pending slots without cancelling their shared task', async () => {
    const snapshot = createSnapshot()
    snapshot.imageNodes.push(
      {
        id: 2,
        type: 'image-node',
        x: 760,
        y: 40,
        width: 280,
        sourceImageUrl: '',
        sourceImageRatio: 1,
        outputStatus: 'running',
        outputImages: [],
        pendingTaskId: 'task-shared',
        pendingResultIndex: 0,
        pendingClientActionId: 'action-task-shared',
      },
      {
        id: 3,
        type: 'image-node',
        x: 760,
        y: 380,
        width: 280,
        sourceImageUrl: '',
        sourceImageRatio: 1,
        outputStatus: 'running',
        outputImages: [],
        pendingTaskId: 'task-shared',
        pendingResultIndex: 1,
        pendingClientActionId: 'action-task-shared',
      },
    )
    snapshot.connections.push(
      {
        id: 4,
        fromNodeType: 'prompt',
        fromNodeId: 1,
        fromPort: 'image',
        toNodeType: 'image',
        toNodeId: 2,
        toPort: 'image',
      },
      {
        id: 5,
        fromNodeType: 'prompt',
        fromNodeId: 1,
        fromPort: 'image',
        toNodeType: 'image',
        toNodeId: 3,
        toPort: 'image',
      },
    )
    const harness = createRuntimeHarness(snapshot)
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    expect(harness.executionRuntime.getNodeState('image-2')).toEqual({
      status: 'running',
      taskId: 'task-shared',
    })
    expect(harness.runtime.graphEditing.deleteNode('prompt-1')).toBe(true)

    expect(harness.fullNodes.value.map((node) => node.id)).toEqual(['image-1', 'prompt-2'])
    expect(harness.executionRuntime.getNodeState('image-2')).toBeNull()
    expect(harness.executionRuntime.getNodeState('image-3')).toBeNull()
  })

  it('warns without mutation when UI node deletion receives a non-canonical id', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    const imageNode = harness.fullNodes.value.find((node) => node.id === 'image-1')
    expect(imageNode).toBeDefined()
    harness.fullNodes.value = harness.fullNodes.value.map((node) =>
      node.id === 'image-1' ? ({ ...node, id: 'node-image-1' } as Node) : node,
    )
    const graphBefore = exportCurrentBoardSnapshot(harness)
    const { graphRuntime } = createCanvasGraphRuntimeHarness(harness)

    graphRuntime.onFlowDeleteNode('node-image-1')

    expect(harness.onWarning).toHaveBeenCalledWith(
      'Failed to write the deletion to the canvas. Refresh and try again.',
    )
    expect(exportCurrentBoardSnapshot(harness)).toEqual(graphBefore)
    expect(harness.isDirty.value).toBe(false)
  })

  it('warns without mutation when a canonical UI delete target is absent from the graph store', async () => {
    const snapshot = createSnapshot()
    snapshot.imageNodes[0] = {
      ...snapshot.imageNodes[0]!,
      outputStatus: 'running',
      pendingTaskId: 'task-corrupt-ui-delete',
      outputImages: [],
    }
    snapshot.groups = [createGroup('group-1', ['image-1', 'prompt-1'])]
    const harness = createRuntimeHarness(snapshot)
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    const graphStore = harness.runtime.graphEditing.readCanvasGraphStore()
    const corruption = graphStore.beginTransaction('delete', {
      recordHistory: false,
      markDirty: false,
    })
    corruption.removeNode('image-1', { removeConnectedEdges: true })
    corruption.commit()
    const nodesBefore = harness.fullNodes.value
    const edgesBefore = harness.fullEdges.value
    const groupsBefore = harness.boardBaseSnapshot.value.groups
    const saveStatusBefore = harness.saveStatus.value
    const { graphRuntime } = createCanvasGraphRuntimeHarness(harness)

    graphRuntime.onFlowDeleteNode('image-1')

    expect(harness.onWarning).not.toHaveBeenCalled()
    expect(harness.fullNodes.value).toBe(nodesBefore)
    expect(harness.fullEdges.value).toBe(edgesBefore)
    expect(harness.boardBaseSnapshot.value.groups).toBe(groupsBefore)
    expect(harness.isDirty.value).toBe(false)
    expect(harness.saveStatus.value).toBe(saveStatusBefore)

    harness.runtime.persistence.history.undo()
    expect(harness.fullNodes.value).toBe(nodesBefore)
    expect(harness.fullEdges.value).toBe(edgesBefore)
    expect(harness.boardBaseSnapshot.value.groups).toBe(groupsBefore)

    await harness.runtime.persistence.flushAutosave({ silent: true })
    expect(harness.savedSnapshots).toEqual([])
  })

  it('records native node and edge remove changes through the graph adapter', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    harness.runtime.flowPane.onNodesChange([{ id: 'image-1', type: 'remove' }])
    harness.runtime.flowPane.onEdgesChange([
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

    expect(harness.graphActions.setNodes).toHaveBeenCalled()
    expect(harness.graphActions.setEdges).toHaveBeenCalled()
    expect(harness.fullNodes.value.map((node) => node.id)).toEqual(['prompt-1', 'prompt-2'])
    expect(harness.fullEdges.value).toHaveLength(0)

    await harness.runtime.persistence.flushAutosave({ silent: true })

    expect(harness.savedSnapshots[0]?.imageNodes).toHaveLength(0)
    expect(harness.savedSnapshots[0]?.connections).toHaveLength(0)
  })

  it('pins the Flow Pane native delete mutation path as a runtime contract', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    await harness.applyNativeNodeChanges([{ id: 'image-1', type: 'remove' }])

    expect(harness.fullNodes.value.map((node) => node.id)).toEqual(['prompt-1', 'prompt-2'])
    expect(harness.fullEdges.value).toHaveLength(0)
    expect(harness.isDirty.value).toBe(true)

    harness.runtime.persistence.history.undo()

    expect(harness.fullNodes.value.map((node) => node.id)).toEqual([
      'image-1',
      'prompt-1',
      'prompt-2',
    ])
    expect(harness.fullEdges.value.map((edge) => edge.id)).toEqual(['connection-3'])

    await harness.runtime.persistence.flushAutosave({ silent: true })

    expect(harness.savedSnapshots[0]?.imageNodes.map((node) => node.id)).toEqual([1])
    expect(harness.savedSnapshots[0]?.connections.map((connection) => connection.id)).toEqual([3])
  })

  it('releases the pending slot without cancelling its submitted task on native delete', async () => {
    const snapshot = createSnapshot()
    snapshot.imageNodes[0] = {
      ...snapshot.imageNodes[0]!,
      outputStatus: 'running',
      pendingTaskId: 'task-native-delete',
      outputImages: [],
    }
    const harness = createRuntimeHarness(snapshot)
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    await harness.applyNativeNodeChanges([{ id: 'image-1', type: 'remove' }])

    expect(harness.executionRuntime.getNodeState('image-1')).toBeNull()
  })

  it('records native edge remove before node remove as one cleanup', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    harness.runtime.flowPane.onEdgesChange([
      {
        id: 'connection-3',
        type: 'remove',
        source: 'image-1',
        target: 'prompt-1',
        sourceHandle: 'out:image',
        targetHandle: 'in:image',
      },
    ])
    harness.runtime.flowPane.onNodesChange([{ id: 'image-1', type: 'remove' }])
    await harness.flushNativeGraphChanges()

    expect(harness.fullNodes.value.map((node) => node.id)).toEqual(['prompt-1', 'prompt-2'])
    expect(harness.fullEdges.value).toHaveLength(0)
  })

  it('records native edge disconnect as one undoable transaction', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    harness.runtime.flowPane.onEdgesChange([
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

    expect(harness.graphActions.setEdges).toHaveBeenCalledWith([])
    expect(harness.fullNodes.value.map((node) => node.id)).toEqual([
      'image-1',
      'prompt-1',
      'prompt-2',
    ])
    expect(harness.fullEdges.value).toHaveLength(0)

    harness.runtime.persistence.history.undo()
    expect(harness.fullEdges.value.map((edge) => edge.id)).toEqual(['connection-3'])

    harness.runtime.persistence.history.redo()
    expect(harness.fullEdges.value).toHaveLength(0)
  })

  it('resolves native edge delete by endpoints when Vue Flow edge metadata is missing', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.persistence.loadInitialOrRouteBoard()
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
      'Failed to apply the deletion. Refresh the canvas and try again.',
    )
    expect(harness.fullEdges.value).toHaveLength(0)
  })

  it('warns and leaves the graph unchanged when native edge remove cannot resolve canonical edge data', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()
    harness.graphActions.applyEdgeChanges.mockClear()
    const currentEdge = harness.fullEdges.value[0]
    expect(currentEdge).toBeDefined()
    const unresolvedEdge = {
      ...(currentEdge as Edge),
      id: 'vueflow__edge-external-source-external-target',
      source: 'external-source',
      target: 'external-target',
      data: undefined,
    } as Edge
    harness.fullEdges.value = [unresolvedEdge]
    const edgeProjectionBefore = readEdgeProjection(harness.fullEdges.value)
    const saveStatusBefore = harness.saveStatus.value

    await harness.applyNativeEdgeChanges([
      {
        id: unresolvedEdge.id,
        type: 'remove',
        source: unresolvedEdge.source,
        target: unresolvedEdge.target,
        sourceHandle: unresolvedEdge.sourceHandle ?? null,
        targetHandle: unresolvedEdge.targetHandle ?? null,
      },
    ])

    expect(harness.onWarning).toHaveBeenCalledWith(
      'Failed to apply the deletion. Refresh the canvas and try again.',
    )
    expect(readEdgeProjection(harness.fullEdges.value)).toEqual(edgeProjectionBefore)
    expect(harness.graphActions.applyEdgeChanges).not.toHaveBeenCalled()
    expect(harness.isDirty.value).toBe(false)
    expect(harness.saveStatus.value).toBe(saveStatusBefore)
    expect(harness.savedSnapshots).toHaveLength(0)
  })

  it('deletes a node when connected Vue Flow edge metadata is missing', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    const [currentEdge] = harness.fullEdges.value
    expect(currentEdge).toBeDefined()
    const generatedEdge = {
      ...currentEdge,
      id: 'vueflow__edge-image-1out:image-prompt-1in:image',
      data: undefined,
    } as Edge
    harness.fullEdges.value = [generatedEdge]

    harness.runtime.flowPane.onNodesChange([{ id: 'image-1', type: 'remove' }])
    harness.runtime.flowPane.onEdgesChange([
      {
        id: generatedEdge.id,
        type: 'remove',
        source: generatedEdge.source,
        target: generatedEdge.target,
        sourceHandle: generatedEdge.sourceHandle ?? null,
        targetHandle: generatedEdge.targetHandle ?? null,
      },
    ])
    await harness.flushNativeGraphChanges()

    expect(harness.onWarning).not.toHaveBeenCalledWith(
      'Failed to apply the deletion. Refresh the canvas and try again.',
    )
    expect(harness.fullNodes.value.map((node) => node.id)).toEqual(['prompt-1', 'prompt-2'])
    expect(harness.fullEdges.value).toHaveLength(0)
  })

  it('deletes a node from Vue Flow delete-key edge-first remove changes', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    const [currentEdge] = harness.fullEdges.value
    expect(currentEdge).toBeDefined()
    const generatedEdge = {
      ...currentEdge,
      id: 'vueflow__edge-image-1out:image-prompt-1in:image',
      data: undefined,
    } as Edge
    harness.fullEdges.value = [generatedEdge]

    harness.runtime.flowPane.onEdgesChange([
      {
        id: generatedEdge.id,
        type: 'remove',
        source: generatedEdge.source,
        target: generatedEdge.target,
        sourceHandle: generatedEdge.sourceHandle ?? null,
        targetHandle: generatedEdge.targetHandle ?? null,
      },
    ])
    harness.runtime.flowPane.onNodesChange([{ id: 'image-1', type: 'remove' }])
    await harness.flushNativeGraphChanges()

    expect(harness.onWarning).not.toHaveBeenCalledWith(
      'Failed to apply the deletion. Refresh the canvas and try again.',
    )
    expect(harness.fullNodes.value.map((node) => node.id)).toEqual(['prompt-1', 'prompt-2'])
    expect(harness.fullEdges.value).toHaveLength(0)
  })

  it('deletes a canonical canvas node even when the Flow node id has drifted', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    const imageNode = harness.fullNodes.value.find((node) => node.id === 'image-1')
    expect(imageNode).toBeDefined()
    const driftedImageNode = { ...imageNode, id: 'node-image-1' } as Node
    harness.fullNodes.value = harness.fullNodes.value.map((node) =>
      node.id === 'image-1' ? driftedImageNode : node,
    )
    harness.fullEdges.value = harness.fullEdges.value.map((edge) =>
      edge.source === 'image-1' ? { ...edge, source: driftedImageNode.id } : edge,
    )

    harness.runtime.flowPane.onEdgesChange([
      {
        id: 'connection-3',
        type: 'remove',
        source: driftedImageNode.id,
        target: 'prompt-1',
        sourceHandle: 'out:image',
        targetHandle: 'in:image',
      },
    ])
    harness.runtime.flowPane.onNodesChange([{ id: driftedImageNode.id, type: 'remove' }])
    await harness.flushNativeGraphChanges()

    expect(harness.onWarning).not.toHaveBeenCalledWith(
      'Failed to apply the deletion. Refresh the canvas and try again.',
    )
    expect(harness.fullNodes.value.map((node) => node.id)).toEqual(['prompt-1', 'prompt-2'])
    expect(harness.fullEdges.value).toHaveLength(0)
  })

  it('coalesces native multi-node delete into one undoable transaction', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    harness.runtime.flowPane.onNodesChange([
      { id: 'image-1', type: 'remove' },
      { id: 'prompt-2', type: 'remove' },
    ])
    harness.runtime.flowPane.onEdgesChange([
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

    expect(harness.graphActions.setNodes).toHaveBeenCalled()
    expect(harness.graphActions.setEdges).toHaveBeenCalled()
    expect(harness.fullNodes.value.map((node) => node.id)).toEqual(['prompt-1'])
    expect(harness.fullEdges.value).toHaveLength(0)

    harness.runtime.persistence.history.undo()
    expect(harness.fullNodes.value.map((node) => node.id)).toEqual([
      'image-1',
      'prompt-1',
      'prompt-2',
    ])
    expect(harness.fullEdges.value.map((edge) => edge.id)).toEqual(['connection-3'])

    harness.runtime.persistence.history.redo()
    expect(harness.fullNodes.value.map((node) => node.id)).toEqual(['prompt-1'])
    expect(harness.fullEdges.value).toHaveLength(0)
  })

  it('restores native deletes through undo and redo', async () => {
    const harness = createRuntimeHarness()
    await harness.runtime.persistence.loadInitialOrRouteBoard()
    await nextTick()

    harness.runtime.flowPane.onNodesChange([{ id: 'image-1', type: 'remove' }])
    harness.runtime.flowPane.onEdgesChange([
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

    harness.runtime.persistence.history.undo()

    expect(harness.fullNodes.value.map((node) => node.id)).toEqual([
      'image-1',
      'prompt-1',
      'prompt-2',
    ])
    expect(harness.fullEdges.value.map((edge) => edge.id)).toEqual(['connection-3'])

    harness.runtime.persistence.history.undo()

    expect(harness.fullNodes.value.map((node) => node.id)).toEqual([
      'image-1',
      'prompt-1',
      'prompt-2',
    ])
    expect(harness.fullEdges.value.map((edge) => edge.id)).toEqual(['connection-3'])

    harness.runtime.persistence.history.redo()

    expect(harness.fullNodes.value.map((node) => node.id)).toEqual(['prompt-1', 'prompt-2'])
    expect(harness.fullEdges.value).toHaveLength(0)
  })
})
