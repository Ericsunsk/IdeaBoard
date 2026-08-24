import { computed, onScopeDispose, ref, shallowRef, type Ref } from 'vue'
import type { Edge, EdgeChange, Node, NodeChange } from '@vue-flow/core'
import { useCanvasGraphHistory } from './useCanvasGraphHistory'
import {
  useCanvasBoardAutosave,
  useCanvasBoardPersistence,
  type SaveStatus,
} from '../board-session'
import {
  useCanvasVueFlowGraphAdapter,
  type CanvasVueFlowGraphActions,
  type CanvasVueFlowGraphAdapter,
} from './useCanvasVueFlowGraphAdapter'
import {
  createCanvasGraphSessionRuntime,
  type CanvasGraphSessionRuntime,
} from './canvasGraphSessionRuntime'
import { createFlowGraphCommandOptions } from './flowGraphAdapterCommandSideEffects'
import { useCanvasGroupRuntime } from './useCanvasGroupRuntime'
import type { CanvasBoardSnapshot } from '../../canvas-board/domain'
import type { CanvasBoardSession } from '../board-session'
import {
  createCanvasDeleteCommandLifecycle,
  createCanvasNodeDragLifecycle,
  type CanvasNodeDragLifecycle,
} from '../flow-pane'
import {
  projectPendingImageExecutionEdges,
  type PendingImageExecutionCore,
  type PendingImageExecutionLease,
} from '../execution/pending-execution'

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
  executionCore: PendingImageExecutionCore
  cancelTasks: (taskIds: readonly string[]) => void
  onWarning: (message: string) => void
  onSuccess: (message: string) => void
  onError: (message: string) => void
  scheduleMicrotask?: (callback: () => void) => void
  scheduleAnimationFrame?: (callback: FrameRequestCallback) => number
  cancelAnimationFrame?: (handle: number) => void
  vueFlowGraphActions?: CanvasVueFlowGraphActions
}

export function useFlowCanvasRuntime(options: UseFlowCanvasRuntimeOptions) {
  const renderNodes = options.nodes
  const renderEdgesRef = options.edges
  const localEditVersion = ref(0)
  let graphSessionRuntime: CanvasGraphSessionRuntime | null = null
  const executionSnapshot = shallowRef(options.executionCore.snapshot())
  const projectEdgesForRendering = (edges: Edge[]) =>
    projectPendingImageExecutionEdges(edges, executionSnapshot.value).edges
  const renderEdges = computed(() => projectEdgesForRendering(renderEdgesRef.value))
  const isNodeExecutionActive = (nodeId: string) =>
    options.executionCore.captureActiveLeases([nodeId as `image-${number}`]).length > 0
  const releaseNodeExecutions = (leases: readonly PendingImageExecutionLease[]) => {
    const release = options.executionCore.release(leases)
    options.cancelTasks(release.cancellableTaskIds)
  }
  function collectSelectedItemIds(
    ...itemGroups: Array<Array<{ id: string; selected?: boolean }>>
  ): Set<string> {
    const ids = new Set<string>()
    for (const items of itemGroups) {
      for (const item of items) {
        if (item.selected === true) ids.add(item.id)
      }
    }
    return ids
  }

  function preserveSelectedItems<T extends { id: string; selected?: boolean }>(
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
    const selectedNodeIds = collectSelectedItemIds(renderNodes.value)
    const selectedEdgeIds = collectSelectedItemIds(renderEdgesRef.value)
    return {
      nodes: preserveSelectedItems(nextGraph.nodes, selectedNodeIds),
      edges: preserveSelectedItems(nextGraph.edges, selectedEdgeIds),
    }
  }

  const graphAdapter: CanvasVueFlowGraphAdapter = useCanvasVueFlowGraphAdapter({
    nodes: renderNodes,
    edges: renderEdgesRef,
    actions: options.vueFlowGraphActions,
    projectEdgesForActions: projectEdgesForRendering,
  })

  function changedExecutionResultNodeIds(
    previous: typeof executionSnapshot.value,
    next: typeof executionSnapshot.value,
  ): Set<string> {
    const previousStates = new Map(
      previous.executions.map((execution) => [execution.resultNodeId, execution.status]),
    )
    const nextStates = new Map(next.executions.map((execution) => [execution.resultNodeId, execution.status]))
    const changed = new Set<string>()
    for (const [nodeId, status] of nextStates) {
      if (previousStates.get(nodeId) !== status) changed.add(nodeId)
    }
    for (const nodeId of previousStates.keys()) {
      if (!nextStates.has(nodeId)) changed.add(nodeId)
    }
    return changed
  }

  const unsubscribeExecutionSnapshot = options.executionCore.subscribe((snapshot) => {
    const previous = executionSnapshot.value
    executionSnapshot.value = snapshot
    const changedResultNodeIds = changedExecutionResultNodeIds(previous, snapshot)
    if (changedResultNodeIds.size === 0) return
    const affectedEdges = renderEdgesRef.value.filter(
      (edge) => edge.targetHandle === 'in:image' && changedResultNodeIds.has(edge.target),
    )
    if (affectedEdges.length === 0) return
    graphAdapter.applyIncrementalProjection({
      edges: [...renderEdgesRef.value],
      changedEdgeIds: affectedEdges.map((edge) => edge.id),
      edgeMutations: { added: [], updated: affectedEdges, removedIds: [] },
    })
  })
  onScopeDispose(unsubscribeExecutionSnapshot)

  function requireGraphSessionRuntime(): CanvasGraphSessionRuntime {
    if (!graphSessionRuntime) throw new Error('Canvas Graph Session runtime is not initialized')
    return graphSessionRuntime
  }

  const readCanvasGraphStore = () => requireGraphSessionRuntime().readCanvasGraphStore()

  const canvasGraphHistory = useCanvasGraphHistory({
    maxSteps: 10,
    applyPatch: (patch, direction) => {
      const summary = readCanvasGraphStore().applyCommitPatch(patch, direction, {
        recordHistory: false,
        markDirty: true,
      })
      return summary.didChange
    },
  })

  const boardPersistence = useCanvasBoardPersistence({
    boardBaseSnapshot: options.boardBaseSnapshot,
    isDirty: options.isDirty,
    saveStatus: options.saveStatus,
    graphRevision: localEditVersion,
    isLoadingBoardSnapshot: options.isLoadingBoardSnapshot,
    isSavingBoard: options.isSavingBoard,
    boardSession: options.boardSession,
    readBoardAttachmentEpoch: () => options.executionCore.snapshot().attachment?.epoch ?? 0,
    onWarning: options.onWarning,
    onSuccess: options.onSuccess,
    onError: options.onError,
    clearGraphHistoryBaseline: canvasGraphHistory.clearBaseline,
    canReconcileGraph: () => !nodeDragLifecycle.isActive(),
    replaceGraphSnapshot: (snapshot, reason) =>
      requireGraphSessionRuntime().replaceGraphSnapshot(snapshot, reason),
    exportGraphSnapshot: (baseSnapshot) =>
      requireGraphSessionRuntime().exportGraphSnapshot(baseSnapshot),
    commitGraph: (graph, commitOptions) =>
      requireGraphSessionRuntime().commitGraph(
        {
          nodes: graph.nodes as Node[],
          edges: graph.edges as Edge[],
        },
        commitOptions.source,
      ),
  })

  const currentBoardId = boardPersistence.currentBoardId
  const loadInitialOrRouteBoardBase = boardPersistence.loadInitialOrRouteBoard
  const loadInitialOrRouteBoard: typeof loadInitialOrRouteBoardBase = async (...args) => {
    nodeDragLifecycle.cancel()
    groupRuntime?.internal.resetTransientSelectionState()
    graphSessionRuntime?.dispose()
    options.executionCore.detachBoard()
    await loadInitialOrRouteBoardBase(...args)
    groupRuntime?.internal.handleCommittedGroupsChanged()
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

  const groupRuntime = useCanvasGroupRuntime({
    nodes: renderNodes,
    edges: renderEdgesRef,
    boardBaseSnapshot: options.boardBaseSnapshot,
    graphAdapter,
    readCanvasGraphStore,
    onWarning: options.onWarning,
    captureActiveNodeExecutions: options.executionCore.captureActiveLeases,
    releaseNodeExecutions,
  })

  const nodeDragLifecycle: CanvasNodeDragLifecycle = createCanvasNodeDragLifecycle({
    createCommand: () => {
      const store = readCanvasGraphStore()
      return {
        store,
        transaction: store.beginTransaction('node-drag'),
      }
    },
    projectChanges: (changes) => graphAdapter.projectNodeChanges(changes),
    syncChanges: (changes) =>
      graphAdapter.syncProjectedNodeChanges(
        changes,
        createFlowGraphCommandOptions({
          recordHistory: false,
          markDirty: false,
        }),
      ),
    onBegin: groupRuntime.internal.onNodeDragStart,
    reconcileMembership: groupRuntime.internal.applyDraggedNodeMembershipChanges,
    scheduleAnimationFrame: options.scheduleAnimationFrame,
    cancelAnimationFrame: options.cancelAnimationFrame,
  })

  graphSessionRuntime = createCanvasGraphSessionRuntime({
    nodes: renderNodes,
    edges: renderEdgesRef,
    boardBaseSnapshot: options.boardBaseSnapshot,
    boardSession: options.boardSession,
    graphAdapter,
    executionCore: options.executionCore,
    cancelTasks: options.cancelTasks,
    history: canvasGraphHistory,
    markDirty,
    isNodeExecutionActive,
    preserveCurrentSelection,
    projectCurrentGroupNodeLayers: (nodes) =>
      groupRuntime.internal.projectCurrentGroupNodeLayers(nodes),
    projectCommittedGroupNodeLayer: (node) =>
      groupRuntime.internal.projectCommittedGroupNodeLayer(node),
    onGroupsChanged: (groups) => {
      options.boardBaseSnapshot.value = { ...options.boardBaseSnapshot.value, groups }
      groupRuntime.internal.handleCommittedGroupsChanged()
    },
    scheduleMicrotask:
      options.scheduleMicrotask ??
      ((callback: () => void) => void Promise.resolve().then(callback)),
  })

  const deleteLifecycle = createCanvasDeleteCommandLifecycle({
    readNodes: () => renderNodes.value,
    readEdges: () => renderEdgesRef.value,
    readGraphStore: readCanvasGraphStore,
    resolveRemovedFlowNodes: groupRuntime.internal.resolveRemovedFlowNodeChanges,
    scheduleMicrotask:
      options.scheduleMicrotask ??
      ((callback: () => void) => void Promise.resolve().then(callback)),
    onWarning: options.onWarning,
    captureActiveNodeExecutions: options.executionCore.captureActiveLeases,
    releaseNodeExecutions,
  })

  function deleteNode(nodeId: string): boolean {
    if (!renderNodes.value.some((node) => node.id === nodeId)) return false
    return deleteLifecycle.deleteNode(nodeId)
  }

  function onNodesChange(changes: NodeChange[]) {
    if (changes.length === 0) return
    groupRuntime?.internal.onBeforeNodesChange(changes)
    if (nodeDragLifecycle.handleChanges(changes)) return
    if (deleteLifecycle.handleNodeChanges(changes)) return
    graphAdapter.applyNodeChanges(changes)
  }

  function onEdgesChange(changes: EdgeChange[]) {
    if (changes.length === 0) return
    groupRuntime?.internal.onBeforeEdgesChange(changes)
    if (deleteLifecycle.handleEdgeChanges(changes)) return
    graphAdapter.applyEdgeChanges(changes)
  }

  return {
    persistence: {
      currentBoardId,
      loadInitialOrRouteBoard,
      flushAutosave,
      history: canvasGraphHistory,
    },
    flowPane: {
      nodes: renderNodes,
      edges: renderEdgesRef,
      renderEdges,
      onNodesChange,
      onEdgesChange,
      nodeDragTransactions: {
        begin: nodeDragLifecycle.begin,
        commit: nodeDragLifecycle.commit,
      },
    },
    graphEditing: {
      adapter: graphAdapter,
      readCanvasGraphStore,
      deleteNode,
      grouping: groupRuntime,
    },
  }
}
