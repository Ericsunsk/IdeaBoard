import { computed, onScopeDispose, ref, shallowRef, type Ref } from 'vue'
import type { Edge, EdgeChange, Node, NodeChange } from '@vue-flow/core'
import { useCanvasGraphHistory } from '../../graph-runtime/application/useCanvasGraphHistory'
import { useCanvasBoardAutosave, useCanvasBoardSync, type SaveStatus } from '../../board-session'
import {
  useCanvasVueFlowGraphAdapter,
  type CanvasVueFlowGraphActions,
  type CanvasVueFlowGraphAdapter,
} from '../../vue-flow-adapter/useCanvasVueFlowGraphAdapter'
import {
  createCanvasGraphSessionRuntime,
  type CanvasGraphSessionRuntime,
} from './canvasGraphSessionRuntime'
import { useCanvasGroupRuntime } from '../../graph-runtime/application/useCanvasGroupRuntime'
import type { CanvasBoardSnapshot } from '../../../canvas-board/domain'
import { canvasBoardSnapshotToFlow } from '../../utils/flowSnapshotAdapter'
import type { CanvasBoardSession } from '../../board-session'
import {
  createCanvasDeleteCommandLifecycle,
  createCanvasNodeDragLifecycle,
  type CanvasNodeDragLifecycle,
} from '../../vue-flow-adapter'
import {
  projectPendingImageExecutionEdges,
  type PendingImageExecutionCore,
  type PendingImageExecutionLease,
} from '../../execution/pending-execution'
import type { CanvasRendererSelectableElement } from '../types'
import {
  preserveCanvasRendererSelection,
  stageCanvasRendererSelection,
} from './canvasSelectionCoordinator'

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
  hydratePendingImageLeases?: (snapshot: CanvasBoardSnapshot) => void
  cancelTasks: (taskIds: readonly string[]) => void
  onWarning: (message: string) => void
  onSuccess: (message: string) => void
  onError: (message: string) => void
  scheduleMicrotask?: (callback: () => void) => void
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
  function preserveCurrentSelection(nextGraph: { nodes: Node[]; edges: Edge[] }) {
    return preserveCanvasRendererSelection(
      { nodes: renderNodes.value, edges: renderEdgesRef.value },
      nextGraph,
    )
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
    const nextStates = new Map(
      next.executions.map((execution) => [execution.resultNodeId, execution.status]),
    )
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

  const boardSync = useCanvasBoardSync({
    buildCurrentSnapshot: () =>
      requireGraphSessionRuntime().exportGraphSnapshot(options.boardBaseSnapshot.value),
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
    canApplyReconciliation: () => !nodeDragLifecycle.isActive(),
  })

  const currentBoardId = boardSync.currentBoardId
  function applyBoardSnapshot(
    snapshot: CanvasBoardSnapshot,
    source: 'hydrate-replace' | 'save-reconcile',
    applyOptions: { clearHistory?: boolean } = {},
  ) {
    const graphRuntime = requireGraphSessionRuntime()
    options.boardBaseSnapshot.value = snapshot
    graphRuntime.replaceGraphSnapshot(
      snapshot,
      source === 'hydrate-replace' ? 'hydrate' : 'save-reconcile',
    )
    const flow = canvasBoardSnapshotToFlow(snapshot)
    graphRuntime.commitGraph({ nodes: flow.nodes as Node[], edges: flow.edges as Edge[] }, source)
    if (source === 'hydrate-replace' || applyOptions.clearHistory) {
      canvasGraphHistory.clearBaseline()
    }
  }

  const loadInitialOrRouteBoard = async () => {
    nodeDragLifecycle.cancel()
    groupRuntime?.internal.resetTransientSelectionState()
    graphSessionRuntime?.dispose()
    options.executionCore.detachBoard()
    const result = await boardSync.loadInitialOrRouteBoard()
    if (result.status === 'empty' || result.status === 'loaded') {
      applyBoardSnapshot(result.snapshot, 'hydrate-replace')
    }
    groupRuntime?.internal.handleCommittedGroupsChanged()
    return result
  }
  const saveCurrentBoardSnapshot = async (
    saveOptions: {
      waitForPreview?: boolean
      silent?: boolean
    } = {},
  ) => {
    const result = await boardSync.saveCurrentBoardSnapshot(saveOptions)
    if (result.reconciliation) {
      applyBoardSnapshot(result.reconciliation.snapshot, 'save-reconcile', {
        clearHistory: !result.reconciliation.hasLocalDraftAdvanced,
      })
    }
    return result.saved
  }
  const canSaveBoard = boardSync.canSaveBoard

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
    isGraphSessionAttached: () => graphSessionRuntime?.isAttached() ?? false,
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
    onBegin: groupRuntime.internal.onNodeDragStart,
    reconcileMembership: groupRuntime.internal.applyDraggedNodeMembershipChanges,
  })

  graphSessionRuntime = createCanvasGraphSessionRuntime({
    nodes: renderNodes,
    edges: renderEdgesRef,
    boardBaseSnapshot: options.boardBaseSnapshot,
    boardSession: options.boardSession,
    graphAdapter,
    executionCore: options.executionCore,
    hydratePendingImageLeases: options.hydratePendingImageLeases,
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
    const dragChanges = nodeDragLifecycle.handleChanges(changes)
    if (dragChanges) {
      graphAdapter.syncControlledNodeChanges(dragChanges.consumed)

      const selectionChanges = dragChanges.remaining.filter(
        (change): change is Extract<NodeChange, { type: 'select' }> => change.type === 'select',
      )
      groupRuntime?.internal.onBeforeNodesChange(selectionChanges)
      graphAdapter.syncControlledNodeChanges(selectionChanges)

      const remainingGraphChanges = dragChanges.remaining.filter(
        (change) => change.type !== 'select',
      )
      if (remainingGraphChanges.length > 0) {
        groupRuntime?.internal.onBeforeNodesChange(remainingGraphChanges)
        if (!deleteLifecycle.handleNodeChanges(remainingGraphChanges)) {
          graphAdapter.applyNodeChanges(remainingGraphChanges)
        }
      }
      return
    }

    const onlyVisualChanges = changes.every(
      (change) => change.type === 'position' || change.type === 'select',
    )
    if (nodeDragLifecycle.isActive() && onlyVisualChanges) {
      const selectionChanges = changes.filter(
        (change): change is Extract<NodeChange, { type: 'select' }> => change.type === 'select',
      )
      groupRuntime?.internal.onBeforeNodesChange(selectionChanges)
      graphAdapter.syncControlledNodeChanges(changes)
      return
    }

    groupRuntime?.internal.onBeforeNodesChange(changes)
    if (deleteLifecycle.handleNodeChanges(changes)) return
    graphAdapter.applyNodeChanges(changes)
    if (changes.some((change) => change.type === 'select')) {
      reconcileRendererSelection([...renderNodes.value, ...renderEdgesRef.value])
    }
  }

  function onEdgesChange(changes: EdgeChange[]) {
    if (changes.length === 0) return
    groupRuntime?.internal.onBeforeEdgesChange(changes)
    if (deleteLifecycle.handleEdgeChanges(changes)) return
    graphAdapter.applyEdgeChanges(changes)
    if (changes.some((change) => change.type === 'select')) {
      reconcileRendererSelection([...renderNodes.value, ...renderEdgesRef.value])
    }
  }

  function reconcileRendererSelection(elements: CanvasRendererSelectableElement[]) {
    // Vue Flow owns renderer selection. During a drag the pending graph
    // transaction is still based on the pre-drag revision, so selection must
    // join that transaction at commit instead of opening a competing one.
    if (nodeDragLifecycle.isActive()) return

    const store = readCanvasGraphStore()
    const transaction = store.beginTransaction('node-update', {
      recordHistory: false,
      markDirty: false,
    })
    stageCanvasRendererSelection(store, transaction, elements)
    transaction.commit()
  }

  function commitNodeDragTransaction() {
    nodeDragLifecycle.commit((store, transaction) =>
      stageCanvasRendererSelection(store, transaction, [
        ...renderNodes.value,
        ...renderEdgesRef.value,
      ]),
    )
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
      reconcileRendererSelection,
      nodeDragTransactions: {
        begin: nodeDragLifecycle.begin,
        commit: commitNodeDragTransaction,
      },
    },
    graphEditing: {
      adapter: graphAdapter,
      readCanvasGraphStore,
      isGraphSessionAttached: () => graphSessionRuntime?.isAttached() ?? false,
      deleteNode,
      grouping: groupRuntime,
    },
  }
}
