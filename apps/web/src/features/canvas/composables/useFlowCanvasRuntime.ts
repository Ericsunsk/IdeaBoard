import type { Ref } from 'vue'
import type { Edge, EdgeChange, Node, NodeChange } from '@vue-flow/core'
import { useFlowHistory, type FlowHistorySnapshot } from './useFlowHistory'
import { useFlowBoardPersistence } from './useFlowBoardPersistence'
import { useFlowAutosave } from './useFlowAutosave'
import {
  useCanvasVueFlowGraphAdapter,
  type CanvasVueFlowGraphActions,
  type CanvasVueFlowGraphAdapter,
  type CanvasVueFlowGraphCommandContext,
  type CanvasVueFlowGraphCommandResult,
} from './useCanvasVueFlowGraphAdapter'
import {
  createFlowGraphCommandOptions,
  readFlowGraphCommandSideEffects,
} from './flowGraphAdapterCommandSideEffects'
import type { CanvasBoardSnapshot, CanvasConnection } from '../../canvas-board/domain'
import type { CanvasBoardSession } from '../board-session'
import {
  flowToCanvasBoardSnapshot,
  resolveFlowEdgeCanvasConnectionId,
} from '../utils/flowSnapshotAdapter'
import { flowConnectionToCanvasConnection } from '../utils/flowConnectionAdapter'
import {
  createCanvasGraphStore,
  toCanvasGraphEdgeId,
  type CanvasGraphEdgeId,
  type CanvasGraphNodeId,
  type CanvasGraphTransaction,
  type CanvasGraphTransactionReason,
  type CanvasGraphTransactionSummary,
} from '../graph-runtime'

type SaveStatus = 'idle' | 'saved' | 'failed'

type ViewportState = {
  x: number
  y: number
  zoom: number
}

type SelectableFlowItem = {
  id: string
  selected?: boolean
}

type FlowGraphCommitOptions = {
  preserveSelection?: boolean
}

type NodeDragChange = NodeChange & {
  id: string
  type: 'position' | 'select'
}

type EdgeLookupInput = Pick<Edge, 'id' | 'data'> &
  Partial<Pick<Edge, 'source' | 'target' | 'sourceHandle' | 'targetHandle'>>
type EdgeRemoveChange = Extract<EdgeChange, { type: 'remove' }>

export type CanvasFlowGraphRuntimeControls = {
  captureHistory: () => FlowHistorySnapshot
  markDirty: () => void
  nodeDrag: {
    begin: () => void
    commit: () => void
    cancel: () => void
  }
}

export type UseFlowCanvasRuntimeOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  boardBaseSnapshot: Ref<CanvasBoardSnapshot>
  isDirty: Ref<boolean>
  saveStatus: Ref<SaveStatus>
  isLoadingBoardSnapshot: Ref<boolean>
  isSavingBoard: Ref<boolean>
  boardSession: CanvasBoardSession
  getViewport: () => ViewportState
  setViewport: (viewport: ViewportState) => void
  onWarning: (message: string) => void
  onSuccess: (message: string) => void
  onError: (message: string) => void
  onNativeRemove?: () => void
  scheduleMicrotask?: (callback: () => void) => void
  scheduleAnimationFrame?: (callback: FrameRequestCallback) => number
  cancelAnimationFrame?: (handle: number) => void
  vueFlowGraphActions?: CanvasVueFlowGraphActions
}

export function useFlowCanvasRuntime(options: UseFlowCanvasRuntimeOptions) {
  let isNodeDragActive = false
  let activeNodeDrag: {
    transaction: CanvasGraphTransaction
  } | null = null
  let pendingNodeDragChanges: NodeDragChange[] = []
  let latestNodeDragChanges: NodeDragChange[] = []
  let pendingNodeDragFrameId: number | null = null
  let nativeRemoveHistoryBefore: FlowHistorySnapshot | null = null
  let isNativeRemoveCommitScheduled = false
  let pendingNativeRemoveNodeChanges: NodeChange[] = []
  let pendingNativeRemoveEdgeChanges: EdgeChange[] = []
  const scheduleMicrotask =
    options.scheduleMicrotask ?? ((callback: () => void) => void Promise.resolve().then(callback))
  const scheduleAnimationFrame =
    options.scheduleAnimationFrame ??
    ((callback: FrameRequestCallback) => window.requestAnimationFrame(callback))
  const cancelAnimationFrame =
    options.cancelAnimationFrame ?? ((handle: number) => window.cancelAnimationFrame(handle))

  function collectSelectedItemIds(...itemGroups: SelectableFlowItem[][]): Set<string> {
    const ids = new Set<string>()
    for (const items of itemGroups) {
      for (const item of items) {
        if (item.selected === true) ids.add(item.id)
      }
    }
    return ids
  }

  function preserveSelectedItems<T extends SelectableFlowItem>(
    nextItems: T[],
    selectedIds: Set<string>,
  ): T[] {
    let resolvedItems: T[] | null = null
    nextItems.forEach((item, index) => {
      const shouldSelect = selectedIds.has(item.id)
      if (item.selected === shouldSelect) return
      if (!resolvedItems) resolvedItems = [...nextItems]
      resolvedItems[index] = { ...item, selected: shouldSelect }
    })
    return resolvedItems ?? nextItems
  }

  function preserveCurrentSelection(nextGraph: { nodes: Node[]; edges: Edge[] }) {
    const selectedNodeIds = collectSelectedItemIds(options.nodes.value)
    const selectedEdgeIds = collectSelectedItemIds(options.edges.value)
    return {
      nodes: preserveSelectedItems(nextGraph.nodes, selectedNodeIds),
      edges: preserveSelectedItems(nextGraph.edges, selectedEdgeIds),
    }
  }

  function captureGraphCommandHistory(context: CanvasVueFlowGraphCommandContext) {
    const sideEffects = readFlowGraphCommandSideEffects(context.metadata)
    if (!sideEffects || sideEffects.recordHistory === false) return null
    return sideEffects.historyBefore ?? flowHistory.capture()
  }

  function commitGraphCommandSideEffects(
    result: CanvasVueFlowGraphCommandResult,
    historyBefore: unknown,
  ) {
    const sideEffects = readFlowGraphCommandSideEffects(result.metadata)
    if (!sideEffects) return
    let didRecordChange = result.didChange
    if (historyBefore) {
      didRecordChange = flowHistory.push(historyBefore as FlowHistorySnapshot)
    }
    if (sideEffects.markDirty !== false && didRecordChange) markDirty()
  }

  const graphAdapter: CanvasVueFlowGraphAdapter = useCanvasVueFlowGraphAdapter({
    nodes: options.nodes,
    edges: options.edges,
    actions: options.vueFlowGraphActions,
    beforeCommand: captureGraphCommandHistory,
    afterCommand: commitGraphCommandSideEffects,
  })

  function commitFlowGraph(
    nextGraph: { nodes: Node[]; edges: Edge[] },
    commitOptions: FlowGraphCommitOptions = {},
  ) {
    const resolvedGraph = commitOptions.preserveSelection
      ? preserveCurrentSelection(nextGraph)
      : nextGraph
    graphAdapter.replaceGraph(resolvedGraph)
  }

  function captureFullGraphHistory() {
    return flowHistory.capture()
  }

  function createRestoreGraphCommandOptions() {
    return createFlowGraphCommandOptions({
      recordHistory: false,
      markDirty: false,
    })
  }

  const flowHistory = useFlowHistory({
    nodes: options.nodes,
    edges: options.edges,
    maxSteps: 10,
    onRestore: () => {
      markDirty()
    },
    commitNodes: (nodes) => graphAdapter.replaceNodes(nodes as Node[], createRestoreGraphCommandOptions()),
    commitEdges: (edges) => graphAdapter.replaceEdges(edges as Edge[], createRestoreGraphCommandOptions()),
  })

  const boardPersistence = useFlowBoardPersistence({
    fullNodes: options.nodes,
    fullEdges: options.edges,
    boardBaseSnapshot: options.boardBaseSnapshot,
    isDirty: options.isDirty,
    saveStatus: options.saveStatus,
    isLoadingBoardSnapshot: options.isLoadingBoardSnapshot,
    isSavingBoard: options.isSavingBoard,
    boardSession: options.boardSession,
    onWarning: options.onWarning,
    onSuccess: options.onSuccess,
    onError: options.onError,
    clearFlowHistoryBaseline: flowHistory.clearBaseline,
    getViewport: options.getViewport,
    setViewport: options.setViewport,
    commitGraph: (graph, commitOptions) =>
      commitFlowGraph(
        {
          nodes: graph.nodes as Node[],
          edges: graph.edges as Edge[],
        },
        commitOptions,
      ),
  })

  const currentBoardId = boardPersistence.currentBoardId
  const loadInitialOrRouteBoard = boardPersistence.loadInitialOrRouteBoard
  const saveCurrentBoardSnapshot = boardPersistence.saveCurrentBoardSnapshot
  const canSaveBoard = boardPersistence.canSaveBoard

  const { markDirtyAndSchedule, flushAutosave } = useFlowAutosave({
    isDirty: options.isDirty,
    isSaving: options.isSavingBoard,
    canRun: () => !isNodeDragActive,
    save: (autosaveOptions) =>
      saveCurrentBoardSnapshot({ waitForPreview: false, silent: autosaveOptions?.silent }),
  })

  function markDirty() {
    if (!canSaveBoard.value) return
    options.isDirty.value = true
    if (options.saveStatus.value !== 'failed') options.saveStatus.value = 'idle'
    markDirtyAndSchedule()
  }

  function isCanvasGraphNodeId(nodeId: string): nodeId is CanvasGraphNodeId {
    return /^(?:image|prompt|fusion)-\d+$/.test(nodeId)
  }

  function isNodeDragChange(change: NodeChange): change is NodeDragChange {
    return (change.type === 'position' || change.type === 'select') && 'id' in change
  }

  function canQueueNodeDragChanges(changes: NodeChange[]): changes is NodeDragChange[] {
    return isNodeDragActive && changes.every(isNodeDragChange)
  }

  function createCurrentCanvasGraphStore() {
    const snapshot = flowToCanvasBoardSnapshot(
      {
        nodes: options.nodes.value,
        edges: options.edges.value as Parameters<typeof flowToCanvasBoardSnapshot>[0]['edges'],
      },
      options.boardBaseSnapshot.value,
    )
    return createCanvasGraphStore(snapshot)
  }

  function createNodeDragTransaction() {
    return createCurrentCanvasGraphStore().beginTransaction('node-drag')
  }

  function createTransactionCommandOptions(
    summary: CanvasGraphTransactionSummary,
    historyBefore?: FlowHistorySnapshot | null,
  ) {
    return createFlowGraphCommandOptions({
      historyBefore: summary.shouldRecordHistory ? historyBefore ?? undefined : undefined,
      recordHistory: summary.shouldRecordHistory,
      markDirty: summary.shouldMarkDirty,
    })
  }

  function stageNodeDragChange(change: NodeDragChange) {
    const transaction = activeNodeDrag?.transaction
    if (!transaction || !isCanvasGraphNodeId(change.id)) return
    if (change.type === 'position' && change.position) {
      transaction.updateNode(change.id, {
        x: change.position.x,
        y: change.position.y,
      })
      return
    }
    if (change.type === 'select') {
      transaction.selectNode(change.id, change.selected)
    }
  }

  function getNodeDragChangeKey(change: NodeDragChange) {
    return `${change.type}:${change.id}`
  }

  function mergeNodeDragChanges(
    current: NodeDragChange[],
    incoming: NodeDragChange[],
  ): NodeDragChange[] {
    const nextByKey = new Map<string, NodeDragChange>()
    current.forEach((change) => {
      nextByKey.set(getNodeDragChangeKey(change), change)
    })
    incoming.forEach((change) => {
      nextByKey.set(getNodeDragChangeKey(change), change)
    })
    return Array.from(nextByKey.values())
  }

  function clearPendingNodeDragFrame() {
    if (pendingNodeDragFrameId === null) return
    cancelAnimationFrame(pendingNodeDragFrameId)
    pendingNodeDragFrameId = null
  }

  function flushQueuedNodeDragChanges() {
    clearPendingNodeDragFrame()
    if (pendingNodeDragChanges.length === 0) return
    const changes = pendingNodeDragChanges
    pendingNodeDragChanges = []
    graphAdapter.projectNodeChanges(changes)
  }

  function syncProjectedNodeDragChanges() {
    if (latestNodeDragChanges.length === 0) return
    const changes = latestNodeDragChanges
    latestNodeDragChanges = []
    graphAdapter.syncProjectedNodeChanges(
      changes,
      createFlowGraphCommandOptions({
        recordHistory: false,
        markDirty: false,
      }),
    )
  }

  function queueNodeDragChanges(changes: NodeDragChange[]) {
    changes.forEach(stageNodeDragChange)
    pendingNodeDragChanges = mergeNodeDragChanges(pendingNodeDragChanges, changes)
    latestNodeDragChanges = mergeNodeDragChanges(latestNodeDragChanges, changes)
    if (pendingNodeDragFrameId !== null) return
    pendingNodeDragFrameId = scheduleAnimationFrame(() => {
      pendingNodeDragFrameId = null
      flushQueuedNodeDragChanges()
    })
  }

  function beginNodeDragTransaction() {
    isNodeDragActive = true
    activeNodeDrag = { transaction: createNodeDragTransaction() }
    flowHistory.onNodeDragStart()
  }

  function commitNodeDragTransaction() {
    try {
      flushQueuedNodeDragChanges()
      syncProjectedNodeDragChanges()
      activeNodeDrag?.transaction.commit()
      flowHistory.onNodeDragStop(() => {
        markDirty()
      })
    } finally {
      pendingNodeDragChanges = []
      latestNodeDragChanges = []
      clearPendingNodeDragFrame()
      activeNodeDrag = null
      isNodeDragActive = false
    }
  }

  function cancelNodeDragTransaction() {
    pendingNodeDragChanges = []
    latestNodeDragChanges = []
    clearPendingNodeDragFrame()
    activeNodeDrag?.transaction.cancel()
    activeNodeDrag = null
    isNodeDragActive = false
  }

  const graphRuntime: CanvasFlowGraphRuntimeControls = {
    captureHistory: captureFullGraphHistory,
    markDirty: () => {
      markDirty()
    },
    nodeDrag: {
      begin: beginNodeDragTransaction,
      commit: commitNodeDragTransaction,
      cancel: cancelNodeDragTransaction,
    },
  }

  function hasRemoveChange(changes: Array<NodeChange | EdgeChange>): boolean {
    return changes.some((change) => change.type === 'remove')
  }

  function dedupeRemoveChanges<T extends NodeChange | EdgeChange>(changes: T[]): T[] {
    const removedIds = new Set<string>()
    return changes.filter((change) => {
      if (change.type !== 'remove') return true
      if (removedIds.has(change.id)) return false
      removedIds.add(change.id)
      return true
    })
  }

  function createConnectedEdgeRemoveChanges(
    nodeChanges: NodeChange[],
    edgeChanges: EdgeChange[],
  ): EdgeChange[] {
    const removedNodeIds = new Set(
      nodeChanges
        .filter((change) => change.type === 'remove')
        .map((change) => change.id),
    )
    if (removedNodeIds.size === 0) return edgeChanges

    const removedEdgeIds = new Set(
      edgeChanges
        .filter((change) => change.type === 'remove')
        .map((change) => change.id),
    )
    const cleanupEdgeChanges = options.edges.value
      .filter(
        (edge) =>
          !removedEdgeIds.has(edge.id) &&
          (removedNodeIds.has(edge.source) || removedNodeIds.has(edge.target)),
      )
      .map(
        (edge): EdgeChange => ({
          id: edge.id,
          type: 'remove',
          source: edge.source,
          target: edge.target,
          sourceHandle: edge.sourceHandle ?? null,
          targetHandle: edge.targetHandle ?? null,
        }),
      )

    return cleanupEdgeChanges.length > 0
      ? edgeChanges.concat(cleanupEdgeChanges)
      : edgeChanges
  }

  function hasApplicableRemoveChanges(nodeChanges: NodeChange[], edgeChanges: EdgeChange[]) {
    const currentNodeIds = new Set(options.nodes.value.map((node) => node.id))
    const currentEdgeIds = new Set(options.edges.value.map((edge) => edge.id))
    return (
      nodeChanges.some((change) => change.type === 'remove' && currentNodeIds.has(change.id)) ||
      edgeChanges.some((change) => change.type === 'remove' && currentEdgeIds.has(change.id))
    )
  }

  function resolveNativeRemoveReason(
    nodeIds: CanvasGraphNodeId[],
    extraEdgeIds: CanvasGraphEdgeId[],
  ): CanvasGraphTransactionReason {
    if (nodeIds.length === 0) return extraEdgeIds.length <= 1 ? 'disconnect' : 'batch-delete'
    if (nodeIds.length === 1 && extraEdgeIds.length === 0) return 'delete'
    return 'batch-delete'
  }

  function isSameCanvasConnectionEndpoint(
    left: CanvasConnection,
    right: CanvasConnection,
  ) {
    return (
      left.fromNodeType === right.fromNodeType &&
      left.fromNodeId === right.fromNodeId &&
      left.fromPort === right.fromPort &&
      left.toNodeType === right.toNodeType &&
      left.toNodeId === right.toNodeId &&
      left.toPort === right.toPort
    )
  }

  function resolveGraphEdgeIdByEndpoint(
    store: ReturnType<typeof createCurrentCanvasGraphStore>,
    edge: EdgeLookupInput,
  ): CanvasGraphEdgeId | null {
    if (!edge.source || !edge.target) return null
    const canvasConnection = flowConnectionToCanvasConnection(1, {
      source: edge.source,
      target: edge.target,
      sourceHandle: edge.sourceHandle ?? null,
      targetHandle: edge.targetHandle ?? null,
    })
    if (!canvasConnection) return null
    return (
      store.getEdgeIds().find((edgeId) => {
        const record = store.getEdge(edgeId)
        return record
          ? isSameCanvasConnectionEndpoint(record.connection, canvasConnection)
          : false
      }) ?? null
    )
  }

  function resolveGraphEdgeIdForFlowEdge(
    store: ReturnType<typeof createCurrentCanvasGraphStore>,
    edge: EdgeLookupInput,
  ): CanvasGraphEdgeId | null {
    const restoredConnectionId = resolveFlowEdgeCanvasConnectionId(edge)
    if (restoredConnectionId !== null) {
      const edgeId = toCanvasGraphEdgeId(restoredConnectionId)
      if (store.getEdge(edgeId)) return edgeId
    }
    return resolveGraphEdgeIdByEndpoint(store, edge)
  }

  function createEdgeLookupInput(change: EdgeRemoveChange): EdgeLookupInput {
    const endpointChange = change as EdgeRemoveChange &
      Partial<Pick<Edge, 'source' | 'target' | 'sourceHandle' | 'targetHandle'>>
    return {
      id: change.id,
      data: undefined,
      source: endpointChange.source,
      target: endpointChange.target,
      sourceHandle: endpointChange.sourceHandle,
      targetHandle: endpointChange.targetHandle,
    }
  }

  function resolveNativeRemovedEdgeIds(
    store: ReturnType<typeof createCurrentCanvasGraphStore>,
    edgeChanges: EdgeChange[],
    removedNodeIds: Set<string>,
  ): CanvasGraphEdgeId[] | null {
    const edgeIds: CanvasGraphEdgeId[] = []
    for (const change of edgeChanges) {
      if (change.type !== 'remove') continue
      const edge = options.edges.value.find((item) => item.id === change.id)
      if (edge && (removedNodeIds.has(edge.source) || removedNodeIds.has(edge.target))) continue
      const edgeId = edge
        ? resolveGraphEdgeIdForFlowEdge(store, edge)
        : resolveGraphEdgeIdForFlowEdge(store, createEdgeLookupInput(change))
      if (!edgeId) return null
      edgeIds.push(edgeId)
    }
    return Array.from(new Set(edgeIds))
  }

  function commitNativeRemoveTransaction(
    historyBefore: FlowHistorySnapshot,
    nodeChanges: NodeChange[],
    edgeChanges: EdgeChange[],
  ): boolean {
    const nodeIds: CanvasGraphNodeId[] = []
    const removedNodeIds = new Set<string>()
    for (const change of nodeChanges) {
      if (change.type !== 'remove') continue
      if (!isCanvasGraphNodeId(change.id)) return false
      nodeIds.push(change.id)
      removedNodeIds.add(change.id)
    }

    const store = createCurrentCanvasGraphStore()
    const edgeIds = resolveNativeRemovedEdgeIds(store, edgeChanges, removedNodeIds)
    if (!edgeIds) return false

    const transaction = store.beginTransaction(resolveNativeRemoveReason(nodeIds, edgeIds))
    transaction.batchDelete({
      nodeIds,
      edgeIds,
      removeConnectedEdges: true,
    })
    const summary = transaction.commit()
    if (!summary.didChange) return true

    const result = graphAdapter.applyGraphChanges(
      { nodes: nodeChanges, edges: edgeChanges },
      createTransactionCommandOptions(summary, historyBefore),
    )
    if (result.didChange) options.onNativeRemove?.()
    return true
  }

  function commitNativeRemoveOperation() {
    isNativeRemoveCommitScheduled = false
    const historyBefore = nativeRemoveHistoryBefore
    const nodeChanges = dedupeRemoveChanges(pendingNativeRemoveNodeChanges)
    const edgeChanges = dedupeRemoveChanges(
      createConnectedEdgeRemoveChanges(nodeChanges, pendingNativeRemoveEdgeChanges),
    )
    nativeRemoveHistoryBefore = null
    pendingNativeRemoveNodeChanges = []
    pendingNativeRemoveEdgeChanges = []
    if (!historyBefore || !hasApplicableRemoveChanges(nodeChanges, edgeChanges)) return

    if (commitNativeRemoveTransaction(historyBefore, nodeChanges, edgeChanges)) {
      return
    }
    options.onWarning('删除操作未能应用，请刷新画布后重试')
  }

  function scheduleNativeRemoveCommit() {
    if (!nativeRemoveHistoryBefore) {
      nativeRemoveHistoryBefore = flowHistory.capture()
    }
    if (isNativeRemoveCommitScheduled) return
    isNativeRemoveCommitScheduled = true
    scheduleMicrotask(() => {
      scheduleMicrotask(commitNativeRemoveOperation)
    })
  }

  function onNodesChange(changes: NodeChange[]) {
    if (changes.length === 0) return
    if (canQueueNodeDragChanges(changes)) {
      queueNodeDragChanges(changes)
      return
    }
    flushQueuedNodeDragChanges()
    syncProjectedNodeDragChanges()
    if (hasRemoveChange(changes)) {
      pendingNativeRemoveNodeChanges = pendingNativeRemoveNodeChanges.concat(changes)
      scheduleNativeRemoveCommit()
      return
    }
    graphAdapter.applyNodeChanges(changes)
  }

  function onEdgesChange(changes: EdgeChange[]) {
    if (changes.length === 0) return
    if (hasRemoveChange(changes)) {
      pendingNativeRemoveEdgeChanges = pendingNativeRemoveEdgeChanges.concat(changes)
      scheduleNativeRemoveCommit()
      return
    }
    graphAdapter.applyEdgeChanges(changes)
  }

  return {
    currentBoardId,
    loadInitialOrRouteBoard,
    flushAutosave,
    flowHistory,
    graphRuntime,
    graphAdapter,
    onNodesChange,
    onEdgesChange,
    fullNodes: options.nodes,
    fullEdges: options.edges,
  }
}
