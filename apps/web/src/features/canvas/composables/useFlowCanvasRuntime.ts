import { computed, onScopeDispose, ref, shallowRef, watch, type Ref } from 'vue'
import type { Edge, EdgeChange, Node, NodeChange } from '@vue-flow/core'
import { useCanvasGraphHistory } from './useCanvasGraphHistory'
import {
  useCanvasBoardAutosave,
  useCanvasBoardPersistence,
  type CanvasGraphSnapshotReplaceReason,
  type SaveStatus,
} from '../board-session'
import {
  useCanvasVueFlowGraphAdapter,
  type CanvasVueFlowGraphActions,
  type CanvasVueFlowGraphAdapter,
} from './useCanvasVueFlowGraphAdapter'
import { createFlowGraphCommandOptions } from './flowGraphAdapterCommandSideEffects'
import { useCanvasGroupRuntime } from './useCanvasGroupRuntime'
import { bindCanvasGraphSessionProjection } from './bindCanvasGraphSessionProjection'
import type { CanvasBoardSnapshot } from '../../canvas-board/domain'
import type { CanvasBoardSession } from '../board-session'
import { canvasBoardSnapshotToFlow } from '../utils/flowSnapshotAdapter'
import { CanvasGraphSession, type CanvasGraphStore } from '../graph-runtime'
import {
  projectVisibleCanvasGraph,
  type CanvasProjectionViewport,
  type CanvasProjectionViewportSize,
} from './canvasVisibleGraphProjection'
import {
  createCanvasDeleteCommandLifecycle,
  createCanvasNodeDragLifecycle,
  type CanvasNodeDragLifecycle,
} from '../flow-pane'
import {
  CanvasBoardGraphSession,
  collectPendingImageTasks,
  projectPendingImageExecutionEdges,
  type PendingImageExecutionCore,
  type PendingImageExecutionLease,
} from '../execution/pending-execution'

type SelectableFlowItem = {
  id: string
  selected?: boolean
}

type FlowGraphCommitOptions = {
  source: 'hydrate-replace' | 'save-reconcile'
}

function createFlowEdgeEndpointKey(edge: Edge): string {
  return JSON.stringify([
    edge.source,
    edge.sourceHandle ?? null,
    edge.target,
    edge.targetHandle ?? null,
  ])
}

export type UseFlowCanvasRuntimeOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  renderNodes?: Ref<Node[]>
  renderEdges?: Ref<Edge[]>
  viewport?: Readonly<Ref<CanvasProjectionViewport>>
  viewportSize?: CanvasProjectionViewportSize
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
  const renderNodes = options.renderNodes ?? options.nodes
  const renderEdgesRef = options.renderEdges ?? options.edges
  const canonicalNodes = options.nodes
  const canonicalEdges = options.edges
  const localEditVersion = ref(0)
  let graphSession: CanvasGraphSession | null = null
  let unsubscribeGraphSessionCommits: (() => void) | null = null
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

  function readAlwaysVisibleNodeIds(): Set<string> {
    return new Set(
      executionSnapshot.value.executions
        .filter((execution) => execution.status === 'running')
        .map((execution) => execution.resultNodeId),
    )
  }

  function refreshVisibleProjection() {
    if (!options.viewport) {
      graphAdapter.replaceFullProjection({ nodes: canonicalNodes.value, edges: canonicalEdges.value })
      return
    }
    const visibleGraph = projectVisibleCanvasGraph(
      { nodes: canonicalNodes.value, edges: canonicalEdges.value },
      {
        viewport: options.viewport.value,
        viewportSize: options.viewportSize,
        alwaysIncludeNodeIds: readAlwaysVisibleNodeIds(),
      },
    )
    // Vue Flow can observe transient dangling edges when its imperative
    // add/remove methods are interleaved with controlled arrays. Replace the
    // bounded viewport projection atomically; the canonical graph stays full.
    graphAdapter.replaceVisibleProjection(visibleGraph)
  }

  let visibleProjectionFrame: number | null = null
  function scheduleVisibleProjection() {
    if (!options.viewport || visibleProjectionFrame !== null) return
    const schedule = options.scheduleAnimationFrame ?? requestAnimationFrame
    visibleProjectionFrame = schedule(() => {
      visibleProjectionFrame = null
      refreshVisibleProjection()
    })
  }

  if (options.viewport) {
    watch(options.viewport, scheduleVisibleProjection, { deep: true })
    onScopeDispose(() => {
      if (visibleProjectionFrame === null) return
      ;(options.cancelAnimationFrame ?? cancelAnimationFrame)(visibleProjectionFrame)
      visibleProjectionFrame = null
    })
  }

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

  function disposeGraphSession() {
    unsubscribeGraphSessionCommits?.()
    unsubscribeGraphSessionCommits = null
    graphSession?.dispose()
    graphSession = null
  }

  function requireGraphSession(): CanvasGraphSession {
    const boardId = options.boardSession.currentBoardId.value
    if (!graphSession || !boardId || graphSession.boardId !== boardId) {
      throw new Error('Canvas Graph Session is not attached to the current Board')
    }
    return graphSession
  }

  function readCanvasGraphStore(): CanvasGraphStore {
    return requireGraphSession().store
  }

  function syncCommittedGroups(session: CanvasGraphSession) {
    if (session !== graphSession || session.boardId !== options.boardSession.currentBoardId.value) return
    const groups = session.store
      .getGroupIds()
      .flatMap((groupId) => {
        const group = session.store.getGroup(groupId)
        return group ? [group] : []
      })
    options.boardBaseSnapshot.value = { ...options.boardBaseSnapshot.value, groups }
    groupRuntime?.internal.handleCommittedGroupsChanged()
  }

  function replaceGraphSnapshot(
    snapshot: CanvasBoardSnapshot,
    reason: CanvasGraphSnapshotReplaceReason,
  ) {
    const boardId = options.boardSession.currentBoardId.value
    if (!boardId) {
      disposeGraphSession()
      options.executionCore.detachBoard()
      return
    }

    if (reason === 'save-reconcile' && graphSession?.boardId === boardId) {
      graphSession.replace(snapshot)
      return
    }

    disposeGraphSession()
    const session = CanvasGraphSession.hydrate(boardId, snapshot)
    graphSession = session
    const unsubscribeHistory = session.subscribe((event) => {
      if (session !== graphSession || session.boardId !== options.boardSession.currentBoardId.value) {
        return
      }
      if (event.sideEffects.recordHistory) canvasGraphHistory.record(event.patch)
      if (event.sideEffects.markDirty) markDirty()
    })
    const unsubscribeProjection = bindCanvasGraphSessionProjection({
      session,
      graphAdapter,
      readCurrent: () => ({ nodes: canonicalNodes.value, edges: canonicalEdges.value }),
      projectFull: () => {
        const flow = canvasBoardSnapshotToFlow(session.exportSnapshot(options.boardBaseSnapshot.value))
        const nodes =
          groupRuntime?.internal.projectCurrentGroupNodeLayers(flow.nodes as Node[]) ??
          (flow.nodes as Node[])
        return { nodes, edges: flow.edges as Edge[] }
      },
      onProjection: (projection) => {
        if (!options.viewport) return false
        canonicalNodes.value = projection.graph.nodes
        canonicalEdges.value = projection.graph.edges
        refreshVisibleProjection()
        return true
      },
      isCurrentAttachment: () =>
        session === graphSession && session.boardId === options.boardSession.currentBoardId.value,
      readGroups: () =>
        session.store
          .getGroupIds()
          .flatMap((groupId) => {
            const group = session.store.getGroup(groupId)
            return group ? [group] : []
          }),
      onGroupsChanged: () => syncCommittedGroups(session),
      projectNode: (node) => groupRuntime?.internal.projectCommittedGroupNodeLayer(node) ?? node,
      scheduleMicrotask:
        options.scheduleMicrotask ??
        ((callback: () => void) => void Promise.resolve().then(callback)),
    })
    unsubscribeGraphSessionCommits = () => {
      unsubscribeProjection()
      unsubscribeHistory()
    }
    options.executionCore.attachBoard(
      CanvasBoardGraphSession.create(boardId, {
        graphSession: session,
      }),
    )
  }
  onScopeDispose(() => {
    disposeGraphSession()
    options.executionCore.detachBoard()
  })

  function rebaseSavedFlowProjection(savedGraph: { nodes: Node[]; edges: Edge[] }) {
    const savedNodesById = new Map(savedGraph.nodes.map((node) => [node.id, node] as const))
    const savedEdgesById = new Map(savedGraph.edges.map((edge) => [edge.id, edge] as const))
    const savedEdgeEndpointKeys = new Set(savedGraph.edges.map(createFlowEdgeEndpointKey))
    return {
      nodes: [
        ...savedGraph.nodes,
        ...canonicalNodes.value.filter(
          (node) => !savedNodesById.has(node.id) && isNodeExecutionActive(node.id),
        ),
      ],
      edges: [
        ...savedGraph.edges,
        ...canonicalEdges.value.filter(
          (edge) =>
            !savedEdgesById.has(edge.id) &&
            !savedEdgeEndpointKeys.has(createFlowEdgeEndpointKey(edge)) &&
            isNodeExecutionActive(edge.target),
        ),
      ],
    }
  }

  function hydratePendingExecutionLeases() {
    const attachment = options.executionCore.currentAttachment()
    if (!attachment) return
    collectPendingImageTasks(options.boardBaseSnapshot.value).forEach((pending) => {
      const reattached = options.executionCore.hydratePendingLease(attachment, {
        resultNodeId: pending.resultNodeId,
        taskId: pending.taskId,
        clientActionId: pending.clientActionId,
      })
      if (reattached.kind === 'started') options.cancelTasks(reattached.cancellableTaskIds)
    })
  }

  function commitFlowGraph(
    nextGraph: { nodes: Node[]; edges: Edge[] },
    commitOptions: FlowGraphCommitOptions,
  ) {
    if (commitOptions.source === 'hydrate-replace') {
      hydratePendingExecutionLeases()
    }
    const semanticGraph =
      commitOptions.source === 'save-reconcile' ? rebaseSavedFlowProjection(nextGraph) : nextGraph
    const projectedGraph = {
      nodes:
        groupRuntime?.internal.projectCurrentGroupNodeLayers(semanticGraph.nodes) ??
        semanticGraph.nodes,
      edges: semanticGraph.edges,
    }
    const resolvedGraph =
      commitOptions.source === 'save-reconcile'
        ? preserveCurrentSelection(projectedGraph)
        : projectedGraph
    if (!options.viewport) {
      graphAdapter.replaceGraph(resolvedGraph)
      return
    }
    canonicalNodes.value = resolvedGraph.nodes
    canonicalEdges.value = resolvedGraph.edges
    refreshVisibleProjection()
  }

  const canvasGraphHistory = useCanvasGraphHistory({
    maxSteps: 10,
    applyPatch: (patch, direction) => {
      const summary = requireGraphSession().applyCommitPatch(patch, direction, {
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
    replaceGraphSnapshot,
    exportGraphSnapshot: (baseSnapshot) => requireGraphSession().exportSnapshot(baseSnapshot),
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
    nodeDragLifecycle.cancel()
    groupRuntime?.internal.resetTransientSelectionState()
    disposeGraphSession()
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
