import { ref, watch, type Ref } from 'vue'
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
import { useCanvasGroupRuntime } from './useCanvasGroupRuntime'
import type { CanvasBoardSnapshot } from '../../canvas-board/domain'
import type { CanvasBoardSession } from '../board-session'
import {
  flowToCanvasBoardSnapshot,
  type FlowToCanvasBoardSnapshotOptions,
} from '../utils/flowSnapshotAdapter'
import { createCanvasGraphStore } from '../graph-runtime'
import {
  createCanvasDeleteCommandLifecycle,
  createCanvasNodeDragLifecycle,
  type CanvasNodeDragLifecycle,
} from '../flow-pane'
import {
  projectCanvasExecutionEdgeAnimation,
  type CanvasExecutionLease,
  type CanvasExecutionRuntime,
} from '../execution/runtime/CanvasExecutionRuntime'

type SelectableFlowItem = {
  id: string
  selected?: boolean
}

type FlowGraphCommitOptions = {
  source: 'hydrate-replace' | 'save-reconcile'
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
  executionRuntime: CanvasExecutionRuntime
  onWarning: (message: string) => void
  onSuccess: (message: string) => void
  onError: (message: string) => void
  scheduleMicrotask?: (callback: () => void) => void
  scheduleAnimationFrame?: (callback: FrameRequestCallback) => number
  cancelAnimationFrame?: (handle: number) => void
  vueFlowGraphActions?: CanvasVueFlowGraphActions
}

export function useFlowCanvasRuntime(options: UseFlowCanvasRuntimeOptions) {
  const localEditVersion = ref(0)
  const releaseNodeExecutions = (leases: readonly CanvasExecutionLease[]) => {
    options.executionRuntime.releaseNodeExecutions(leases)
  }
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

  function rebaseSavedFlowProjection(savedGraph: { nodes: Node[]; edges: Edge[] }) {
    const savedNodesById = new Map(savedGraph.nodes.map((node) => [node.id, node] as const))
    const savedEdgesById = new Map(savedGraph.edges.map((edge) => [edge.id, edge] as const))
    return {
      nodes: [
        ...savedGraph.nodes,
        ...options.nodes.value.filter(
          (node) => !savedNodesById.has(node.id) && options.executionRuntime.isNodeActive(node.id),
        ),
      ],
      edges: [
        ...savedGraph.edges,
        ...options.edges.value.filter(
          (edge) =>
            !savedEdgesById.has(edge.id) && options.executionRuntime.isNodeActive(edge.target),
        ),
      ],
    }
  }

  function projectExecutionEdges(edges: Edge[]): Edge[] {
    return projectCanvasExecutionEdgeAnimation(edges, options.executionRuntime.nodeStates.value)
      .edges
  }

  function hydratePendingExecutionLeases(nodes: Node[]) {
    const attachment = options.executionRuntime.attachment.value
    if (!attachment) return
    nodes.forEach((node) => {
      const snapshotNode = node.data?.snapshotNode as
        { type?: unknown; outputStatus?: unknown; pendingTaskId?: unknown } | undefined
      const taskId =
        typeof snapshotNode?.pendingTaskId === 'string' ? snapshotNode.pendingTaskId.trim() : ''
      if (
        snapshotNode?.type !== 'image-node' ||
        snapshotNode.outputStatus !== 'running' ||
        !taskId
      ) {
        return
      }
      options.executionRuntime.hydratePending(attachment, node.id, taskId)
    })
  }

  function commitFlowGraph(
    nextGraph: { nodes: Node[]; edges: Edge[] },
    commitOptions: FlowGraphCommitOptions,
  ) {
    if (commitOptions.source === 'hydrate-replace') {
      hydratePendingExecutionLeases(nextGraph.nodes)
    }
    const semanticGraph =
      commitOptions.source === 'save-reconcile' ? rebaseSavedFlowProjection(nextGraph) : nextGraph
    const projectedGraph = {
      nodes:
        groupRuntime?.internal.projectCurrentGroupNodeLayers(semanticGraph.nodes) ??
        semanticGraph.nodes,
      edges: projectExecutionEdges(semanticGraph.edges),
    }
    const resolvedGraph =
      commitOptions.source === 'save-reconcile'
        ? preserveCurrentSelection(projectedGraph)
        : projectedGraph
    graphAdapter.replaceGraph(resolvedGraph)
  }

  function createRestoreGraphCommandOptions() {
    return createFlowGraphCommandOptions({
      recordHistory: false,
      markDirty: false,
    })
  }

  watch(
    [options.edges, options.executionRuntime.nodeStates],
    () => {
      const projectedEdges = projectExecutionEdges(options.edges.value)
      if (projectedEdges === options.edges.value) return
      graphAdapter.replaceEdges(projectedEdges, createRestoreGraphCommandOptions())
    },
    { flush: 'sync' },
  )

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
    graphRevision: localEditVersion,
    isLoadingBoardSnapshot: options.isLoadingBoardSnapshot,
    isSavingBoard: options.isSavingBoard,
    boardSession: options.boardSession,
    readBoardAttachmentEpoch: () => options.executionRuntime.attachment.value?.epoch ?? 0,
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
    nodeDragLifecycle.cancel()
    groupRuntime?.internal.resetTransientSelectionState()
    const boardId = options.boardSession.currentBoardId.value
    if (boardId) options.executionRuntime.attachBoard(boardId)
    else options.executionRuntime.detachBoard()
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
    captureActiveNodeExecutions: options.executionRuntime.captureActiveNodeExecutions,
    releaseNodeExecutions,
  })

  const nodeDragLifecycle: CanvasNodeDragLifecycle = createCanvasNodeDragLifecycle({
    createCommand: () => {
      const store = createCurrentCanvasGraphStore()
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
    captureHistory: flowHistory.capture,
    commitHistory: flowHistory.push,
    advanceLocalEditVersion,
    markDirtyAndScheduleAutosave: markDirty,
    onBegin: groupRuntime.internal.onNodeDragStart,
    reconcileMembership: groupRuntime.internal.applyDraggedNodeMembershipChanges,
    scheduleAnimationFrame: options.scheduleAnimationFrame,
    cancelAnimationFrame: options.cancelAnimationFrame,
  })

  const deleteLifecycle = createCanvasDeleteCommandLifecycle({
    readNodes: () => options.nodes.value,
    readEdges: () => options.edges.value,
    readBoardBaseSnapshot: () => options.boardBaseSnapshot.value,
    createGraphStore: createCurrentCanvasGraphStore,
    graphAdapter,
    captureHistory: flowHistory.capture,
    commitGroups: commitHistoryGroups,
    resolveRemovedFlowNodes: groupRuntime.internal.resolveRemovedFlowNodeChanges,
    scheduleMicrotask:
      options.scheduleMicrotask ??
      ((callback: () => void) => void Promise.resolve().then(callback)),
    onWarning: options.onWarning,
    captureActiveNodeExecutions: options.executionRuntime.captureActiveNodeExecutions,
    releaseNodeExecutions,
  })

  function deleteNode(nodeId: string): boolean {
    if (!options.nodes.value.some((node) => node.id === nodeId)) return false
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
      history: flowHistory,
    },
    flowPane: {
      onNodesChange,
      onEdgesChange,
      nodeDragTransactions: {
        begin: nodeDragLifecycle.begin,
        commit: nodeDragLifecycle.commit,
      },
    },
    graphEditing: {
      adapter: graphAdapter,
      createCurrentCanvasGraphStore,
      deleteNode,
      grouping: groupRuntime,
    },
  }
}
