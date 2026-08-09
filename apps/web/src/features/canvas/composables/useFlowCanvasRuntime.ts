import { ref, type Ref } from 'vue'
import type { Edge, EdgeChange, Node, NodeChange } from '@vue-flow/core'
import { useFlowHistory, type FlowHistorySnapshot } from './useFlowHistory'
import {
  useCanvasBoardAutosave,
  useCanvasBoardPersistence,
  type SaveStatus,
} from '../board-session'
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
import { applyFlowGraphDeleteProjection } from './flowGraphDeleteProjection'
import { useCanvasGroupRuntime, type CanvasGroupRuntimeControls } from './useCanvasGroupRuntime'
import type { CanvasBoardSnapshot } from '../../canvas-board/domain'
import type { CanvasBoardSession } from '../board-session'
import {
  flowToCanvasBoardSnapshot,
  type FlowToCanvasBoardSnapshotOptions,
} from '../utils/flowSnapshotAdapter'
import {
  commitCanvasGraphDeleteIntent,
  createCanvasGraphStore,
  isCanvasGraphNodeId,
  type CanvasGraphEdgeId,
} from '../graph-runtime'
import {
  createCanvasNodeDragLifecycle,
  type CanvasNodeDragLifecycle,
} from '../flow-pane'
import {
  createFlowEdgeProjectionIndex,
  createFlowEdgeIdentityFromRemoveChange,
  type FlowEdgeIdentity,
  type FlowEdgeProjectionIndex,
} from './flowConnectionIdentity'

type SelectableFlowItem = {
  id: string
  selected?: boolean
}

type FlowGraphCommitOptions = {
  preserveSelection?: boolean
}

export type { CanvasSelectionGroupAction } from './useCanvasGroupRuntime'

type EdgeRemoveChange = Extract<EdgeChange, { type: 'remove' }>

export type CanvasFlowGraphRuntimeControls = {
  captureHistory: () => FlowHistorySnapshot
  createCurrentCanvasGraphStore: (
    snapshotOptions?: FlowToCanvasBoardSnapshotOptions,
  ) => ReturnType<typeof createCanvasGraphStore>
  deleteNode: (nodeId: string) => boolean
  markDirty: () => void
  grouping: CanvasGroupRuntimeControls
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
  autosaveResumeSignal?: Ref<unknown>
  boardSession: CanvasBoardSession
  onWarning: (message: string) => void
  onSuccess: (message: string) => void
  onError: (message: string) => void
  onNativeRemove?: () => void
  stopDeletedRunningTasks?: (taskIds: string[]) => void
  scheduleMicrotask?: (callback: () => void) => void
  scheduleAnimationFrame?: (callback: FrameRequestCallback) => number
  cancelAnimationFrame?: (handle: number) => void
  vueFlowGraphActions?: CanvasVueFlowGraphActions
}

export function useFlowCanvasRuntime(options: UseFlowCanvasRuntimeOptions) {
  let nativeRemoveHistoryBefore: FlowHistorySnapshot | null = null
  let isNativeRemoveCommitScheduled = false
  let pendingNativeRemoveNodeChanges: NodeChange[] = []
  let pendingNativeRemoveEdgeChanges: EdgeChange[] = []
  const scheduleMicrotask =
    options.scheduleMicrotask ?? ((callback: () => void) => void Promise.resolve().then(callback))
  const localEditVersion = ref(0)
  const historyGroups: { value: unknown[] } = {
    get value() {
      return options.boardBaseSnapshot.value.groups
    },
    set value(nextGroups) {
      commitHistoryGroups(nextGroups)
    },
  }

  function commitHistoryGroups(groups: unknown[]) {
    options.boardBaseSnapshot.value = {
      ...options.boardBaseSnapshot.value,
      groups: groups as CanvasBoardSnapshot['groups'],
    }
    groupRuntime?.internal.handleCommittedGroupsChanged()
  }

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
    const projectedGraph = {
      nodes:
        groupRuntime?.internal.projectCurrentGroupNodeLayers(nextGraph.nodes) ?? nextGraph.nodes,
      edges: nextGraph.edges,
    }
    const resolvedGraph = commitOptions.preserveSelection
      ? preserveCurrentSelection(projectedGraph)
      : projectedGraph
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
    commitNodes: (nodes) =>
      graphAdapter.replaceNodes(nodes as Node[], createRestoreGraphCommandOptions()),
    commitEdges: (edges) =>
      graphAdapter.replaceEdges(edges as Edge[], createRestoreGraphCommandOptions()),
    groups: historyGroups,
    commitGroups: commitHistoryGroups,
  })

  const boardPersistence = useCanvasBoardPersistence({
    fullNodes: options.nodes,
    fullEdges: options.edges,
    boardBaseSnapshot: options.boardBaseSnapshot,
    isDirty: options.isDirty,
    saveStatus: options.saveStatus,
    localEditVersion,
    isLoadingBoardSnapshot: options.isLoadingBoardSnapshot,
    isSavingBoard: options.isSavingBoard,
    boardSession: options.boardSession,
    onWarning: options.onWarning,
    onSuccess: options.onSuccess,
    onError: options.onError,
    clearFlowHistoryBaseline: flowHistory.clearBaseline,
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
  const loadInitialOrRouteBoardBase = boardPersistence.loadInitialOrRouteBoard
  const loadInitialOrRouteBoard: typeof loadInitialOrRouteBoardBase = async (...args) => {
    groupRuntime?.internal.resetTransientSelectionState()
    const result = await loadInitialOrRouteBoardBase(...args)
    groupRuntime?.internal.handleCommittedGroupsChanged()
    return result
  }
  const saveCurrentBoardSnapshot = boardPersistence.saveCurrentBoardSnapshot
  const canSaveBoard = boardPersistence.canSaveBoard

  const { markDirtyAndSchedule, flushAutosave } = useCanvasBoardAutosave({
    isDirty: options.isDirty,
    isSaving: options.isSavingBoard,
    resumeSignal: options.autosaveResumeSignal,
    canRun: () => !nodeDragLifecycle.isActive(),
    save: (autosaveOptions) =>
      saveCurrentBoardSnapshot({ waitForPreview: false, silent: autosaveOptions?.silent }),
  })

  function markDirty() {
    if (!canSaveBoard.value) return
    localEditVersion.value += 1
    options.isDirty.value = true
    if (options.saveStatus.value === 'idle' || options.saveStatus.value === 'saved') {
      options.saveStatus.value = 'pending'
    }
    markDirtyAndSchedule()
  }

  function advanceLocalEditVersion() {
    if (!canSaveBoard.value) return
    localEditVersion.value += 1
  }

  function createCurrentCanvasGraphStore(snapshotOptions?: FlowToCanvasBoardSnapshotOptions) {
    const snapshot = flowToCanvasBoardSnapshot(
      {
        nodes: options.nodes.value,
        edges: options.edges.value as Parameters<typeof flowToCanvasBoardSnapshot>[0]['edges'],
      },
      options.boardBaseSnapshot.value,
      snapshotOptions,
    )
    return createCanvasGraphStore(snapshot)
  }

  const groupRuntime = useCanvasGroupRuntime({
    nodes: options.nodes,
    edges: options.edges,
    boardBaseSnapshot: options.boardBaseSnapshot,
    graphAdapter,
    flowHistory,
    createCurrentCanvasGraphStore,
    commitGroups: commitHistoryGroups,
    markDirty,
    onWarning: options.onWarning,
    onNativeRemove: options.onNativeRemove,
    stopDeletedRunningTasks: options.stopDeletedRunningTasks,
  })

  const nodeDragLifecycle: CanvasNodeDragLifecycle = createCanvasNodeDragLifecycle({
    createTransaction: () => createCurrentCanvasGraphStore().beginTransaction('node-drag'),
    projectChanges: (changes) => graphAdapter.projectNodeChanges(changes),
    syncChanges: (changes) =>
      graphAdapter.syncProjectedNodeChanges(
        changes,
        createFlowGraphCommandOptions({
          recordHistory: false,
          markDirty: false,
        }),
      ),
    captureHistory: flowHistory.capture,
    commitHistory: flowHistory.push,
    advanceLocalEditVersion,
    markDirtyAndScheduleAutosave: markDirty,
    onBegin: groupRuntime.internal.onNodeDragStart,
    reconcileMembership: groupRuntime.internal.applyDraggedNodeMembershipChanges,
    scheduleAnimationFrame: options.scheduleAnimationFrame,
    cancelAnimationFrame: options.cancelAnimationFrame,
  })

  function deleteNode(nodeId: string): boolean {
    if (!options.nodes.value.some((node) => node.id === nodeId)) return false
    if (!isCanvasGraphNodeId(nodeId)) {
      options.onWarning('删除操作未能写入画布，请刷新后重试')
      return false
    }

    const store = createCurrentCanvasGraphStore()
    if (!store.getNode(nodeId)) {
      options.onWarning('删除操作未能写入画布，请刷新后重试')
      return false
    }

    const historyBefore = captureFullGraphHistory()
    const projectionIndex = createFlowEdgeProjectionIndex(store, options.edges.value)
    const summary = commitCanvasGraphDeleteIntent(store, {
      nodeIds: [nodeId],
      removeConnectedEdges: true,
    })
    if (!summary.didChange) return false

    const snapshot = store.exportSnapshot(options.boardBaseSnapshot.value)
    commitHistoryGroups(snapshot.groups)
    const result = applyFlowGraphDeleteProjection({
      graphAdapter,
      nodes: options.nodes.value,
      edges: options.edges.value,
      commitPolicy: { summary, historyBefore },
      flowNodeIds: [nodeId],
      projectionIndex,
      onNativeRemove: options.onNativeRemove,
      stopDeletedRunningTasks: options.stopDeletedRunningTasks,
    })
    return result?.didChange ?? false
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

  function hasApplicableRemoveChanges(nodeChanges: NodeChange[], edgeChanges: EdgeChange[]) {
    const currentNodeIds = new Set(options.nodes.value.map((node) => node.id))
    const currentEdgeIds = new Set(options.edges.value.map((edge) => edge.id))
    return (
      nodeChanges.some((change) => change.type === 'remove' && currentNodeIds.has(change.id)) ||
      edgeChanges.some((change) => change.type === 'remove' && currentEdgeIds.has(change.id))
    )
  }

  function resolveNativeRemovedEdgeIds(
    projectionIndex: FlowEdgeProjectionIndex,
    edgeChanges: EdgeChange[],
    removedNodeIds: Set<string>,
  ): CanvasGraphEdgeId[] | null {
    const edgeIds: CanvasGraphEdgeId[] = []
    for (const change of edgeChanges) {
      if (change.type !== 'remove') continue
      const edge = projectionIndex.getFlowEdge(change.id)
      if (edge && (removedNodeIds.has(edge.source) || removedNodeIds.has(edge.target))) continue
      const edgeIdentity: FlowEdgeIdentity =
        edge ?? createFlowEdgeIdentityFromRemoveChange(change as EdgeRemoveChange)
      const edgeId = projectionIndex.resolveCanvasEdgeId(edgeIdentity)
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
    const removedNodes = groupRuntime?.internal.resolveRemovedFlowNodeChanges(nodeChanges) ?? null
    if (!removedNodes) return false
    const nodeIds = removedNodes.graphNodeIds
    const flowNodeIds = removedNodes.flowNodeIds
    const removedNodeIds = new Set<string>()
    flowNodeIds.forEach((flowNodeId) => removedNodeIds.add(flowNodeId))
    nodeIds.forEach((graphNodeId) => removedNodeIds.add(graphNodeId))

    const store = createCurrentCanvasGraphStore()
    const projectionIndex = createFlowEdgeProjectionIndex(store, options.edges.value)
    const edgeIds = resolveNativeRemovedEdgeIds(projectionIndex, edgeChanges, removedNodeIds)
    if (!edgeIds) return false

    const summary = commitCanvasGraphDeleteIntent(store, {
      nodeIds,
      edgeIds,
      removeConnectedEdges: true,
    })
    if (!summary.didChange) return true

    const snapshot = store.exportSnapshot(options.boardBaseSnapshot.value)
    commitHistoryGroups(snapshot.groups)
    applyFlowGraphDeleteProjection({
      graphAdapter,
      nodes: options.nodes.value,
      edges: options.edges.value,
      commitPolicy: { summary, historyBefore },
      flowNodeIds,
      projectionIndex,
      edgeRemoveChanges: edgeChanges,
      onNativeRemove: options.onNativeRemove,
      stopDeletedRunningTasks: options.stopDeletedRunningTasks,
    })
    return true
  }

  function commitNativeRemoveOperation() {
    isNativeRemoveCommitScheduled = false
    const historyBefore = nativeRemoveHistoryBefore
    const nodeChanges = dedupeRemoveChanges(pendingNativeRemoveNodeChanges)
    const edgeChanges = dedupeRemoveChanges(pendingNativeRemoveEdgeChanges)
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
    groupRuntime?.internal.onBeforeNodesChange(changes)
    if (nodeDragLifecycle.handleChanges(changes)) return
    if (hasRemoveChange(changes)) {
      pendingNativeRemoveNodeChanges = pendingNativeRemoveNodeChanges.concat(changes)
      scheduleNativeRemoveCommit()
      return
    }
    graphAdapter.applyNodeChanges(changes)
  }

  function onEdgesChange(changes: EdgeChange[]) {
    if (changes.length === 0) return
    groupRuntime?.internal.onBeforeEdgesChange(changes)
    if (hasRemoveChange(changes)) {
      pendingNativeRemoveEdgeChanges = pendingNativeRemoveEdgeChanges.concat(changes)
      scheduleNativeRemoveCommit()
      return
    }
    graphAdapter.applyEdgeChanges(changes)
  }

  const graphRuntime: CanvasFlowGraphRuntimeControls = {
    captureHistory: captureFullGraphHistory,
    createCurrentCanvasGraphStore,
    deleteNode,
    markDirty: () => {
      markDirty()
    },
    grouping: groupRuntime,
    nodeDrag: {
      begin: nodeDragLifecycle.begin,
      commit: nodeDragLifecycle.commit,
      cancel: nodeDragLifecycle.cancel,
    },
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
